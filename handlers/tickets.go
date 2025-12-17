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