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