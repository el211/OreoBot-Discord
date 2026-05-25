package payments

import (
	"fmt"
	"log/slog"
	"strings"

	"discord-bot/config"

	"github.com/bwmarrin/discordgo"
)

type PaymentButton struct {
	Label string
	URL   string
	Emoji string
}

var Svc *Service

type Service struct {
	cfg      *config.Config
	session  *discordgo.Session
	paypal   *paypalClient
	stripe   *stripeClient
	coinbase *coinbaseClient
}

func Start(cfg *config.Config, session *discordgo.Session) {
	svc := &Service{cfg: cfg, session: session}

	pp := &cfg.Payment.PayPal
	if pp.Enabled && pp.ClientID != "" && pp.ClientSecret != "" {
		svc.paypal = newPayPalClient(pp)
		sandbox := ""
		if pp.UseSandbox {
			sandbox = " (sandbox)"
		}
		slog.Info("paypal gateway enabled", "sandbox", sandbox, "merchant", pp.MerchantEmail)
	}

	st := &cfg.Payment.Stripe
	if st.Enabled && st.SecretKey != "" {
		svc.stripe = newStripeClient(st)
		slog.Info("stripe gateway enabled")
	}

	cb := &cfg.Payment.Coinbase
	if cb.Enabled && cb.APIKey != "" {
		svc.coinbase = newCoinbaseClient(cb)
		slog.Info("coinbase commerce gateway enabled")
	}

	Svc = svc

	if svc.hasPollingGateway() {
		go svc.runPolling()
	}
	if cfg.Payment.Webhook.Enabled && cfg.Payment.Webhook.Port > 0 {
		go svc.runWebhookServer()
	}
}

func (svc *Service) hasPollingGateway() bool {
	pp := svc.cfg.Payment.PayPal
	if pp.Enabled && pp.ClientID != "" && pp.PaymentNotifications.Type == "polling" {
		return true
	}
	st := svc.cfg.Payment.Stripe
	if st.Enabled && st.SecretKey != "" && st.PaymentNotifications.Type == "polling" {
		return true
	}
	cb := svc.cfg.Payment.Coinbase
	if cb.Enabled && cb.APIKey != "" && cb.PaymentNotifications.Type == "polling" {
		return true
	}
	return false
}

func (svc *Service) CreateLinks(inv *config.CommissionInvoice) ([]PaymentButton, []error) {
	var buttons []PaymentButton
	var errs []error

	if svc.paypal != nil {
		invoiceID, payerURL, err := svc.paypal.CreateInvoice(inv)
		if err != nil {
			slog.Warn("paypal create invoice failed", "error", err)
			errs = append(errs, fmt.Errorf("PayPal: %w", err))
		} else {
			inv.PayPalInvoiceID = invoiceID
			inv.PayPalPayerURL = payerURL
			if payerURL != "" {
				label := svc.cfg.Payment.PayPal.ButtonLabel
				if label == "" {
					label = "Pay with PayPal"
				}
				buttons = append(buttons, PaymentButton{Label: label, URL: payerURL, Emoji: "💳"})
			}