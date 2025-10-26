package handlers

import (
	"log/slog"
	"strconv"
	"strings"

	"discord-bot/config"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) RegisterWelcomeLeave(s *discordgo.Session) {
	if h.cfg.Welcome.Enabled {
		s.AddHandler(func(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
			h.handleWelcome(s, m)
		})
	}

	if h.cfg.Leave.Enabled {
		s.AddHandler(func(s *discordgo.Session, m *discordgo.GuildMemberRemove) {
			h.handleLeave(s, m)
		})
	}

	s.AddHandler(func(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
		AssignJoinRole(s, m.GuildID, m.User.ID)
	})
}

func (h *Handler) handleWelcome(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	if h.cfg.Welcome.ChannelID == "" || h.cfg.Welcome.ChannelID == "PUT_WELCOME_CHANNEL_ID_HERE" {
		return
	}

	embed := buildWelcomeLeaveEmbed(s, &h.cfg.Welcome, m.User, m.GuildID)
	if _, err := s.ChannelMessageSendEmbed(h.cfg.Welcome.ChannelID, embed); err != nil {
		slog.Error("welcome send failed", "error", err)
	}
}

func (h *Handler) handleLeave(s *discordgo.Session, m *discordgo.GuildMemberRemove) {
	if h.cfg.Leave.ChannelID == "" || h.cfg.Leave.ChannelID == "PUT_LEAVE_CHANNEL_ID_HERE" {
		return
	}

	embed := buildWelcomeLeaveEmbed(s, &h.cfg.Leave, m.User, m.GuildID)
	if _, err := s.ChannelMessageSendEmbed(h.cfg.Leave.ChannelID, embed); err != nil {
		slog.Error("leave send failed", "error", err)
	}
}

func buildWelcomeLeaveEmbed(s *discordgo.Session, cfg *config.WelcomeLeaveConfig, user *discordgo.User, guildID string) *discordgo.MessageEmbed {
	msg := cfg.Embed.Message
	msg = strings.ReplaceAll(msg, "%joined_user%", user.Mention())
	msg = strings.ReplaceAll(msg, "%username%", user.Username)

	title := cfg.Embed.Title
	title = strings.ReplaceAll(title, "%joined_user%", user.Username)
	title = strings.ReplaceAll(title, "%username%", user.Username)

	colour := 0
	hex := strings.TrimPrefix(cfg.Embed.Colour, "#")
	if v, err := strconv.ParseInt(hex, 16, 64); err == nil {
		colour = int(v)
	}

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: msg,
		Color:       colour,
	}

	if cfg.Embed.Thumbnail != "" {
		thumbURL := cfg.Embed.Thumbnail
		if strings.EqualFold(thumbURL, "BOT") {
			if s.State != nil && s.State.User != nil {
				thumbURL = s.State.User.AvatarURL("256")
			} else {
				thumbURL = ""
			}
		} else if strings.EqualFold(thumbURL, "USER") {
			thumbURL = user.AvatarURL("256")
		}
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: thumbURL}
	}

	if cfg.Embed.ImageEnabled && cfg.Embed.ImageURL != "" {
		embed.Image = &discordgo.MessageEmbedImage{URL: cfg.Embed.ImageURL}
	}

	_ = guildID
	return embed
}
