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
	}
	for _, threadMsgID := range ct.ClientThreadMessages {
		if threadMsgID == referencedID {
			return true
		}
	}
	ref, err := s.ChannelMessage(threadID, referencedID)
	if err != nil || ref == nil || ref.Author == nil {
		return false
	}
	if s.State != nil && s.State.User != nil && ref.Author.ID == s.State.User.ID {
		content := strings.TrimSpace(ref.Content)
		return strings.HasPrefix(content, "Client ") || strings.HasPrefix(content, "Commission brief") || strings.Contains(content, "Project Description")
	}
	return false
}

func formatCommissionClientMirror(m *discordgo.MessageCreate) string {
	body := strings.TrimSpace(m.Content)
	if body == "" {
		body = "*(no text content)*"
	}
	if len(m.Attachments) > 0 {
		var urls []string
		for _, a := range m.Attachments {
			urls = append(urls, a.URL)
		}
		body += "\n\nAttachments:\n" + strings.Join(urls, "\n")
	}
	return truncateMessage(fmt.Sprintf("Client <@%s> wrote:\n%s", m.Author.ID, body), 1900)
}

func formatCommissionFreelancerReply(m *discordgo.MessageCreate) string {
	body := strings.TrimSpace(m.Content)
	if body == "" {
		body = "*(no text content)*"
	}
	if len(m.Attachments) > 0 {
		var urls []string
		for _, a := range m.Attachments {
			urls = append(urls, a.URL)
		}
		body += "\n\nAttachments:\n" + strings.Join(urls, "\n")
	}
	return truncateMessage(fmt.Sprintf("Freelancer %s: %s", m.Author.Username, body), 1900)
}
