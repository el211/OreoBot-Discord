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
