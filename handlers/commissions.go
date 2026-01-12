package handlers

import (
	"fmt"
	"strings"

	"discord-bot/config"
	"discord-bot/payments"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

func commissionCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:                     "commission",
			Description:              "Commissions system management",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name: "setup", Description: "Configure the commissions system",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel for the commissions panel", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "paypal-email", Description: "PayPal email address for invoices", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "paypal-me", Description: "PayPal.me username (without paypal.me/)", Required: false},
						{
							Type: discordgo.ApplicationCommandOptionChannel, Name: "category",
							Description:  "Discord CATEGORY for commission channels (must be a category, not a text channel)",
							Required:     false,
							ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildCategory},
						},
						{Type: discordgo.ApplicationCommandOptionChannel, Name: "log-channel", Description: "Channel for commission logs", Required: false},
						{Type: discordgo.ApplicationCommandOptionString, Name: "staff-roles", Description: "Staff role IDs, comma-separated", Required: false},
					},
				},
				{
					Name: "toggle", Description: "Enable or disable the commissions system",
					Type: discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name: "addservice", Description: "Add a commission service type",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "id", Description: "Short identifier (e.g. plugin)", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "Display name (e.g. Plugin Development)", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "emoji", Description: "Emoji (e.g. 🔌)", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "description", Description: "Short description", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "starting-price", Description: "Starting price info (e.g. Starting at $20)", Required: false},
					},
				},
				{
					Name: "removeservice", Description: "Remove a commission service type",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "id", Description: "Service ID to remove", Required: true},
					},
				},
				{
					Name: "panel", Description: "Send or refresh the commissions panel",
					Type: discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name: "list", Description: "List all open commissions",
					Type: discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name: "config", Description: "Show the current commissions configuration",
					Type: discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name: "setcategory", Description: "Set the commission category by pasting its ID (right-click category → Copy ID)",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "id", Description: "Category snowflake ID", Required: true},
					},
				},
				{
					Name: "setlogchannel", Description: "Set the commission log channel by pasting its ID",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "id", Description: "Log channel snowflake ID", Required: true},
					},
				},
			},
		},
		{
			Name:                     "invoice",
			Description:              "Create a payment invoice for a commission",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name: "create", Description: "Generate a PayPal invoice for a client",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionUser, Name: "client", Description: "The client to invoice", Required: true},
						{Type: discordgo.ApplicationCommandOptionNumber, Name: "amount", Description: "Invoice total (e.g. 50.00)", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "description", Description: "Service / work description", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "currency", Description: "Currency code — leave blank to use your configured default", Required: false, Autocomplete: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "note", Description: "Additional note (e.g. Due in 7 days)", Required: false},
					},
				},
				{
					Name:        "list",
					Description: "List all invoices for this server",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
	}
}

func handleCommissionCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	sub := i.ApplicationCommandData().Options[0]
	switch sub.Name {
	case "setup":
		handleCommissionSetup(s, i, sub.Options)
	case "toggle":
		handleCommissionToggle(s, i)
	case "addservice":
		handleCommissionAddService(s, i, sub.Options)
	case "removeservice":
		handleCommissionRemoveService(s, i, sub.Options)
	case "panel":
		handleCommissionPanel(s, i)
	case "list":
		handleCommissionList(s, i)
	case "config":
		handleCommissionConfig(s, i)
	case "setcategory":
		handleCommissionSetCategory(s, i, sub.Options)
	case "setlogchannel":
		handleCommissionSetLogChannel(s, i, sub.Options)
	}
}

func handleInvoiceCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	sub := i.ApplicationCommandData().Options[0]
	switch sub.Name {
	case "create":
		handleInvoiceCreate(s, i, sub.Options)
	case "list":
		handleInvoiceList(s, i)
	}
}

func handleCommissionSetup(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	gs := storage.GetGuild(i.GuildID)

	gs.Lock()
	gs.CommissionsRuntime.PanelChannelOverride = om["channel"].ChannelValue(s).ID
	gs.CommissionsRuntime.PayPalEmail = om["paypal-email"].StringValue()
	if v, ok := om["paypal-me"]; ok {
		gs.CommissionsRuntime.PayPalMeUser = strings.TrimPrefix(v.StringValue(), "paypal.me/")
	}
	if v, ok := om["category"]; ok {
		gs.CommissionsRuntime.DiscordCategoryOverride = v.ChannelValue(s).ID
	}
	if v, ok := om["log-channel"]; ok {
		gs.CommissionsRuntime.LogChannelOverride = v.ChannelValue(s).ID
	}
	if v, ok := om["staff-roles"]; ok {
		gs.CommissionsRuntime.StaffRolesOverride = v.StringValue()
	}
	gs.CommissionsRuntime.Enabled = true
	gs.Unlock()
	_ = gs.Save()

	respond(s, i, "✅ Commissions system configured and **enabled**. Use `/commission addservice` to add service types, then `/commission panel` to post the panel.", true)
}

func handleCommissionToggle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	gs.CommissionsRuntime.Enabled = !gs.CommissionsRuntime.Enabled
	enabled := gs.CommissionsRuntime.Enabled
	gs.Unlock()
	_ = gs.Save()

	if enabled {
		respond(s, i, "✅ Commissions are now **open**. The panel button will allow new orders.", true)
	} else {
		respond(s, i, "🔒 Commissions are now **closed**. The panel button will reject new orders.", true)
	}
}

func handleCommissionAddService(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	gs := storage.GetGuild(i.GuildID)

	svc := config.CommissionService{
		ID:          om["id"].StringValue(),
		Name:        om["name"].StringValue(),
		Emoji:       om["emoji"].StringValue(),
		Description: om["description"].StringValue(),
	}
	if v, ok := om["starting-price"]; ok {
		svc.StartingPrice = v.StringValue()
	}

	gs.Lock()
	replaced := false
	for idx, existing := range gs.CommissionsRuntime.Services {
		if existing.ID == svc.ID {
			gs.CommissionsRuntime.Services[idx] = svc
			replaced = true
			break
		}
	}
	if !replaced {
		gs.CommissionsRuntime.Services = append(gs.CommissionsRuntime.Services, svc)
	}
	gs.Unlock()
	_ = gs.Save()

	verb := "added"
	if replaced {
		verb = "updated"
	}
	respond(s, i, fmt.Sprintf("✅ Service %s **%s** %s.", svc.Emoji, svc.Name, verb), true)
}

func handleCommissionRemoveService(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	id := om["id"].StringValue()
	gs := storage.GetGuild(i.GuildID)

	gs.Lock()
	found := false
	svcs := gs.CommissionsRuntime.Services
	for idx, svc := range svcs {
		if svc.ID == id {
			gs.CommissionsRuntime.Services = append(svcs[:idx], svcs[idx+1:]...)
			found = true
			break
		}
	}
	gs.Unlock()
	_ = gs.Save()

	if !found {
		respond(s, i, fmt.Sprintf("❌ No runtime service with ID `%s` found. Config-file services cannot be removed at runtime.", id), true)
		return
	}
	respond(s, i, fmt.Sprintf("🗑️ Service `%s` removed.", id), true)
}

func handleCommissionPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cfg := storage.Cfg
	gs := storage.GetGuild(i.GuildID)

	panelCh := config.EffectiveCommissionPanelChannel(cfg, gs)
	if panelCh == "" {
		respond(s, i, "❌ No panel channel configured. Run `/commission setup` first.", true)
		return
	}

	services := config.MergedCommissionServices(cfg, gs)

	gs.Lock()
	enabled := gs.CommissionsRuntime.Enabled
	oldMsgID := gs.CommissionsRuntime.PanelMessageID
	gs.Unlock()

	embed := buildCommissionPanelEmbed(gs, services, enabled)

	statusLabel := "Order Here"
	statusStyle := discordgo.SuccessButton
	if !enabled {
		statusLabel = "Commissions Closed"
		statusStyle = discordgo.DangerButton
	}

	msg, err := s.ChannelMessageSendComplex(panelCh, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    statusLabel,
						Style:    statusStyle,
						CustomID: "commission_order",
						Emoji:    &discordgo.ComponentEmoji{Name: "📋"},
					},
				},
			},
		},
	})
	if err != nil {
		respond(s, i, fmt.Sprintf("❌ Failed to send panel: %s", err.Error()), true)
		return
	}

	gs.Lock()
	gs.CommissionsRuntime.PanelMessageID = msg.ID
	gs.Unlock()
	_ = gs.Save()

	if oldMsgID != "" {
		_ = s.ChannelMessageDelete(panelCh, oldMsgID)
	}

	respond(s, i, "✅ Commissions panel posted.", true)
}

func buildCommissionPanelEmbed(gs *config.GuildState, services []config.CommissionService, enabled bool) *discordgo.MessageEmbed {
	cfg := storage.Cfg

	statusLine := "🟢 **Status: Open** — Accepting new orders!"
	if !enabled {
		statusLine = "🔴 **Status: Closed** — Not accepting orders at this time."
	}

	var desc strings.Builder
	desc.WriteString(statusLine + "\n\n")

	if len(services) == 0 {
		desc.WriteString("*No services listed yet.*\n")
	} else {
		desc.WriteString("**Available Services**\n")
		desc.WriteString("━━━━━━━━━━━━━━━━━━━━━━\n\n")
		for _, svc := range services {
			desc.WriteString(fmt.Sprintf("%s **%s**\n", svc.Emoji, svc.Name))
			desc.WriteString(fmt.Sprintf("┃ %s\n", svc.Description))
			if svc.StartingPrice != "" {
				desc.WriteString(fmt.Sprintf("┃ 💰 %s\n", svc.StartingPrice))
			}
			desc.WriteString("\n")
		}
		desc.WriteString("━━━━━━━━━━━━━━━━━━━━━━\n\n")
	}

	desc.WriteString("> 📬 Click **Order Here** to open a private commission ticket.\n")
	desc.WriteString("> You will be asked to fill out a short order form.")

	embed := &discordgo.MessageEmbed{
		Title:       "📋 Commissions",
		Description: desc.String(),
		Color:       0x5865F2,
	}
	if footer := payments.FooterText(cfg, gs); footer != "" {
		embed.Footer = &discordgo.MessageEmbedFooter{Text: footer}
	}
	return embed
}

func handleCommissionList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	count := len(gs.CommissionsRuntime.OpenCommissions)
	var sb strings.Builder
	if count > 0 {
		sb.WriteString(fmt.Sprintf("**Open Commissions** (%d):\n", count))
		for _, ct := range gs.CommissionsRuntime.OpenCommissions {
			sb.WriteString(fmt.Sprintf("• <#%s> — #%d by <@%s> [%s]\n", ct.ChannelID, ct.Number, ct.UserID, ct.ServiceName))
		}
	}
	gs.Unlock()

	if count == 0 {
		respond(s, i, "📭 No open commissions.", true)
		return
	}
	respond(s, i, sb.String(), true)
}

func handleCommissionConfig(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cfg := storage.Cfg
	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	cr := gs.CommissionsRuntime
	gs.Unlock()

	services := config.MergedCommissionServices(cfg, gs)

	var sb strings.Builder
	sb.WriteString("**Commissions System Configuration**\n\n")
	sb.WriteString(fmt.Sprintf("Enabled: `%v`\n", cr.Enabled))
	sb.WriteString(fmt.Sprintf("Panel Channel: `%s`\n", cr.PanelChannelOverride))
	sb.WriteString(fmt.Sprintf("Log Channel: `%s`\n", cr.LogChannelOverride))
	sb.WriteString(fmt.Sprintf("Staff Roles: `%s`\n", cr.StaffRolesOverride))
	sb.WriteString(fmt.Sprintf("Discord Category: `%s`\n", cr.DiscordCategoryOverride))
	sb.WriteString(fmt.Sprintf("PayPal Email: `%s`\n", cr.PayPalEmail))
	sb.WriteString(fmt.Sprintf("PayPal.me User: `%s`\n", cr.PayPalMeUser))
	sb.WriteString(fmt.Sprintf("Open Commissions: `%d`\n", len(cr.OpenCommissions)))
	sb.WriteString(fmt.Sprintf("Total Invoices: `%d`\n\n", len(cr.Invoices)))
	sb.WriteString("__Services:__\n")
	for _, svc := range services {
		price := ""
		if svc.StartingPrice != "" {