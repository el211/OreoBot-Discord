package handlers

import (
	"fmt"
	"strings"

	"discord-bot/config"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

// defaultInvoiceCurrency returns the configured payment currency, falling back to "USD".
// It checks Stripe first, then PayPal, then Coinbase, so there is always a consistent default
// regardless of which invoice entry point (slash command vs modal) is used.
func defaultInvoiceCurrency() string {
	cfg := storage.Cfg
	if cfg.Payment.Stripe.Currency != "" {
		return strings.ToUpper(cfg.Payment.Stripe.Currency)
	}
	if cfg.Payment.PayPal.Currency != "" {
		return strings.ToUpper(cfg.Payment.PayPal.Currency)
	}
	if cfg.Payment.Coinbase.Currency != "" {
		return strings.ToUpper(cfg.Payment.Coinbase.Currency)
	}
	return "USD"
}

func modalTextValues(rows []discordgo.MessageComponent) map[string]string {
	values := make(map[string]string)
	for _, row := range rows {
		var components []discordgo.MessageComponent
		switch ar := row.(type) {
		case discordgo.ActionsRow:
			components = ar.Components
		case *discordgo.ActionsRow:
			if ar != nil {
				components = ar.Components
			}
		default:
			continue
		}
		for _, comp := range components {
			switch ti := comp.(type) {
			case discordgo.TextInput:
				values[ti.CustomID] = ti.Value
			case *discordgo.TextInput:
				if ti != nil {
					values[ti.CustomID] = ti.Value
				}
			}
		}
	}
	return values
}

func mapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func ensureCommissionTicketRuntime(ct *config.CommissionTicket) {
	if ct.ClientThreadMessages == nil {
		ct.ClientThreadMessages = make(map[string]string)
	}
	if ct.Quotes == nil {
		ct.Quotes = make(map[string]config.CommissionQuote)
	}
}

func safeEmbedValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "*None provided*"
	}
	return truncateMessage(value, 1024)
}

func truncateMessage(value string, max int) string {
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func commissionLogComponents(guildID, ticketChannelID, threadID string, allowQuote bool, showViewLink bool) []discordgo.MessageComponent {
	var buttons []discordgo.MessageComponent
	if showViewLink {
		buttons = append(buttons, discordgo.Button{Label: "View Commission", Style: discordgo.LinkButton, URL: discordChannelURL(guildID, ticketChannelID)})
	}
	if allowQuote {