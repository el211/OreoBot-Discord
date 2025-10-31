package handlers

import (
	"fmt"
	"log/slog"
	"strings"

	"discord-bot/config"
	"discord-bot/lang"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

func autoroleCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:                     "joinrole",
			Description:              "Configure the role automatically given to new members",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "set",
					Description: "Set the role given on join",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionRole, Name: "role", Description: "Role to assign when someone joins", Required: true},
					},
				},
				{
					Name:        "disable",
					Description: "Disable auto-role on join",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "status",
					Description: "Show the current join role configuration",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "check",
					Description: "Check if the bot can assign the auto-role (permission check)",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
		{
			Name:                     "rolemenu",
			Description:              "Create self-assignable role menus for members",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "create",
					Description: "Create a new role menu",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "title", Description: "Menu title (e.g. 'Choose your gender')", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "description", Description: "Menu description"},
						{Type: discordgo.ApplicationCommandOptionBoolean, Name: "single", Description: "Only one role at a time — selecting one removes the others (default: false)"},
					},
				},
				{
					Name:        "add",
					Description: "Add a role button to an existing menu",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "menu_id", Description: "Menu ID (from /rolemenu list)", Required: true},
						{Type: discordgo.ApplicationCommandOptionRole, Name: "role", Description: "Role to add", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "label", Description: "Button label — include emoji here if you want (e.g. 🇫🇷 Français)", Required: true},
					},
				},
				{
					Name:        "post",
					Description: "Post the role menu in a channel so members can use it",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "menu_id", Description: "Menu ID to post", Required: true},
						{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel to post the menu in", Required: true},
					},
				},
				{
					Name:        "list",
					Description: "List all role menus",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "delete",
					Description: "Delete a role menu",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "menu_id", Description: "Menu ID to delete", Required: true},
					},
				},
			},
		},
	}
}

func (h *Handler) handleJoinRoleCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.isAdmin(s, i) {
		respond(s, i, lang.T("no_permission"), true)
		return
	}
	sub := i.ApplicationCommandData().Options[0]
	gs := storage.GetGuild(i.GuildID)

	switch sub.Name {
	case "set":
		om := subOptMap(sub.Options)
		role := om["role"].RoleValue(s, i.GuildID)

		if warning := checkBotRoleHierarchy(s, i.GuildID, role); warning != "" {
			respond(s, i, lang.T("autorole_set_warning", "warning", warning), true)
		} else {
			respond(s, i, lang.T("autorole_set_success", "role_id", role.ID), true)
		}

		gs.Lock()
		gs.AutoRole = config.AutoRoleState{Enabled: true, RoleID: role.ID}
		gs.Unlock()
		_ = gs.Save()

	case "disable":
		gs.Lock()
		gs.AutoRole = config.AutoRoleState{Enabled: false}
		gs.Unlock()
		_ = gs.Save()
		respond(s, i, lang.T("autorole_disabled"), true)

	case "status":
		gs.Lock()
		ar := gs.AutoRole
		gs.Unlock()
		if ar.Enabled && ar.RoleID != "" {
			respond(s, i, lang.T("autorole_status_enabled", "role_id", ar.RoleID), true)
		} else {
			respond(s, i, lang.T("autorole_status_disabled"), true)
		}

	case "check":
		gs.Lock()
		ar := gs.AutoRole
		gs.Unlock()
		if !ar.Enabled || ar.RoleID == "" {
			respond(s, i, lang.T("autorole_hint_set"), true)
			return
		}
		role, err := s.State.Role(i.GuildID, ar.RoleID)
		if err != nil {
			respond(s, i, lang.T("autorole_role_deleted", "role_id", ar.RoleID), true)
			return
		}
		if w := checkBotRoleHierarchy(s, i.GuildID, role); w != "" {
			respond(s, i, lang.T("autorole_problem", "warning", w), true)
		} else {
			respond(s, i, lang.T("autorole_ok", "role_id", ar.RoleID), true)
		}
	}
}

func checkBotRoleHierarchy(s *discordgo.Session, guildID string, targetRole *discordgo.Role) string {
	if s.State == nil || s.State.User == nil {
		return lang.T("autorole_fetch_bot_failed", "error", "session not ready")
	}
	botID := s.State.User.ID

	botMember, err := s.GuildMember(guildID, botID)
	if err != nil {
		return lang.T("autorole_fetch_bot_failed", "error", err.Error())
	}

	allRoles, err := s.GuildRoles(guildID)
	if err != nil {
		return lang.T("autorole_fetch_roles_failed", "error", err.Error())
	}

	roleMap := make(map[string]*discordgo.Role, len(allRoles))
	for _, r := range allRoles {
		roleMap[r.ID] = r
	}

	botHighestPos := 0
	for _, rid := range botMember.Roles {
		if r, ok := roleMap[rid]; ok && r.Position > botHighestPos {
			botHighestPos = r.Position
		}
	}

	if targetRole.Position >= botHighestPos {
		return fmt.Sprintf(
			"<@&%s> (position %d) is **equal to or above** the bot's highest role (position %d).\n"+
				"👉 Move the bot's role **above** <@&%s> in Server Settings → Roles.",
			targetRole.ID, targetRole.Position,
			botHighestPos, targetRole.ID,
		)
	}