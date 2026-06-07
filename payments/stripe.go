package payments

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"discord-bot/config"
)

type stripeClient struct {
	cfg  *config.StripePaymentConfig
	http *http.Client
}

func newStripeClient(cfg *config.StripePaymentConfig) *stripeClient {
	return &stripeClient{
		cfg:  cfg,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *stripeClient) CreateCheckoutSession(inv *config.CommissionInvoice) (string, string, error) {
	currency := strings.ToLower(inv.Currency)
	if currency == "" {
		currency = strings.ToLower(c.cfg.Currency)
	}
	if currency == "" {
		currency = "usd"
	}

	amount := inv.Amount
	if c.cfg.HandlingFee > 0 {
		amount = amount * (1 + c.cfg.HandlingFee)
	}
	amountCents := int64(amount * 100)

	successURL := c.cfg.SuccessURL
	if successURL == "" {
		successURL = "https://example.com/payment/success"
	}
	cancelURL := c.cfg.CancelURL
	if cancelURL == "" {
		cancelURL = "https://example.com/payment/cancel"
	}

	params := url.Values{}
	params.Set("mode", "payment")
	params.Set("payment_method_types[]", "card")
	params.Set("line_items[0][price_data][currency]", currency)
	params.Set("line_items[0][price_data][product_data][name]", inv.Description)
	params.Set("line_items[0][price_data][unit_amount]", fmt.Sprintf("%d", amountCents))
	params.Set("line_items[0][quantity]", "1")
	params.Set("success_url", successURL)
	params.Set("cancel_url", cancelURL)
	params.Set("metadata[invoice_id]", fmt.Sprintf("INV-%04d", inv.Number))
	params.Set("metadata[client_discord_id]", inv.ClientID)

	req, err := http.NewRequest("POST", "https://api.stripe.com/v1/checkout/sessions",
		strings.NewReader(params.Encode()))
	if err != nil {
		return "", "", err
	}
	req.SetBasicAuth(c.cfg.SecretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("stripe request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		var errResp struct {
			Error struct{ Message string `json:"message"` } `json:"error"`
		}
		_ = json.Unmarshal(body, &errResp)
		return "", "", fmt.Errorf("stripe (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
	}

	var session struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &session); err != nil {
		return "", "", err
	}
	return session.ID, session.URL, nil
}

func (c *stripeClient) GetSessionStatus(sessionID string) (string, error) {
	req, _ := http.NewRequest("GET", "https://api.stripe.com/v1/checkout/sessions/"+sessionID, nil)
	req.SetBasicAuth(c.cfg.SecretKey, "")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var session struct {
		PaymentStatus string `json:"payment_status"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&session)
	return session.PaymentStatus, nil
}
