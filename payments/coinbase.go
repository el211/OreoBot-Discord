package payments

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"discord-bot/config"
)

const coinbaseBase = "https://api.commerce.coinbase.com"

type coinbaseClient struct {
	cfg  *config.CoinbasePaymentConfig
	http *http.Client
}

func newCoinbaseClient(cfg *config.CoinbasePaymentConfig) *coinbaseClient {
	return &coinbaseClient{
		cfg:  cfg,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *coinbaseClient) request(method, path string, body interface{}) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, coinbaseBase+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("X-CC-Api-Key", c.cfg.APIKey)
	req.Header.Set("X-CC-Version", "2018-03-22")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}

func (c *coinbaseClient) CreateCharge(inv *config.CommissionInvoice) (string, string, error) {
	currency := inv.Currency
	if currency == "" {
		currency = c.cfg.Currency
	}
	if currency == "" {
		currency = "USD"
	}

	amount := inv.Amount
	if c.cfg.HandlingFee > 0 {
		amount = amount * (1 + c.cfg.HandlingFee)
	}

	payload := map[string]interface{}{
		"name":         inv.Description,
		"description":  fmt.Sprintf("Commission invoice INV-%04d", inv.Number),
		"pricing_type": "fixed_price",
		"local_price": map[string]string{
			"amount":   fmt.Sprintf("%.2f", amount),
			"currency": currency,
		},
		"metadata": map[string]string{
			"invoice_id":        fmt.Sprintf("INV-%04d", inv.Number),
			"client_discord_id": inv.ClientID,
		},
	}

	body, status, err := c.request("POST", "/charges", payload)
	if err != nil {
		return "", "", fmt.Errorf("coinbase create charge: %w", err)
	}
	if status != 201 {
		return "", "", fmt.Errorf("coinbase (HTTP %d): %s", status, string(body))
	}

	var result struct {
		Data struct {
			ID        string `json:"id"`
			HostedURL string `json:"hosted_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", err
	}
	return result.Data.ID, result.Data.HostedURL, nil
}

func (c *coinbaseClient) GetChargeStatus(chargeID string) (string, error) {
	body, _, err := c.request("GET", "/charges/"+chargeID, nil)
	if err != nil {
		return "", err
	}

	var result struct {
		Data struct {
			Timeline []struct {
				Status string `json:"status"`
			} `json:"timeline"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &result)

	tl := result.Data.Timeline
	if len(tl) == 0 {
		return "NEW", nil
	}
	return tl[len(tl)-1].Status, nil
}
