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