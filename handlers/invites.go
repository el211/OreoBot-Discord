package handlers

import (
	"fmt"
	"log/slog"
	"sync"

	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

var (
	inviteCache   = make(map[string]map[string]int)
	inviteCacheMu sync.Mutex
)

func (h *Handler) RegisterInviteTracker(s *discordgo.Session) {
	s.AddHandler(func(s *discordgo.Session, e *discordgo.GuildCreate) {
		cacheGuildInvites(s, e.Guild.ID)
	})

	s.AddHandler(func(s *discordgo.Session, e *discordgo.GuildMemberAdd) {
		trackNewMember(s, e.GuildID, e.User)
	})

	s.AddHandler(func(s *discordgo.Session, e *discordgo.InviteCreate) {
		cacheGuildInvites(s, e.GuildID)
	})

	s.AddHandler(func(s *discordgo.Session, e *discordgo.InviteDelete) {
		cacheGuildInvites(s, e.GuildID)
	})
}

func cacheGuildInvites(s *discordgo.Session, guildID string) {
	invites, err := s.GuildInvites(guildID)
	if err != nil {
		slog.Error("failed to fetch invites", "guild_id", guildID, "error", err)
		return
	}

	snapshot := make(map[string]int, len(invites))
	for _, inv := range invites {
		snapshot[inv.Code] = inv.Uses
	}

	inviteCacheMu.Lock()
	inviteCache[guildID] = snapshot
	inviteCacheMu.Unlock()
}

func trackNewMember(s *discordgo.Session, guildID string, user *discordgo.User) {
	if user.Bot {
		return
	}

	freshInvites, err := s.GuildInvites(guildID)
	if err != nil {
		slog.Error("failed to fetch invites on member join", "guild_id", guildID, "error", err)
		return
	}

	inviteCacheMu.Lock()
	old := inviteCache[guildID]
	if old == nil {
		old = make(map[string]int)
	}

	var inviterID string
	newSnapshot := make(map[string]int, len(freshInvites))
	for _, inv := range freshInvites {
		newSnapshot[inv.Code] = inv.Uses
		if inv.Uses > old[inv.Code] && inv.Inviter != nil {
			inviterID = inv.Inviter.ID
		}
	}
	inviteCache[guildID] = newSnapshot
	inviteCacheMu.Unlock()

	if inviterID == "" {
		return
	}

	gs := storage.GetGuild(guildID)
	gs.Lock()
	gs.InviteCounts[inviterID]++
	gs.Unlock()
	if err := gs.Save(); err != nil {
		slog.Error("failed to save invite count", "error", err)
	}
}

func inviteCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "invites",
			Description: "Check how many invites a member has",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "User to check (defaults to yourself)",
					Required:    false,
				},
			},
		},
		{
			Name:        "resetinvites",
			Description: "Reset a member's invite count to zero (admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "User whose invites to reset",
					Required:    true,
				},
			},
		},
	}
}

func (h *Handler) handleInvitesCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := optionMap(i)

	var targetUser *discordgo.User
	if o, ok := opts["user"]; ok {
		targetUser = o.UserValue(s)
	} else {
		targetUser = i.Member.User
	}

	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	count := gs.InviteCounts[targetUser.ID]
	gs.Unlock()

	embed := &discordgo.MessageEmbed{
		Title:       "Invite Tracker",
		Description: fmt.Sprintf("<@%s> has **%d** invite(s).", targetUser.ID, count),
		Color:       0x5865F2,
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: targetUser.AvatarURL("64")},
	}
	respondEmbed(s, i, embed, false)
}

func (h *Handler) handleResetInvitesCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.isAdmin(s, i) {
		respond(s, i, "You need administrator permissions to reset invites.", true)
		return
	}

	opts := optionMap(i)
	targetUser := opts["user"].UserValue(s)

	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	gs.InviteCounts[targetUser.ID] = 0
	gs.Unlock()
	if err := gs.Save(); err != nil {
		respond(s, i, "Failed to save: "+err.Error(), true)
		return
	}

	respond(s, i, fmt.Sprintf("Reset invites for <@%s> to **0**.", targetUser.ID), false)
}
