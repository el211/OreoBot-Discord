package handlers

import (
	"log/slog"
	"strconv"
	"strings"

	"discord-bot/config"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) Commands() []*discordgo.ApplicationCommand {
	cmds := make([]*discordgo.ApplicationCommand, 0)
	cmds = append(cmds, moderationCommands()...)
	cmds = append(cmds, ticketCommands()...)
	cmds = append(cmds, commissionCommands()...)
	cmds = append(cmds, utilityCommands()...)
	cmds = append(cmds, autoroleCommands()...)
	cmds = append(cmds, giveawayCommands()...)
	cmds = append(cmds, inviteCommands()...)
	cmds = append(cmds, nopingCommands()...)
	cmds = append(cmds, linkfilterCommands()...)
	cmds = append(cmds, verifyCommands(h.cfg)...)
	if h.cfg.Minecraft.Enabled {
		cmds = append(cmds, minecraftCommands()...)
	}
	if h.cfg.Music.Enabled {
		cmds = append(cmds, musicCommands()...)
	}
	cmds = append(cmds, githubCommands()...)
	cmds = append(cmds, buildCustomCommands(h.cfg)...)
	return cmds
}

func buildCustomCommands(cfg *config.Config) []*discordgo.ApplicationCommand {
	cmds := make([]*discordgo.ApplicationCommand, 0, len(cfg.CustomCommands))
	for _, cc := range cfg.CustomCommands {
		if cc.Name == "" || cc.Message == "" {
			continue
		}
		desc := cc.Description
		if desc == "" {
			desc = cc.Name
		}
		cmds = append(cmds, &discordgo.ApplicationCommand{
			Name:        strings.ToLower(cc.Name),
			Description: desc,
		})
	}
	return cmds
}

func (h *Handler) Register(s *discordgo.Session) {
	s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		handleCommissionMessageCreate(s, m)
	})

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.GuildID == "" {
			return
		}

		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			h.handleSlashCommand(s, i)
		case discordgo.InteractionMessageComponent:
			handleComponent(s, i)
		case discordgo.InteractionApplicationCommandAutocomplete:
			h.handleAutocomplete(s, i)
		case discordgo.InteractionModalSubmit:
			handleModal(s, i)
		}
	})
}

func (h *Handler) RegisterCustomCommands() {
	h.customCmdsMu.Lock()
	defer h.customCmdsMu.Unlock()
	h.customCmds = make(map[string]config.CustomCommandConfig, len(h.cfg.CustomCommands))
	for _, cc := range h.cfg.CustomCommands {
		if cc.Name != "" && cc.Message != "" {
			h.customCmds[strings.ToLower(cc.Name)] = cc
		}
	}
	slog.Info("custom commands registered", "count", len(h.customCmds))
}

func (h *Handler) handleSlashCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	name := i.ApplicationCommandData().Name

	switch name {
	case "ban":
		handleBan(s, i)
	case "unban":
		handleUnban(s, i)
	case "kick":
		handleKick(s, i)