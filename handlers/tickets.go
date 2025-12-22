package handlers

import (
	"fmt"
	"strings"
	"time"

	"discord-bot/config"
	"discord-bot/lang"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

func ticketCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:                     "ticket",
			Description:              "Ticket system management",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name: "setup", Description: "Set up or update the ticket system (overrides config.json values)",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel to post the ticket panel in", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "staff-roles", Description: "Staff role ID(s), comma-separated", Required: true},
						{Type: discordgo.ApplicationCommandOptionChannel, Name: "log-channel", Description: "Channel for ticket logs"},
						{Type: discordgo.ApplicationCommandOptionChannel, Name: "category", Description: "Discord category for ticket channels"},
					},
				},
				{
					Name: "addcategory", Description: "Add a ticket category (in addition to config.json ones)",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "id", Description: "Short identifier (e.g. sales)", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "Display name", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "emoji", Description: "Emoji (e.g. 🎫)", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "description", Description: "Short description", Required: true},
					},
				},
				{
					Name: "removecategory", Description: "Remove a runtime-added ticket category",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "id", Description: "Category ID to remove", Required: true},
					},
				},
				{
					Name: "addsubcategory", Description: "Add a subcategory under an existing category",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "category-id", Description: "Parent category ID", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "id", Description: "Subcategory ID", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "Display name", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "emoji", Description: "Emoji", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "description", Description: "Short description", Required: true},
					},
				},
				{
					Name: "removesubcategory", Description: "Remove a subcategory",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "category-id", Description: "Parent category ID", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "id", Description: "Subcategory ID to remove", Required: true},
					},
				},
				{
					Name: "panel", Description: "Send or refresh the ticket panel",
					Type: discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name: "list", Description: "List all open tickets",
					Type: discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name: "config", Description: "Show the current ticket configuration",
					Type: discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
		{Name: "close", Description: "Close the current ticket"},
		{
			Name: "add", Description: "Add a user to the current ticket",
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to add", Required: true},
			},
		},
		{
			Name: "remove", Description: "Remove a user from the current ticket",
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to remove", Required: true},
			},
		},
	}
}

func handleTicketCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	sub := i.ApplicationCommandData().Options[0]
	switch sub.Name {
	case "setup":
		handleTicketSetup(s, i, sub.Options)
	case "addcategory":
		handleTicketAddCategory(s, i, sub.Options)
	case "removecategory":
		handleTicketRemoveCategory(s, i, sub.Options)
	case "addsubcategory":
		handleTicketAddSubcategory(s, i, sub.Options)
	case "removesubcategory":
		handleTicketRemoveSubcategory(s, i, sub.Options)
	case "panel":
		handleTicketPanel(s, i)
	case "list":
		handleTicketList(s, i)
	case "config":
		handleTicketConfigCmd(s, i)
	}
}

func handleTicketSetup(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	gs := storage.GetGuild(i.GuildID)

	gs.Lock()
	gs.TicketRuntime.PanelChannelOverride = om["channel"].ChannelValue(s).ID
	gs.TicketRuntime.StaffRolesOverride = om["staff-roles"].StringValue()
	if lc, ok := om["log-channel"]; ok {
		gs.TicketRuntime.LogChannelOverride = lc.ChannelValue(s).ID
	}
	if cat, ok := om["category"]; ok {
		gs.TicketRuntime.DiscordCategoryOverride = cat.ChannelValue(s).ID
	}
	gs.Unlock()
	_ = gs.Save()

	respond(s, i, lang.T("ticket_setup_done"), true)
}

func handleTicketAddCategory(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	gs := storage.GetGuild(i.GuildID)

	cat := config.TicketCategory{
		ID:          om["id"].StringValue(),
		Name:        om["name"].StringValue(),
		Emoji:       om["emoji"].StringValue(),
		Description: om["description"].StringValue(),
	}

	gs.Lock()
	gs.TicketRuntime.ExtraCategories = append(gs.TicketRuntime.ExtraCategories, cat)
	gs.Unlock()
	_ = gs.Save()

	respond(s, i, lang.T("ticket_category_added", "emoji", cat.Emoji, "name", cat.Name), true)
}

func handleTicketRemoveCategory(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	id := om["id"].StringValue()
	gs := storage.GetGuild(i.GuildID)

	gs.Lock()
	found := false
	extras := gs.TicketRuntime.ExtraCategories
	for idx, c := range extras {
		if c.ID == id {
			gs.TicketRuntime.ExtraCategories = append(extras[:idx], extras[idx+1:]...)
			found = true
			break
		}
	}
	gs.Unlock()
	_ = gs.Save()

	if !found {
		respond(s, i, lang.T("ticket_category_not_found_runtime", "id", id), true)
		return
	}
	respond(s, i, lang.T("ticket_category_removed", "id", id), true)
}

func handleTicketAddSubcategory(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	catID := om["category-id"].StringValue()
	gs := storage.GetGuild(i.GuildID)

	sub := config.TicketSubcategory{
		ID:          om["id"].StringValue(),
		Name:        om["name"].StringValue(),
		Emoji:       om["emoji"].StringValue(),
		Description: om["description"].StringValue(),
	}

	gs.Lock()
	found := false
	for idx := range gs.TicketRuntime.ExtraCategories {
		if gs.TicketRuntime.ExtraCategories[idx].ID == catID {
			gs.TicketRuntime.ExtraCategories[idx].Subcategories = append(gs.TicketRuntime.ExtraCategories[idx].Subcategories, sub)
			found = true
			break
		}
	}
	gs.Unlock()

	if !found {
		for _, c := range storage.Cfg.Tickets.Categories {
			if c.ID == catID {
				respond(s, i, lang.T("ticket_category_config_only", "id", catID), true)
				return
			}
		}
		respond(s, i, lang.T("ticket_category_not_found", "id", catID), true)
		return
	}

	_ = gs.Save()
	respond(s, i, lang.T("ticket_subcategory_added", "emoji", sub.Emoji, "name", sub.Name, "parent", catID), true)
}

func handleTicketRemoveSubcategory(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	catID := om["category-id"].StringValue()
	subID := om["id"].StringValue()
	gs := storage.GetGuild(i.GuildID)

	gs.Lock()
	found := false
	for ci := range gs.TicketRuntime.ExtraCategories {
		if gs.TicketRuntime.ExtraCategories[ci].ID == catID {
			subs := gs.TicketRuntime.ExtraCategories[ci].Subcategories
			for si, sc := range subs {
				if sc.ID == subID {
					gs.TicketRuntime.ExtraCategories[ci].Subcategories = append(subs[:si], subs[si+1:]...)
					found = true
					break
				}
			}
			break
		}
	}
	gs.Unlock()
	_ = gs.Save()

	if !found {
		respond(s, i, lang.T("ticket_subcategory_not_found"), true)
		return
	}
	respond(s, i, lang.T("ticket_subcategory_removed", "id", subID), true)
}

func handleTicketPanel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cfg := storage.Cfg
	gs := storage.GetGuild(i.GuildID)

	panelCh := config.EffectiveTicketPanelChannel(cfg, gs)
	if panelCh == "" {
		respond(s, i, lang.T("ticket_no_panel_channel"), true)
		return
	}

	categories := config.MergedTicketCategories(cfg, gs)
	if len(categories) == 0 {
		respond(s, i, lang.T("ticket_no_categories"), true)
		return
	}

	var desc strings.Builder
	desc.WriteString(lang.T("ticket_panel_description") + "\n\n")
	for _, cat := range categories {
		desc.WriteString(fmt.Sprintf("%s **%s** — %s\n", cat.Emoji, cat.Name, cat.Description))
		for _, sub := range cat.Subcategories {
			desc.WriteString(fmt.Sprintf("   ↳ %s %s — %s\n", sub.Emoji, sub.Name, sub.Description))
		}
		desc.WriteString("\n")
	}

	embed := &discordgo.MessageEmbed{
		Title:       lang.T("ticket_panel_title"),
		Description: desc.String(),
		Color:       0x5865F2,
		Footer:      &discordgo.MessageEmbedFooter{Text: "Click the menu below to open a ticket"},
	}

	menuOpts := make([]discordgo.SelectMenuOption, 0, len(categories))
	for _, cat := range categories {
		menuOpts = append(menuOpts, discordgo.SelectMenuOption{
			Label:       cat.Name,
			Value:       cat.ID,
			Description: cat.Description,
			Emoji:       parseComponentEmoji(cat.Emoji),
		})
	}
