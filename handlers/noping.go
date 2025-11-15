package handlers

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"discord-bot/config"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

func nopingCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:                     "noping",
			Description:              "Manage who can ping protected users/roles",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "whitelist",
					Description: "Allow a user to ping protected users/roles",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to allow", Required: true},
					},
				},
				{
					Name:        "blacklist",
					Description: "Remove a user from the no-ping whitelist",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to block again", Required: true},
					},
				},
				{
					Name:        "list",
					Description: "List users currently allowed to ping protected users/roles",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
	}
}

func (h *Handler) RegisterNoPing(s *discordgo.Session) {
	if !h.cfg.NoPing.Enabled || len(h.cfg.NoPing.ProtectedRoles) == 0 {
		return
	}

	protected := make(map[string]bool, len(h.cfg.NoPing.ProtectedRoles))
	for _, id := range h.cfg.NoPing.ProtectedRoles {
		protected[strings.TrimSpace(id)] = true
	}

	bypass := make(map[string]bool, len(h.cfg.NoPing.BypassRoles))
	for _, id := range h.cfg.NoPing.BypassRoles {
		bypass[strings.TrimSpace(id)] = true
	}

	s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		handleNoPing(s, m, &h.cfg.NoPing, protected, bypass)
	})

	slog.Info("noping active", "protected_roles", len(protected), "bypass_roles", len(bypass))
}

func (h *Handler) handleNoPingCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.isAdmin(s, i) {
		respond(s, i, "You do not have permission to use this command.", true)
		return
	}

	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		respond(s, i, "Use `/noping whitelist`, `/noping blacklist`, or `/noping list`.", true)
		return
	}

	sub := data.Options[0]
	switch sub.Name {
	case "whitelist":
		handleNoPingWhitelist(s, i, sub.Options)
	case "blacklist":
		handleNoPingBlacklist(s, i, sub.Options)
	case "list":
		handleNoPingList(s, i)
	default:
		respond(s, i, "Unknown no-ping subcommand.", true)
	}
}

func handleNoPingWhitelist(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	target := om["user"].UserValue(s)