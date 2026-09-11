package payments

import (
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"discord-bot/config"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

func (svc *Service) runPolling() {
	slog.Info("payment polling started, interval 60s")
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		svc.pollAll()
	}
}

func (svc *Service) pollAll() {
	for _, gs := range storage.GetAllGuildStates() {
		svc.pollGuild(gs)
	}
}

func (svc *Service) pollGuild(gs *config.GuildState) {
	gs.Lock()
	snapshot := make([]config.CommissionInvoice, len(gs.CommissionsRuntime.Invoices))
	copy(snapshot, gs.CommissionsRuntime.Invoices)
	gs.Unlock()

	for _, inv := range snapshot {
		if inv.Paid {
			continue
		}

		paid, gateway := svc.checkPaid(inv)
		if !paid {
			continue
		}

		gs.Lock()
		for j := range gs.CommissionsRuntime.Invoices {
			if gs.CommissionsRuntime.Invoices[j].Number == inv.Number {
				gs.CommissionsRuntime.Invoices[j].Paid = true
				break
			}
		}
		gs.Unlock()
		_ = gs.Save()

		svc.notifyPaid(inv, gateway)
	}
}

func (svc *Service) checkPaid(inv config.CommissionInvoice) (bool, string) {
	cfg := svc.cfg

	if svc.paypal != nil && inv.PayPalInvoiceID != "" &&
		cfg.Payment.PayPal.PaymentNotifications.Type == "polling" {
		status, err := svc.paypal.GetInvoiceStatus(inv.PayPalInvoiceID)
		if err != nil {
			slog.Warn("paypal invoice status check failed", "invoice", inv.Number, "error", err)
		} else if status == "PAID" || status == "MARKED_AS_PAID" {
			name := cfg.Payment.PayPal.Name
			if name == "" {
				name = "PayPal"
			}
			return true, name
		}
	}

	if svc.stripe != nil && inv.StripeSessionID != "" &&
		cfg.Payment.Stripe.PaymentNotifications.Type == "polling" {
		status, err := svc.stripe.GetSessionStatus(inv.StripeSessionID)
		if err != nil {
			slog.Warn("stripe session status check failed", "invoice", inv.Number, "error", err)
		} else if status == "paid" {
			name := cfg.Payment.Stripe.Name
			if name == "" {
				name = "Stripe"
			}
			return true, name
		}
	}

	if svc.coinbase != nil && inv.CoinbaseChargeID != "" &&
		cfg.Payment.Coinbase.PaymentNotifications.Type == "polling" {
		status, err := svc.coinbase.GetChargeStatus(inv.CoinbaseChargeID)
		if err != nil {
			slog.Warn("coinbase charge status check failed", "invoice", inv.Number, "error", err)
		} else if status == "COMPLETED" || status == "RESOLVED" {
			name := cfg.Payment.Coinbase.Name
			if name == "" {
				name = "Coinbase Commerce"
			}
			return true, name
		}
	}

	// CDP address flow: an invoice is paid once any of its per-asset addresses
	// has received at least the expected amount (0.5% under-payment tolerance).
	if svc.coinbaseCDP != nil && len(inv.CoinbaseCryptoPayments) > 0 {
		for _, p := range inv.CoinbaseCryptoPayments {
			expected, perr := strconv.ParseFloat(p.Amount, 64)
			if perr != nil || expected <= 0 {
				continue
			}
			received, err := svc.coinbaseCDP.AddressReceived(p.AccountID, p.AddressID)
			if err != nil {
				slog.Warn("coinbase cdp address check failed", "invoice", inv.Number, "asset", p.Asset, "error", err)
				continue
			}
			if received >= expected*0.995 {
				name := cfg.Payment.Coinbase.Name
				if name == "" {
					name = "Coinbase"
				}
				return true, fmt.Sprintf("%s (%s)", name, p.Asset)
			}
		}
	}

	return false, ""
}

func (svc *Service) notifyPaid(inv config.CommissionInvoice, gateway string) {
	if inv.ChannelID == "" || svc.session == nil {
		return
	}
	embed := &discordgo.MessageEmbed{
		Title: "✅ Payment Received!",
		Color: 0x57F287,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Invoice", Value: fmt.Sprintf("`INV-%04d`", inv.Number), Inline: true},
			{Name: "Client", Value: fmt.Sprintf("<@%s>", inv.ClientID), Inline: true},
			{Name: "Amount", Value: fmt.Sprintf("**%.2f %s**", inv.Amount, inv.Currency), Inline: true},
			{Name: "Gateway", Value: gateway, Inline: true},
		},
		Footer:    &discordgo.MessageEmbedFooter{Text: "Payment confirmed via " + gateway},
		Timestamp: time.Now().Format(time.RFC3339),
	}
	_, err := svc.session.ChannelMessageSendComplex(inv.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s> Your payment has been confirmed! Thank you! 🎉", inv.ClientID),
		Embeds:  []*discordgo.MessageEmbed{embed},
	})
	if err != nil {
		slog.Warn("failed to notify payment channel", "channel", inv.ChannelID, "error", err)
	}
}
