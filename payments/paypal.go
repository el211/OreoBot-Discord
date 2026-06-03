package payments

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"discord-bot/config"
)

type paypalClient struct {
	cfg        *config.PayPalPaymentConfig
	http       *http.Client
	tokenMu    sync.Mutex
	token      string
	tokenExpAt time.Time
}

func newPayPalClient(cfg *config.PayPalPaymentConfig) *paypalClient {
	return &paypalClient{
		cfg:  cfg,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *paypalClient) base() string {
	if c.cfg.UseSandbox {
		return "https://api-m.sandbox.paypal.com"
	}
	return "https://api-m.paypal.com"
}

func (c *paypalClient) getToken() (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExpAt) {
		return c.token, nil
	}

	req, err := http.NewRequest("POST", c.base()+"/v1/oauth2/token",
		bytes.NewBufferString("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}
	creds := base64.StdEncoding.EncodeToString([]byte(c.cfg.ClientID + ":" + c.cfg.ClientSecret))
	req.Header.Set("Authorization", "Basic "+creds)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("paypal oauth request: %w", err)
	}
	defer resp.Body.Close()

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", err
	}
	if tok.Error != "" {
		return "", fmt.Errorf("paypal auth: %s — %s", tok.Error, tok.ErrorDesc)
	}

	c.token = tok.AccessToken
	c.tokenExpAt = time.Now().Add(time.Duration(tok.ExpiresIn-60) * time.Second)
	return c.token, nil
}

func (c *paypalClient) do(method, path string, body interface{}) ([]byte, int, error) {
	tok, err := c.getToken()
	if err != nil {
		return nil, 0, err
	}

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.base()+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}

func (c *paypalClient) CreateInvoice(inv *config.CommissionInvoice) (string, string, error) {
	currency := inv.Currency
	if currency == "" {
		currency = c.cfg.Currency
	}
	if currency == "" {
		currency = "USD"
	}

	baseAmount := inv.Amount
	handlingFee := 0.0
	if c.cfg.HandlingFee > 0 {
		handlingFee = baseAmount * c.cfg.HandlingFee
	}

	items := []map[string]interface{}{
		{
			"name":     inv.Description,
			"quantity": "1",
			"unit_amount": map[string]string{
				"currency_code": currency,
				"value":         fmt.Sprintf("%.2f", baseAmount),
			},
			"unit_of_measure": "QUANTITY",
		},
	}
	if handlingFee > 0 {
		items = append(items, map[string]interface{}{
			"name":     "Handling Fee",
			"quantity": "1",
			"unit_amount": map[string]string{
				"currency_code": currency,
				"value":         fmt.Sprintf("%.2f", handlingFee),
			},
			"unit_of_measure": "QUANTITY",
		})
	}

	note := "Thank you for your commission!"
	if c.cfg.EnablePartialPayments && c.cfg.MinimumDuePercentage > 0 && c.cfg.MinimumDuePercentage < 100 {
		minDue := (baseAmount + handlingFee) * c.cfg.MinimumDuePercentage / 100
		note = fmt.Sprintf("Thank you! Partial payment accepted — minimum due: %.2f %s (%.0f%% of total).",
			minDue, currency, c.cfg.MinimumDuePercentage)
	}

	invoicePayload := map[string]interface{}{
		"detail": map[string]interface{}{
			"invoice_number": fmt.Sprintf("INV-%04d", inv.Number),
			"currency_code":  currency,
			"note":           note,
			"payment_term":   map[string]string{"term_type": "DUE_ON_RECEIPT"},
		},
		"invoicer": map[string]interface{}{
			"name":          map[string]string{"full_name": c.cfg.MerchantName},
			"email_address": c.cfg.MerchantEmail,
		},
		"items": items,
	}

	respBody, status, err := c.do("POST", "/v2/invoicing/invoices", invoicePayload)
	if err != nil {
		return "", "", fmt.Errorf("paypal create invoice: %w", err)
	}
	if status != 201 {
		return "", "", fmt.Errorf("paypal create invoice (HTTP %d): %s", status, string(respBody))
	}

	var created struct {
		ID    string `json:"id"`
		Links []struct {
			Href string `json:"href"`
			Rel  string `json:"rel"`
		} `json:"links"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil {
		return "", "", err
	}

	sendPayload := map[string]interface{}{
		"send_to_invoicer": true,
		"send_to_recipient": false,
	}
	if _, _, err := c.do("POST", "/v2/invoicing/invoices/"+created.ID+"/send", sendPayload); err != nil {
		return "", "", fmt.Errorf("paypal send invoice: %w", err)
	}

	payerURL := ""
	for _, link := range created.Links {
		if link.Rel == "payer-view" {
			payerURL = link.Href
			break
		}
	}

	if payerURL == "" {
		if body, _, err := c.do("GET", "/v2/invoicing/invoices/"+created.ID, nil); err == nil {
			var fetched struct {
				Links []struct {
					Href string `json:"href"`
					Rel  string `json:"rel"`
				} `json:"links"`
			}
			if json.Unmarshal(body, &fetched) == nil {
				for _, link := range fetched.Links {
					if link.Rel == "payer-view" {
						payerURL = link.Href
						break
					}
				}
			}
		}
	}

	return created.ID, payerURL, nil
}

func (c *paypalClient) GetInvoiceStatus(invoiceID string) (string, error) {
	body, _, err := c.do("GET", "/v2/invoicing/invoices/"+invoiceID, nil)
	if err != nil {
		return "", err
	}
	var result struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(body, &result)
	return result.Status, nil
}
