package handlers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"discord-bot/config"
	"discord-bot/lang"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

var modPermission int64 = discordgo.PermissionBanMembers
var adminPerm int64 = discordgo.PermissionAdministrator

func moderationCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:                     "ban",
			Description:              "Ban a member from the server",
			DefaultMemberPermissions: &modPermission,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to ban", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Reason for ban"},
				{Type: discordgo.ApplicationCommandOptionInteger, Name: "days", Description: "Days of messages to delete (0-7)"},
			},
		},
		{
			Name:                     "unban",
			Description:              "Unban a user from the server",
			DefaultMemberPermissions: &modPermission,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "user-id", Description: "User ID to unban", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Reason for unban"},
			},
		},
		{
			Name:                     "kick",
			Description:              "Kick a member from the server",
			DefaultMemberPermissions: &modPermission,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to kick", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Reason for kick"},
			},
		},
		{
			Name:                     "mute",
			Description:              "Timeout (mute) a member",
			DefaultMemberPermissions: &modPermission,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to mute", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "duration", Description: "Duration (e.g. 10m, 1h, 1d)", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Reason for mute"},
			},
		},
		{
			Name:                     "unmute",
			Description:              "Remove timeout from a member",
			DefaultMemberPermissions: &modPermission,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to unmute", Required: true},
			},
		},
		{
			Name:                     "warn",
			Description:              "Issue a warning to a member",
			DefaultMemberPermissions: &modPermission,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to warn", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Reason for warning", Required: true},
			},
		},
		{
			Name:                     "warnings",
			Description:              "View warnings for a member",
			DefaultMemberPermissions: &modPermission,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to check", Required: true},
			},
		},
		{
			Name:                     "clearwarnings",
			Description:              "Clear all warnings for a member",
			DefaultMemberPermissions: &modPermission,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to clear warnings for", Required: true},
			},
		},
		{
			Name:                     "purge",
			Description:              "Delete a number of messages from the channel",
			DefaultMemberPermissions: &modPermission,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionInteger, Name: "count", Description: "Number of messages to delete (1-100)", Required: true},
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "Only delete messages from this user"},
			},
		},
		{
			Name:                     "clear",
			Description:              "Clear a number of messages from the channel",
			DefaultMemberPermissions: &modPermission,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionInteger, Name: "count", Description: "Number of messages to delete (1-100)", Required: true},
			},
		},
		{
			Name:                     "slowmode",
			Description:              "Set slowmode delay for the current channel",
			DefaultMemberPermissions: &modPermission,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionInteger, Name: "seconds", Description: "Slowmode delay in seconds (0 to disable)", Required: true},
			},
		},
		{
			Name:                     "lock",
			Description:              "Lock the current channel (prevent @everyone from sending messages)",
			DefaultMemberPermissions: &modPermission,
		},
		{
			Name:                     "unlock",
			Description:              "Unlock the current channel",
			DefaultMemberPermissions: &modPermission,
		},
		{
			Name:                     "modlog",
			Description:              "Set the moderation log channel",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel for mod logs", Required: true},
			},
		},
		{
			Name:                     "userinfo",
			Description:              "Show information about a user",
			DefaultMemberPermissions: &modPermission,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to inspect"},
			},
		},
	}
}

func handleBan(s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := optionMap(i)
	target := opts["user"].UserValue(s)
	reason := optStr(opts, "reason", "No reason provided")
	days := int(optInt(opts, "days", 0))
	if days > 7 {
		days = 7
	}

	err := s.GuildBanCreateWithReason(i.GuildID, target.ID, reason, days)
	if err != nil {
		respond(s, i, lang.T("mod_ban_failed", "error", err.Error()), true)
		return
	}

	respond(s, i, lang.T("mod_ban_success", "user", target.Username, "reason", reason), false)
	logModAction(s, i.GuildID, "Ban", target, i.Member.User, reason, "")
}

func handleUnban(s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := optionMap(i)
	userID := opts["user-id"].StringValue()
	reason := optStr(opts, "reason", "No reason provided")

	err := s.GuildBanDelete(i.GuildID, userID)
	if err != nil {
		respond(s, i, lang.T("mod_unban_failed", "error", err.Error()), true)
		return
	}

	respond(s, i, lang.T("mod_unban_success", "user_id", userID, "reason", reason), false)
}

func handleKick(s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := optionMap(i)
	target := opts["user"].UserValue(s)
	reason := optStr(opts, "reason", "No reason provided")

	err := s.GuildMemberDeleteWithReason(i.GuildID, target.ID, reason)
	if err != nil {
		respond(s, i, lang.T("mod_kick_failed", "error", err.Error()), true)
		return
	}

	respond(s, i, lang.T("mod_kick_success", "user", target.Username, "reason", reason), false)
	logModAction(s, i.GuildID, "Kick", target, i.Member.User, reason, "")
}

func handleMute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := optionMap(i)
	target := opts["user"].UserValue(s)