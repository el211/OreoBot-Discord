package payments

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	if status != 200 && status != 201 && status != 202 {
		return "", "", fmt.Errorf("paypal create invoice (HTTP %d): %s", status, string(respBody))
	}

	// The create response is usually just a reference link
	// {"rel":"self","href":".../v2/invoicing/invoices/<ID>"} — the invoice ID is
	// only in the href. Occasionally it is the full object with "id"/"links".
	var created struct {
		ID    string   `json:"id"`
		Href  string   `json:"href"`
		Links []ppLink `json:"links"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil {
		return "", "", err
	}
	invoiceID := created.ID
	if invoiceID == "" {
		href := created.Href
		if href == "" {
			for _, l := range created.Links {
				if l.Rel == "self" {
					href = l.Href
					break
				}
			}
		}
		invoiceID = lastPathSegment(href)
	}
	if invoiceID == "" {
		return "", "", fmt.Errorf("paypal create invoice: could not determine invoice id from response: %s", string(respBody))
	}

	// Sending the invoice returns the public payer-view URL in its response body.
	// send_to_recipient is false because the payment link is delivered via Discord.
	sendPayload := map[string]interface{}{
		"send_to_invoicer":  false,
		"send_to_recipient": false,
	}
	sendBody, sendStatus, err := c.do("POST", "/v2/invoicing/invoices/"+invoiceID+"/send", sendPayload)
	if err != nil {
		return invoiceID, "", fmt.Errorf("paypal send invoice: %w", err)
	}
	if sendStatus != 200 && sendStatus != 202 {
		return invoiceID, "", fmt.Errorf("paypal send invoice (HTTP %d): %s", sendStatus, string(sendBody))
	}

	// Primary source: the send response. Fallback: GET the invoice. Last resort:
	// construct the canonical public payment URL from the invoice ID.
	payerURL := parsePayerView(sendBody)
	if payerURL == "" {
		if body, _, gerr := c.do("GET", "/v2/invoicing/invoices/"+invoiceID, nil); gerr == nil {
			payerURL = parsePayerView(body)
		}
	}
	if payerURL == "" {
		host := "https://www.paypal.com"
		if c.cfg.UseSandbox {
			host = "https://www.sandbox.paypal.com"
		}
		payerURL = host + "/invoice/p/#" + invoiceID
	}

	return invoiceID, payerURL, nil
}

// ppLink is a PayPal HATEOAS link.
type ppLink struct {
	Href   string `json:"href"`
	Rel    string `json:"rel"`
	Method string `json:"method"`
}

// parsePayerView extracts the "payer-view" href from a PayPal response body,
// which may be a single link object or an object with a "links" array.
func parsePayerView(body []byte) string {
	var single ppLink
	if json.Unmarshal(body, &single) == nil && single.Rel == "payer-view" && single.Href != "" {
		return single.Href
	}
	var wrap struct {
		Links []ppLink `json:"links"`
	}
	if json.Unmarshal(body, &wrap) == nil {
		for _, l := range wrap.Links {
			if l.Rel == "payer-view" {
				return l.Href
			}
		}
	}
	return ""
}

// lastPathSegment returns the final non-empty path segment of a URL/href.
func lastPathSegment(href string) string {
	href = strings.TrimRight(strings.TrimSpace(href), "/")
	if href == "" {
		return ""
	}
	if i := strings.LastIndex(href, "/"); i != -1 {
		return href[i+1:]
	}
	return href
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
