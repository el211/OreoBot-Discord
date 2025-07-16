package config

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

type Config struct {
	Discord        DiscordConfig         `json:"discord"`
	YouTube        YouTubeConfig         `json:"youtube"`
	Database       DatabaseConfig        `json:"database"`
	Minecraft      MinecraftConfig       `json:"minecraft"`
	Permissions    PermissionsConfig     `json:"permissions"`
	Music          MusicConfig           `json:"music"`
	Welcome        WelcomeLeaveConfig    `json:"welcome"`
	Leave          WelcomeLeaveConfig    `json:"leave"`
	Moderation     ModerationConfig      `json:"moderation"`
	Tickets        TicketsConfig         `json:"tickets"`
	Commissions    CommissionsConfig     `json:"commissions"`
	ChatBridge     ChatBridgeConfig      `json:"chat_bridge"`
	CountingGame   CountingGameConfig    `json:"counting_game"`
	NoPing         NoPingConfig          `json:"no_ping"`
	LinkFilter     LinkFilterConfig      `json:"link_filter"`
	Verify         VerifyConfig          `json:"verify"`
	CustomCommands []CustomCommandConfig `json:"custom_commands"`

	// AutoRestartMinutes sets how often the bot performs a clean scheduled
	// restart (exit code 2). The restart script relaunches it immediately.
	// Set to 0 to disable. Can be overridden by the -restart CLI flag.
	AutoRestartMinutes int `json:"auto_restart_minutes"`

	Payment PaymentConfig `json:"payment"`
	GitHub  GitHubConfig  `json:"github"`
}

// ──────────────────────────────────────────
// Payment gateway configuration
// ──────────────────────────────────────────

// PaymentConfig is the top-level payment configuration block.
type PaymentConfig struct {
	PayPal   PayPalPaymentConfig   `json:"paypal"`
	Stripe   StripePaymentConfig   `json:"stripe"`
	Coinbase CoinbasePaymentConfig `json:"coinbase"`
	Webhook  WebhookServerConfig   `json:"webhook"`
}

// PaymentNotificationsConfig selects how the bot learns about new payments.
type PaymentNotificationsConfig struct {
	// Type is "polling" (bot polls the API every minute) or "webhook" (gateway POSTs to the bot).
	Type string `json:"type"`
	// WebhookID is the PayPal webhook ID (only used when type = "webhook").
	WebhookID string `json:"webhook_id,omitempty"`
	// WebhookSharedSecret is the Coinbase Commerce shared secret (type = "webhook").
	WebhookSharedSecret string `json:"webhook_shared_secret,omitempty"`
	// WebhookSigningSecret is the Stripe webhook signing secret (type = "webhook").
	WebhookSigningSecret string `json:"webhook_signing_secret,omitempty"`
}

type PayPalPaymentConfig struct {
	Enabled bool `json:"enabled"`
	// Name shown in logs; defaults to "PayPal".
	Name string `json:"name"`
	// ButtonLabel is the text on the Discord payment button.
	ButtonLabel string `json:"button_label"`
	// UseSandbox switches to the PayPal sandbox environment.
	UseSandbox bool `json:"use_sandbox"`
	// ClientID and ClientSecret are your PayPal REST app credentials.
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	// MerchantName and MerchantEmail appear on the generated PayPal invoice.
	MerchantName  string `json:"merchant_name"`
	MerchantEmail string `json:"merchant_email"`
	// EnablePartialPayments allows clients to pay a portion of the invoice upfront.
	EnablePartialPayments bool `json:"enable_partial_payments"`
	// MinimumDuePercentage is the minimum % of the total that must be paid (0–100).
	MinimumDuePercentage float64 `json:"minimum_due_percentage"`
	// HandlingFee is an additional percentage added on top of the invoice amount (0.1 = 10%).
	HandlingFee float64 `json:"handling_fee"`
	// Currency is the default ISO currency code (e.g. "EUR", "USD").
	Currency             string                     `json:"currency"`
	PaymentNotifications PaymentNotificationsConfig `json:"payment_notifications"`
}

type StripePaymentConfig struct {
	Enabled     bool   `json:"enabled"`
	Name        string `json:"name"`
	ButtonLabel string `json:"button_label"`
	// UseSandbox is informational only — use test keys when true.
	UseSandbox     bool    `json:"use_sandbox"`
	PublishableKey string  `json:"publishable_key"`
	SecretKey      string  `json:"secret_key"`
	HandlingFee    float64 `json:"handling_fee"`
	Currency       string  `json:"currency"`
	// SuccessURL and CancelURL are the redirect URLs after Stripe checkout.
	SuccessURL           string                     `json:"success_url"`