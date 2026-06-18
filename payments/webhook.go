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