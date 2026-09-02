package handlers

import (
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"discord-bot/config"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

// urlRe matches explicit http(s):// links as well as bare domains like
// "example.com/path" and "www.example.com". It intentionally errs towards
// catching things: the whitelist decides what is actually allowed.
var urlRe = regexp.MustCompile(`(?i)\b((?:https?://)?(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,24})(?::\d{1,5})?(?:/[^\s]*)?`)

// inviteRe matches Discord invite links specifically.
var inviteRe = regexp.MustCompile(`(?i)(?:discord\.gg|discord(?:app)?\.com/invite|discord\.com/invite|discord\.gg)/\S+`)

func linkfilterCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:                     "linkfilter",
			Description:              "Manage the link/invite filter",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "adddomain",
					Description: "Whitelist a domain so anyone can post links to it (e.g. github.com)",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "domain", Description: "Domain to allow (e.g. github.com)", Required: true},
					},
				},
				{
					Name:        "removedomain",
					Description: "Remove a domain from the whitelist",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "domain", Description: "Domain to remove", Required: true},
					},
				},
				{
					Name:        "addrole",
					Description: "Allow a role to post any link",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionRole, Name: "role", Description: "Role allowed to post links", Required: true},
					},
				},
				{
					Name:        "removerole",
					Description: "Stop allowing a role to post links",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionRole, Name: "role", Description: "Role to remove", Required: true},
					},
				},
				{
					Name:        "message",
					Description: "Set the warning message shown when a link is removed ({user} = mention)",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "text", Description: "Warning text. Use {user} for the mention. Empty resets to default.", Required: false},
					},
				},
				{
					Name:        "list",
					Description: "Show the current link-filter whitelist and allowed roles",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
	}
}

func (h *Handler) RegisterLinkFilter(s *discordgo.Session) {
	if !h.cfg.LinkFilter.Enabled {
		return
	}
	s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		h.handleLinkFilterMessage(s, m)
	})
	slog.Info("link filter active",
		"allowed_roles", len(h.cfg.LinkFilter.AllowedRoles),
		"whitelist_domains", len(h.cfg.LinkFilter.WhitelistDomains))
}

func (h *Handler) handleLinkFilterMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot || m.GuildID == "" || m.Content == "" {
		return
	}
