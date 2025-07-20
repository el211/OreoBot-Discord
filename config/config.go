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
	CancelURL            string                     `json:"cancel_url"`
	PaymentNotifications PaymentNotificationsConfig `json:"payment_notifications"`
}

type CoinbasePaymentConfig struct {
	Enabled              bool                       `json:"enabled"`
	Name                 string                     `json:"name"`
	ButtonLabel          string                     `json:"button_label"`
	APIKey               string                     `json:"api_key"`
	HandlingFee          float64                    `json:"handling_fee"`
	Currency             string                     `json:"currency"`
	PaymentNotifications PaymentNotificationsConfig `json:"payment_notifications"`
}

// WebhookServerConfig configures the optional HTTP server for receiving payment events.
type WebhookServerConfig struct {
	// Enabled starts the HTTP server to receive webhook events.
	Enabled bool `json:"enabled"`
	// Port the server listens on (e.g. 8080).
	Port int `json:"port"`
	// APIURL is the public base URL of this server (e.g. "https://api.example.com").
	// Used to tell you where to point PayPal/Stripe/Coinbase webhook settings.
	APIURL string `json:"api_url"`
}

// VerifyConfig maps product names to Discord role IDs for the /verify command.
type VerifyConfig struct {
	// Products is the list of purchasable products and their associated role.
	Products []VerifyProduct `json:"products"`
}

// VerifyProduct pairs a product name (shown in autocomplete) with the role to assign.
type VerifyProduct struct {
	// Name is the display name shown in the /verify autocomplete list (e.g. "ModeledNPCs").
	Name string `json:"name"`

	// RoleID is the Discord role ID to grant when this product is verified.
	RoleID string `json:"role_id"`
}

// CustomCommandConfig defines a user-created slash command that replies with a fixed message.
type CustomCommandConfig struct {
	// Name of the slash command (no spaces, lowercase).
	Name string `json:"name"`

	// Description shown in Discord's command picker.
	Description string `json:"description"`

	// Message sent when the command is used. Supports Discord markdown.
	Message string `json:"message"`

	// Ephemeral: if true, only the user who ran the command sees the reply.
	Ephemeral bool `json:"ephemeral"`
}

// NoPingConfig prevents certain roles from being mentioned.
type NoPingConfig struct {
	// Enable the no-ping rule.
	Enabled bool `json:"enabled"`

	// ProtectedRoles is a list of role IDs that must not be pinged.
	ProtectedRoles []string `json:"protected_roles"`

	// BypassRoles is a list of role IDs that are allowed to ping protected roles.
	// Members with any of these roles are exempt from the no-ping restriction.
	BypassRoles []string `json:"bypass_roles"`

	// Message sent to the user when they ping a protected role.
	// Use {user} for the offender's mention and {role} for the pinged role name.
	Message string `json:"message"`

	// DeleteMessage: delete the offending message (default true).
	DeleteMessage bool `json:"delete_message"`
}

// LinkFilterConfig auto-deletes messages containing links, except for
// members with an allowed role or links to whitelisted domains.
type LinkFilterConfig struct {
	// Enable the link filter.
	Enabled bool `json:"enabled"`

	// AllowedRoles is a list of role IDs whose members may post any link.
	AllowedRoles []string `json:"allowed_roles"`

	// WhitelistDomains is a list of domains that are always allowed for everyone,
	// e.g. ["github.com", "youtube.com"]. Subdomains are matched too
	// (whitelisting "github.com" also allows "gist.github.com").
	WhitelistDomains []string `json:"whitelist_domains"`

	// BlockInvites always blocks Discord invite links (discord.gg / discord.com/invite)
	// even if discord.com is whitelisted. Default true.
	BlockInvites bool `json:"block_invites"`

	// DeleteMessage: delete the offending message (default true).
	DeleteMessage bool `json:"delete_message"`

	// Message sent to the user when their message is removed.
	// Use {user} for the offender's mention. A default is used if empty.
	Message string `json:"message"`
}

// UnmarshalJSON decodes LinkFilterConfig with DeleteMessage and BlockInvites
// defaulting to true when the keys are omitted, while still honouring an
// explicit "false" in the config file.
func (c *LinkFilterConfig) UnmarshalJSON(data []byte) error {
	type alias LinkFilterConfig
	tmp := alias{DeleteMessage: true, BlockInvites: true}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*c = LinkFilterConfig(tmp)
	return nil
}

// CountingGameConfig configures the counting minigame channel.
type CountingGameConfig struct {
	// Enable the counting game.
	Enabled bool `json:"enabled"`

	// Discord channel ID where the game takes place.
	ChannelID string `json:"channel_id"`

	// FailResets: if true, a wrong number resets the count back to 0.
	// If false, the wrong message is simply deleted and the count stays.
	FailResets bool `json:"fail_resets"`

	// DeleteWrong: delete messages that contain the wrong number (default true).
	// Set to false to only warn without deleting.
	DeleteWrong bool `json:"delete_wrong"`

	// DeleteNonNumbers: delete messages that are not numbers at all (keeps the channel clean).
	DeleteNonNumbers bool `json:"delete_non_numbers"`
}

// ChatBridgeConfig wires up a bidirectional Minecraft ↔ Discord chat bridge
// via RabbitMQ (the same fanout exchange used by OreoEssentials ChatSyncManager).
type ChatBridgeConfig struct {
	// Enable the bridge. Everything below is ignored when false.
	Enabled bool `json:"enabled"`

	// RabbitMQ connection URI, e.g. "amqp://user:pass@host:5672/"
	// Leave empty to disable the bridge even if Enabled is true.
	RabbitMQURI string `json:"rabbitmq_uri"`

	// Discord channel ID where MC chat is relayed and Discord users can chat back.
	ChannelID string `json:"channel_id"`

	// (Optional) OreoEssentials channel ID to target when channels mode is active.
	// Leave empty if your servers run without the channels system — legacy format is used instead.
	MCChannelID string `json:"mc_channel_id"`

	// BanSync: when true, banning a linked user from Discord also bans them in Minecraft via RCON.
	// Requires Minecraft.Enabled and a working RCON connection.
	BanSync bool `json:"ban_sync"`

	// ModSync: when true, /mute and /unmute on a linked user also mutes/unmutes them
	// on all Minecraft servers via RabbitMQ (CTRL;;MUTE / CTRL;;UNMUTE).
	ModSync bool `json:"mod_sync"`
}

type DiscordConfig struct {
	Token   string `json:"token"`
	GuildID string `json:"guild_id"`
	Prefix  string `json:"prefix"`
}

type YouTubeConfig struct {
	APIKey string `json:"api_key"`
}

type DatabaseConfig struct {
	Driver  string        `json:"driver"`
	SQLite  SQLiteConfig  `json:"sqlite"`
	MongoDB MongoDBConfig `json:"mongodb"`
}

type SQLiteConfig struct {
	Path string `json:"path"`
}

type MongoDBConfig struct {
	URI      string `json:"uri"`
	Database string `json:"database"`
}

type MinecraftConfig struct {
	Enabled      bool   `json:"enabled"`
	RCONAddress  string `json:"rcon_ip"`
	RCONPort     int    `json:"rcon_port"`
	RCONPassword string `json:"rcon_password"`

	LinkBackend string `json:"link_backend"`
}

type PermissionsConfig struct {
	AdminRoles     []string `json:"admin_roles"`