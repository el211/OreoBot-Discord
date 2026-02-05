package handlers

import (
	"fmt"
	"log/slog"
	"strings"

	"discord-bot/config"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

func handleCommissionMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot {
		return
	}
	gs, ct, ok, fromTicket := findCommissionTicketForMessage(m)
	if !ok {
		return
	}
	if fromTicket {
		handleCommissionClientMessage(s, gs, &ct, m)
		return
	}
	handleCommissionThreadReply(s, &ct, m)
}

func findCommissionTicketForMessage(m *discordgo.MessageCreate) (*config.GuildState, config.CommissionTicket, bool, bool) {
	if m.GuildID != "" {
		gs := storage.GetGuild(m.GuildID)
		if ct, ok, fromTicket := findCommissionTicketInGuild(gs, m.ChannelID); ok {
			return gs, ct, true, fromTicket
		}
	}
	for _, gs := range storage.GetAllGuildStates() {
		if ct, ok, fromTicket := findCommissionTicketInGuild(gs, m.ChannelID); ok {
			return gs, ct, true, fromTicket
		}
	}
	return nil, config.CommissionTicket{}, false, false
}

func findCommissionTicketInGuild(gs *config.GuildState, channelID string) (config.CommissionTicket, bool, bool) {
	gs.Lock()
	defer gs.Unlock()
	for ticketChannelID, ticket := range gs.CommissionsRuntime.OpenCommissions {
		if channelID == ticketChannelID {
			return ticket, true, true
		}
		if ticket.DiscussionThreadID != "" && channelID == ticket.DiscussionThreadID {
			return ticket, true, false
		}
	}
	return config.CommissionTicket{}, false, false
}

func handleCommissionClientMessage(s *discordgo.Session, gs *config.GuildState, ct *config.CommissionTicket, m *discordgo.MessageCreate) {
	if m.Author.ID != ct.UserID {
		return
	}
	if ct.DiscussionThreadID == "" {
		slog.Warn("commission client message missing discussion thread", "channel_id", ct.ChannelID)
		return
	}
	content := formatCommissionClientMirror(m)
	threadMsg, err := s.ChannelMessageSendComplex(ct.DiscussionThreadID, &discordgo.MessageSend{
		Content:         content,
		AllowedMentions: &discordgo.MessageAllowedMentions{},
	})
	if err != nil {
		slog.Error("commission failed to mirror client message", "message_id", m.ID, "thread_id", ct.DiscussionThreadID, "error", err)
		return
	}

	gs.Lock()
	stored := gs.CommissionsRuntime.OpenCommissions[ct.ChannelID]
	ensureCommissionTicketRuntime(&stored)
	stored.ClientThreadMessages[m.ID] = threadMsg.ID
	gs.CommissionsRuntime.OpenCommissions[ct.ChannelID] = stored
	gs.Unlock()
	_ = gs.Save()
}

func handleCommissionThreadReply(s *discordgo.Session, ct *config.CommissionTicket, m *discordgo.MessageCreate) {
	content := formatCommissionFreelancerReply(m)
	_, err := s.ChannelMessageSendComplex(ct.ChannelID, &discordgo.MessageSend{
		Content:         content,
		AllowedMentions: &discordgo.MessageAllowedMentions{},
	})
	if err != nil {
		slog.Error("commission failed to forward thread message", "message_id", m.ID, "channel_id", ct.ChannelID, "error", err)
	}
}

func commissionThreadReferenceTargetsClient(s *discordgo.Session, ct *config.CommissionTicket, threadID, referencedID string) bool {
	if referencedID == ct.LogMessageID {
		return true