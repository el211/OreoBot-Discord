package payments

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"discord-bot/config"
	"discord-bot/storage"
)

func (svc *Service) runWebhookServer() {
	port := svc.cfg.Payment.Webhook.Port
	apiURL := svc.cfg.Payment.Webhook.APIURL

	mux := http.NewServeMux()

	if svc.paypal != nil && svc.cfg.Payment.PayPal.PaymentNotifications.Type == "webhook" {
		mux.HandleFunc("/ipn/paypal", svc.handlePayPalWebhook)
		slog.Info("paypal webhook registered", "url", apiURL+"/ipn/paypal")
	}
	if svc.stripe != nil && svc.cfg.Payment.Stripe.PaymentNotifications.Type == "webhook" {
		mux.HandleFunc("/webhook/stripe", svc.handleStripeWebhook)
		slog.Info("stripe webhook registered", "url", apiURL+"/webhook/stripe")
	}
	if svc.coinbase != nil && svc.cfg.Payment.Coinbase.PaymentNotifications.Type == "webhook" {
		mux.HandleFunc("/webhook/coinbase", svc.handleCoinbaseWebhook)
		slog.Info("coinbase webhook registered", "url", apiURL+"/webhook/coinbase")
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	slog.Info("webhook server listening", "port", port)
	if err := server.ListenAndServe(); err != nil {
		slog.Error("webhook server error", "error", err)
	}
}

func (svc *Service) handlePayPalWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", 400)
		return
	}

	var event struct {
		EventType string `json:"event_type"`
		Resource  struct {
			ID string `json:"id"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "bad request", 400)
		return
	}

	slog.Info("paypal webhook event", "type", event.EventType)
	if event.EventType == "INVOICING.INVOICE.PAID" {
		svc.webhookConfirm("paypal", event.Resource.ID, svc.cfg.Payment.PayPal.Name)
	}
	w.WriteHeader(200)
}

func (svc *Service) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", 400)
		return
	}

	secret := svc.cfg.Payment.Stripe.PaymentNotifications.WebhookSigningSecret
	if secret != "" {
		sig := r.Header.Get("Stripe-Signature")
		if !verifyStripeSignature(body, sig, secret) {
			slog.Warn("stripe webhook signature verification failed")
			http.Error(w, "unauthorized", 401)
			return
		}
	}

	var event struct {
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID string `json:"id"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "bad request", 400)
		return
	}

	slog.Info("stripe webhook event", "type", event.Type)
	if event.Type == "checkout.session.completed" {
		svc.webhookConfirm("stripe", event.Data.Object.ID, svc.cfg.Payment.Stripe.Name)
	}
	w.WriteHeader(200)
}

func (svc *Service) handleCoinbaseWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", 400)
		return
	}

	secret := svc.cfg.Payment.Coinbase.PaymentNotifications.WebhookSharedSecret
	if secret != "" {
		sig := r.Header.Get("X-CC-Webhook-Signature")
		if !verifyCoinbaseSignature(body, sig, secret) {
			slog.Warn("coinbase webhook signature verification failed")
			http.Error(w, "unauthorized", 401)
			return
		}
	}

	var event struct {
		Event struct {
			Type string `json:"type"`
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		} `json:"event"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "bad request", 400)
		return
	}

	slog.Info("coinbase webhook event", "type", event.Event.Type)
	if event.Event.Type == "charge:confirmed" {
		svc.webhookConfirm("coinbase", event.Event.Data.ID, svc.cfg.Payment.Coinbase.Name)
	}
	w.WriteHeader(200)
}

func (svc *Service) webhookConfirm(gateway, paymentID, gatewayName string) {
	if gatewayName == "" {
		gatewayName = gateway
	}
	for _, gs := range storage.GetAllGuildStates() {
		gs.Lock()
		var found *config.CommissionInvoice
		for i := range gs.CommissionsRuntime.Invoices {
			inv := &gs.CommissionsRuntime.Invoices[i]
			if inv.Paid {
				continue
			}
			match := false
			switch gateway {
			case "paypal":
				match = inv.PayPalInvoiceID == paymentID
			case "stripe":
				match = inv.StripeSessionID == paymentID
			case "coinbase":
				match = inv.CoinbaseChargeID == paymentID
			}
			if match {
				inv.Paid = true
				cp := *inv
				found = &cp
				break
			}
		}
		gs.Unlock()

		if found != nil {
			_ = gs.Save()
			svc.notifyPaid(*found, gatewayName)
			return
		}
	}
	slog.Warn("no matching invoice for payment", "gateway", gateway, "paymentId", paymentID)
}

func verifyStripeSignature(payload []byte, sigHeader, secret string) bool {
	var ts, sig string
	for _, part := range strings.Split(sigHeader, ",") {
		switch {
		case strings.HasPrefix(part, "t="):
			ts = strings.TrimPrefix(part, "t=")
		case strings.HasPrefix(part, "v1="):
			sig = strings.TrimPrefix(part, "v1=")
		}
	}
	if ts == "" || sig == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + string(payload)))
	return hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(sig))
}

func verifyCoinbaseSignature(payload []byte, sigHeader, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(sigHeader))
}
