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
	Enabled     bool   `json:"enabled"`
	Name        string `json:"name"`
	ButtonLabel string `json:"button_label"`

	// APIKey is the legacy Coinbase Commerce API key (X-CC-Api-Key). When set and
	// no CDP key is configured, the hosted-checkout Commerce flow is used.
	APIKey string `json:"api_key"`

	// CDPKeyName and CDPPrivateKey configure the Coinbase Developer Platform
	// (Coinbase App API) address flow, using a normal Coinbase account.
	// CDPKeyName is "organizations/{org}/apiKeys/{id}"; CDPPrivateKey is the
	// base64-encoded Ed25519 secret. When both are set, the CDP address flow is
	// used instead of Commerce: the bot generates a receive address per asset.
	CDPKeyName    string          `json:"cdp_key_name"`
	CDPPrivateKey string          `json:"cdp_private_key"`
	Assets        []CoinbaseAsset `json:"assets"`

	HandlingFee          float64                    `json:"handling_fee"`
	Currency             string                     `json:"currency"`
	PaymentNotifications PaymentNotificationsConfig `json:"payment_notifications"`
}

// CoinbaseAsset is one crypto asset customers may pay an invoice in (CDP mode).
type CoinbaseAsset struct {
	// Asset is the Coinbase account currency code, e.g. "BTC", "ETH", "USDC".
	Asset string `json:"asset"`
	// Network is the optional address network, e.g. "base" for USDC. Empty uses
	// the asset's default network.
	Network string `json:"network,omitempty"`
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
	ModeratorRoles []string `json:"moderator_roles"`
	DJRoles        []string `json:"dj_roles"`
}

type MusicConfig struct {
	Enabled         bool   `json:"enabled"`
	Backend         string `json:"backend"`
	MaxQueueSize    int    `json:"max_queue_size"`
	MaxSongDuration int    `json:"max_song_duration"`
	AllowPlaylists  bool   `json:"allow_playlists"`
	DefaultVolume   int    `json:"default_volume"`

	Direct   DirectMusicConfig   `json:"direct"`
	Lavalink LavalinkMusicConfig `json:"lavalink"`
}

type DirectMusicConfig struct {
	YTDLPPath  string `json:"ytdlp_path"`
	FFmpegPath string `json:"ffmpeg_path"`
}

type LavalinkMusicConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	Secure   bool   `json:"secure"`
}

type ModerationConfig struct {
	ModLogChannel string        `json:"mod_log_channel"`
	MuteRole      string        `json:"mute_role"`
	AutoMod       AutoModConfig `json:"auto_mod"`
}

type WelcomeLeaveConfig struct {
	Enabled   bool        `json:"enabled"`
	ChannelID string      `json:"channel_id"`
	Embed     EmbedConfig `json:"embed"`
}

type EmbedConfig struct {
	Colour       string `json:"colour"`
	Title        string `json:"title"`
	Message      string `json:"message"`
	Thumbnail    string `json:"thumbnail"`
	ImageEnabled bool   `json:"image_enabled"`
	ImageURL     string `json:"image_url"`
}

type AutoModConfig struct {
	Enabled         bool `json:"enabled"`
	MaxMentions     int  `json:"max_mentions"`
	MaxLines        int  `json:"max_lines"`
	AntiSpamSeconds int  `json:"anti_spam_seconds"`
	AntiSpamCount   int  `json:"anti_spam_count"`
}

type TicketsConfig struct {
	Enabled         bool             `json:"enabled"`
	PanelChannel    string           `json:"panel_channel"`
	LogChannel      string           `json:"log_channel"`
	StaffRoles      string           `json:"staff_roles"`
	DiscordCategory string           `json:"discord_category"`
	MaxOpenPerUser  int              `json:"max_open_per_user"`
	Categories      []TicketCategory `json:"categories"`
}

type TicketCategory struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Emoji         string              `json:"emoji"`
	Description   string              `json:"description"`
	StaffRoles    string              `json:"staff_role"`
	Subcategories []TicketSubcategory `json:"subcategories"`
}

type TicketSubcategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Emoji       string `json:"emoji"`
	Description string `json:"description"`
}

// ──────────────────────────────────────────
// Commissions system
// ──────────────────────────────────────────

type CommissionsConfig struct {
	Enabled         bool                `json:"enabled"`
	PanelChannel    string              `json:"panel_channel"`
	LogChannel      string              `json:"log_channel"`
	StaffRoles      string              `json:"staff_roles"`
	DiscordCategory string              `json:"discord_category"`
	PayPalEmail     string              `json:"paypal_email"`
	PayPalMeUser    string              `json:"paypal_me_user"`
	Services        []CommissionService `json:"services"`
}

type CommissionService struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Emoji         string `json:"emoji"`
	Description   string `json:"description"`
	StartingPrice string `json:"starting_price,omitempty"`
}

type CommissionTicket struct {
	ChannelID            string                     `json:"channel_id"`
	UserID               string                     `json:"user_id"`
	ServiceID            string                     `json:"service_id"`
	ServiceName          string                     `json:"service_name"`
	Details              string                     `json:"details"`
	Budget               string                     `json:"budget"`
	Timeline             string                     `json:"timeline"`
	Notes                string                     `json:"notes"`
	Number               int                        `json:"number"`
	CreatedAt            string                     `json:"created_at"`
	LogChannelID         string                     `json:"log_channel_id,omitempty"`
	LogMessageID         string                     `json:"log_message_id,omitempty"`
	DiscussionThreadID   string                     `json:"discussion_thread_id,omitempty"`
	AcceptedFreelancerID string                     `json:"accepted_freelancer_id,omitempty"`
	ClientThreadMessages map[string]string          `json:"client_thread_messages,omitempty"`
	Quotes               map[string]CommissionQuote `json:"quotes,omitempty"`
}

type CommissionQuote struct {
	ID           string  `json:"id"`
	FreelancerID string  `json:"freelancer_id"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
	Timeline     string  `json:"timeline"`
	Message      string  `json:"message"`
	Status       string  `json:"status"`
	Reason       string  `json:"reason,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

type CommissionInvoice struct {
	Number      int     `json:"number"`
	ChannelID   string  `json:"channel_id"`
	GuildID     string  `json:"guild_id"`
	ClientID    string  `json:"client_id"`
	CreatedBy   string  `json:"created_by"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Description string  `json:"description"`
	Note        string  `json:"note"`
	CreatedAt   string  `json:"created_at"`
	Paid        bool    `json:"paid"`
	// Payment gateway tracking
	PayPalInvoiceID   string `json:"paypal_invoice_id,omitempty"`
	PayPalPayerURL    string `json:"paypal_payer_url,omitempty"`
	StripeSessionID   string `json:"stripe_session_id,omitempty"`
	StripePaymentURL  string `json:"stripe_payment_url,omitempty"`
	CoinbaseChargeID  string `json:"coinbase_charge_id,omitempty"`
	CoinbaseHostedURL string `json:"coinbase_hosted_url,omitempty"`
	// CoinbaseCryptoPayments holds per-asset receive addresses (CDP address flow).
	CoinbaseCryptoPayments []CoinbaseCryptoPayment `json:"coinbase_crypto_payments,omitempty"`
}

// CoinbaseCryptoPayment is a generated receive address + expected amount for one
// asset on an invoice (Coinbase CDP address flow).
type CoinbaseCryptoPayment struct {
	Asset     string `json:"asset"`
	Network   string `json:"network,omitempty"`
	Address   string `json:"address"`
	AddressID string `json:"address_id"`
	AccountID string `json:"account_id"`
	// Amount is the expected crypto amount as a decimal string, e.g. "0.00042".
	Amount string `json:"amount"`
}

type CommissionsRuntime struct {
	Enabled                 bool                        `json:"enabled"`
	PanelChannelOverride    string                      `json:"panel_channel_override,omitempty"`
	LogChannelOverride      string                      `json:"log_channel_override,omitempty"`
	StaffRolesOverride      string                      `json:"staff_roles_override,omitempty"`
	DiscordCategoryOverride string                      `json:"discord_category_override,omitempty"`
	PayPalEmail             string                      `json:"paypal_email,omitempty"`
	PayPalMeUser            string                      `json:"paypal_me_user,omitempty"`
	PanelMessageID          string                      `json:"panel_message_id,omitempty"`
	CommissionCounter       int                         `json:"commission_counter"`
	InvoiceCounter          int                         `json:"invoice_counter"`
	Services                []CommissionService         `json:"services"`
	OpenCommissions         map[string]CommissionTicket `json:"open_commissions"`
	Invoices                []CommissionInvoice         `json:"invoices"`
}

type GitHubSubscription struct {
	Repo      string   `json:"repo"`       // "owner/repo" lowercase
	ChannelID string   `json:"channel_id"` // Discord channel ID
	Events    []string `json:"events"`     // e.g. ["push","pull_request"]
}

type GitHubConfig struct {
	Enabled       bool                 `json:"enabled"`
	WebhookSecret string               `json:"webhook_secret"`
	WebhookPort   int                  `json:"webhook_port"`
	Subscriptions []GitHubSubscription `json:"subscriptions"`
}

type AutoRoleState struct {
	Enabled bool   `json:"enabled"`
	RoleID  string `json:"role_id"`
}

type RoleMenu struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	ChannelID    string          `json:"channel_id"`
	MessageID    string          `json:"message_id"`
	SingleSelect bool            `json:"single_select"`
	Roles        []RoleMenuEntry `json:"roles"`
}

type RoleMenuEntry struct {
	RoleID string `json:"role_id"`
	Label  string `json:"label"`
	Emoji  string `json:"emoji"`
}

type Giveaway struct {
	ID         string          `json:"id"`
	GuildID    string          `json:"guild_id"`
	ChannelID  string          `json:"channel_id"`
	MessageID  string          `json:"message_id"`
	Prize      string          `json:"prize"`
	Winners    int             `json:"winners"`
	EndsAt     string          `json:"ends_at"` // RFC3339
	HostID     string          `json:"host_id"`
	Ended      bool            `json:"ended"`
	WinnerIDs  []string        `json:"winner_ids"`
	EntrantIDs map[string]bool `json:"entrant_ids"`
}

type NoPingRuntime struct {
	BypassUsers map[string]bool `json:"bypass_users"`
}

// LinkFilterRuntime holds link-filter settings added at runtime via /linkfilter,
// merged on top of the config-file LinkFilterConfig.
type LinkFilterRuntime struct {
	// ExtraAllowedRoles are role IDs allowed to post links, added via command.
	ExtraAllowedRoles []string `json:"extra_allowed_roles,omitempty"`
	// ExtraWhitelistDomains are domains whitelisted via command.
	ExtraWhitelistDomains []string `json:"extra_whitelist_domains,omitempty"`
	// MessageOverride replaces the config-file warning message when set.
	MessageOverride string `json:"message_override,omitempty"`
}

type GuildState struct {
	mu       sync.RWMutex
	filePath string

	GuildID string `json:"guild_id"`

	ModLogChannelOverride string `json:"mod_log_channel_override,omitempty"`
	MuteRoleOverride      string `json:"mute_role_override,omitempty"`

	TicketRuntime      TicketRuntime      `json:"ticket_runtime"`
	CommissionsRuntime CommissionsRuntime `json:"commissions_runtime"`

	Warnings map[string][]Warning `json:"warnings"`

	AutoRole     AutoRoleState  `json:"autorole"`
	RoleMenus    []RoleMenu     `json:"role_menus"`
	Giveaways    []Giveaway     `json:"giveaways"`
	InviteCounts map[string]int    `json:"invite_counts"`
	NoPing       NoPingRuntime     `json:"no_ping"`
	LinkFilter   LinkFilterRuntime `json:"link_filter"`

	GitHubSubscriptions []GitHubSubscription `json:"github_subscriptions,omitempty"`
}

type TicketRuntime struct {
	PanelChannelOverride    string            `json:"panel_channel_override,omitempty"`
	LogChannelOverride      string            `json:"log_channel_override,omitempty"`
	StaffRolesOverride      string            `json:"staff_roles_override,omitempty"`
	DiscordCategoryOverride string            `json:"discord_category_override,omitempty"`
	PanelMessageID          string            `json:"panel_message_id"`
	TicketCounter           int               `json:"ticket_counter"`
	OpenTickets             map[string]Ticket `json:"open_tickets"`

	ExtraCategories []TicketCategory `json:"extra_categories,omitempty"`
}

type Ticket struct {
	ChannelID   string `json:"channel_id"`
	UserID      string `json:"user_id"`
	CategoryID  string `json:"category_id"`
	SubCategory string `json:"sub_category"`
	Number      int    `json:"number"`
	CreatedAt   string `json:"created_at"`
}

type Warning struct {
	ID        int    `json:"id"`
	Reason    string `json:"reason"`
	ModID     string `json:"mod_id"`
	Timestamp string `json:"timestamp"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Tickets.MaxOpenPerUser <= 0 {
		cfg.Tickets.MaxOpenPerUser = 1
	}
	if cfg.Music.MaxQueueSize <= 0 {
		cfg.Music.MaxQueueSize = 100
	}
	if cfg.Music.DefaultVolume <= 0 {
		cfg.Music.DefaultVolume = 50
	}
	if cfg.Music.Backend == "" {
		cfg.Music.Backend = "direct"
	}
	if cfg.Music.Direct.YTDLPPath == "" {
		cfg.Music.Direct.YTDLPPath = "yt-dlp"
	}
	if cfg.Music.Direct.FFmpegPath == "" {
		cfg.Music.Direct.FFmpegPath = "ffmpeg"
	}
	if cfg.Music.Lavalink.Host == "" {
		cfg.Music.Lavalink.Host = "localhost"
	}
	if cfg.Music.Lavalink.Port == 0 {
		cfg.Music.Lavalink.Port = 2333
	}
	if cfg.Music.Lavalink.Password == "" {
		cfg.Music.Lavalink.Password = "youshallnotpass"
	}
	if cfg.Database.Driver == "" {
		cfg.Database.Driver = "sqlite"
	}
	if cfg.Database.SQLite.Path == "" {
		cfg.Database.SQLite.Path = "data/bot.db"
	}
	return &cfg, nil
}

func SaveConfig(cfg *Config, path string) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func LoadGuildState(guildID string) *GuildState {
	dir := "data/guilds"
	_ = os.MkdirAll(dir, 0755)
	path := dir + "/" + guildID + ".json"

	gs := &GuildState{
		GuildID:  guildID,
		filePath: path,
		Warnings: make(map[string][]Warning),
		TicketRuntime: TicketRuntime{
			OpenTickets: make(map[string]Ticket),
		},
		CommissionsRuntime: CommissionsRuntime{
			OpenCommissions: make(map[string]CommissionTicket),
			Services:        []CommissionService{},
			Invoices:        []CommissionInvoice{},
		},
		RoleMenus: []RoleMenu{},
		Giveaways: []Giveaway{},
		NoPing: NoPingRuntime{
			BypassUsers: make(map[string]bool),
		},
	}

	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, gs)
	}
	gs.filePath = path
	if gs.Warnings == nil {
		gs.Warnings = make(map[string][]Warning)
	}
	if gs.TicketRuntime.OpenTickets == nil {
		gs.TicketRuntime.OpenTickets = make(map[string]Ticket)
	}
	if gs.CommissionsRuntime.OpenCommissions == nil {
		gs.CommissionsRuntime.OpenCommissions = make(map[string]CommissionTicket)
	}
	if gs.CommissionsRuntime.Services == nil {
		gs.CommissionsRuntime.Services = []CommissionService{}
	}
	if gs.CommissionsRuntime.Invoices == nil {
		gs.CommissionsRuntime.Invoices = []CommissionInvoice{}
	}
	if gs.RoleMenus == nil {
		gs.RoleMenus = []RoleMenu{}
	}
	if gs.Giveaways == nil {
		gs.Giveaways = []Giveaway{}
	}
	if gs.InviteCounts == nil {
		gs.InviteCounts = make(map[string]int)
	}
	if gs.NoPing.BypassUsers == nil {
		gs.NoPing.BypassUsers = make(map[string]bool)
	}
	if gs.GitHubSubscriptions == nil {
		gs.GitHubSubscriptions = []GitHubSubscription{}
	}
	return gs
}

func (gs *GuildState) Save() error {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	data, err := json.MarshalIndent(gs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(gs.filePath, data, 0644)
}

func (gs *GuildState) Lock()   { gs.mu.Lock() }
func (gs *GuildState) Unlock() { gs.mu.Unlock() }

// MergedLinkFilterAllowedRoles merges config-file allowed roles with runtime ones.
func MergedLinkFilterAllowedRoles(cfg *Config, gs *GuildState) []string {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	out := make([]string, 0, len(cfg.LinkFilter.AllowedRoles)+len(gs.LinkFilter.ExtraAllowedRoles))
	out = append(out, cfg.LinkFilter.AllowedRoles...)
	out = append(out, gs.LinkFilter.ExtraAllowedRoles...)
	return out
}

// MergedLinkFilterWhitelist merges config-file whitelisted domains with runtime ones.
func MergedLinkFilterWhitelist(cfg *Config, gs *GuildState) []string {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	out := make([]string, 0, len(cfg.LinkFilter.WhitelistDomains)+len(gs.LinkFilter.ExtraWhitelistDomains))
	out = append(out, cfg.LinkFilter.WhitelistDomains...)
	out = append(out, gs.LinkFilter.ExtraWhitelistDomains...)
	return out
}

// EffectiveLinkFilterMessage returns the runtime message override if set,
// otherwise the config-file message, otherwise a built-in default.
func EffectiveLinkFilterMessage(cfg *Config, gs *GuildState) string {
	gs.mu.RLock()
	override := gs.LinkFilter.MessageOverride
	gs.mu.RUnlock()
	if override != "" {
		return override
	}
	if cfg.LinkFilter.Message != "" {
		return cfg.LinkFilter.Message
	}
	return "{user} You are not allowed to post links here."
}

func MergedTicketCategories(cfg *Config, gs *GuildState) []TicketCategory {
	all := make([]TicketCategory, 0, len(cfg.Tickets.Categories)+len(gs.TicketRuntime.ExtraCategories))
	all = append(all, cfg.Tickets.Categories...)
	all = append(all, gs.TicketRuntime.ExtraCategories...)
	return all
}

func EffectiveTicketPanelChannel(cfg *Config, gs *GuildState) string {
	if gs.TicketRuntime.PanelChannelOverride != "" {
		return gs.TicketRuntime.PanelChannelOverride
	}
	return cfg.Tickets.PanelChannel
}

func EffectiveTicketLogChannel(cfg *Config, gs *GuildState) string {
	if gs.TicketRuntime.LogChannelOverride != "" {
		return gs.TicketRuntime.LogChannelOverride
	}
	return cfg.Tickets.LogChannel
}

func EffectiveTicketStaffRoles(cfg *Config, gs *GuildState) []string {
	raw := cfg.Tickets.StaffRoles
	if gs.TicketRuntime.StaffRolesOverride != "" {
		raw = gs.TicketRuntime.StaffRolesOverride
	}
	return ParseRoleIDs(raw)
}

func CategoryStaffRoles(cat *TicketCategory, fallback []string) []string {
	if cat.StaffRoles != "" {
		return ParseRoleIDs(cat.StaffRoles)
	}
	return fallback
}

func ParseRoleIDs(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if IsSnowflake(p) {
			ids = append(ids, p)
		}
	}
	return ids
}

// IsSnowflake returns true if s looks like a valid Discord snowflake ID
// (non-empty, all digits, at least 15 characters).
func IsSnowflake(s string) bool {
	if len(s) < 15 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func EffectiveTicketCategory(cfg *Config, gs *GuildState) string {
	if gs.TicketRuntime.DiscordCategoryOverride != "" {
		return gs.TicketRuntime.DiscordCategoryOverride
	}
	return cfg.Tickets.DiscordCategory
}

func EffectiveModLogChannel(cfg *Config, gs *GuildState) string {
	if gs.ModLogChannelOverride != "" {
		return gs.ModLogChannelOverride
	}
	return cfg.Moderation.ModLogChannel
}

// ──────────────────────────────────────────
// Commissions helpers
// ──────────────────────────────────────────

func EffectiveCommissionPanelChannel(cfg *Config, gs *GuildState) string {
	if IsSnowflake(gs.CommissionsRuntime.PanelChannelOverride) {
		return gs.CommissionsRuntime.PanelChannelOverride
	}
	if IsSnowflake(cfg.Commissions.PanelChannel) {
		return cfg.Commissions.PanelChannel
	}
	return ""
}

func EffectiveCommissionLogChannel(cfg *Config, gs *GuildState) string {
	if IsSnowflake(gs.CommissionsRuntime.LogChannelOverride) {
		return gs.CommissionsRuntime.LogChannelOverride
	}
	if IsSnowflake(cfg.Commissions.LogChannel) {
		return cfg.Commissions.LogChannel
	}
	return ""
}

func EffectiveCommissionStaffRoles(cfg *Config, gs *GuildState) []string {
	raw := cfg.Commissions.StaffRoles
	if gs.CommissionsRuntime.StaffRolesOverride != "" {
		raw = gs.CommissionsRuntime.StaffRolesOverride
	}
	return ParseRoleIDs(raw)
}

func EffectiveCommissionCategory(cfg *Config, gs *GuildState) string {
	if IsSnowflake(gs.CommissionsRuntime.DiscordCategoryOverride) {
		return gs.CommissionsRuntime.DiscordCategoryOverride
	}
	if IsSnowflake(cfg.Commissions.DiscordCategory) {
		return cfg.Commissions.DiscordCategory
	}
	return "" // no category — channel will be created without a parent
}

func EffectiveCommissionPayPalEmail(cfg *Config, gs *GuildState) string {
	if gs.CommissionsRuntime.PayPalEmail != "" {
		return gs.CommissionsRuntime.PayPalEmail
	}
	return cfg.Commissions.PayPalEmail
}

func EffectiveCommissionPayPalMe(cfg *Config, gs *GuildState) string {
	if gs.CommissionsRuntime.PayPalMeUser != "" {
		return gs.CommissionsRuntime.PayPalMeUser
	}
	return cfg.Commissions.PayPalMeUser
}

// MergedCommissionServices merges config-file services with runtime-added ones.
func MergedCommissionServices(cfg *Config, gs *GuildState) []CommissionService {
	all := make([]CommissionService, 0, len(cfg.Commissions.Services)+len(gs.CommissionsRuntime.Services))
	all = append(all, cfg.Commissions.Services...)
	all = append(all, gs.CommissionsRuntime.Services...)
	return all
}
