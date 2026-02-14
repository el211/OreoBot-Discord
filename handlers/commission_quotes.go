package handlers

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"discord-bot/config"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

func handleCommissionQuoteButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	channelID := strings.TrimPrefix(i.MessageComponentData().CustomID, "commission_quote_btn:")
	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	ct, ok := gs.CommissionsRuntime.OpenCommissions[channelID]
	gs.Unlock()
	if !ok {
		respond(s, i, "Could not find this commission ticket.", true)
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "commission_quote_modal:" + channelID,
			Title:    fmt.Sprintf("Quote Commission #%04d", ct.Number),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "amount", Label: "Quote amount", Style: discordgo.TextInputShort, Required: true, Placeholder: "50.00", MaxLength: 20}}},
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "currency", Label: "Currency", Style: discordgo.TextInputShort, Required: false, Placeholder: "EUR", MaxLength: 5}}},
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "timeline", Label: "Delivery timeline", Style: discordgo.TextInputShort, Required: true, Placeholder: "e.g. 3 days", MaxLength: 100}}},
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "message", Label: "Message to client", Style: discordgo.TextInputParagraph, Required: true, Placeholder: "Explain what is included in your quote.", MaxLength: 800}}},
			},
		},
	})
}

func handleCommissionQuoteModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	channelID := strings.TrimPrefix(data.CustomID, "commission_quote_modal:")
	fields := modalTextValues(data.Components)

	amountStr := strings.ReplaceAll(strings.TrimSpace(fields["amount"]), ",", ".")
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		respond(s, i, "Invalid quote amount. Use a number like `50.00`.", true)
		return
	}
	currency := strings.ToUpper(strings.TrimSpace(fields["currency"]))
	if currency == "" {
		currency = "EUR"
	}
	quoteID := fmt.Sprintf("q%d", time.Now().UnixNano())
	quote := config.CommissionQuote{
		ID:           quoteID,
		FreelancerID: i.Member.User.ID,
		Amount:       amount,
		Currency:     currency,
		Timeline:     strings.TrimSpace(fields["timeline"]),
		Message:      strings.TrimSpace(fields["message"]),
		Status:       "pending",
		CreatedAt:    time.Now().Format(time.RFC3339),
	}

	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	ct, ok := gs.CommissionsRuntime.OpenCommissions[channelID]
	if ok {
		ensureCommissionTicketRuntime(&ct)
		ct.Quotes[quoteID] = quote
		gs.CommissionsRuntime.OpenCommissions[channelID] = ct
	}
	gs.Unlock()
	if !ok {
		respond(s, i, "Could not find this commission ticket.", true)
		return
	}
	_ = gs.Save()

	embed := &discordgo.MessageEmbed{
		Title: "New Freelancer Quote",
		Color: 0xF0A500,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Freelancer", Value: fmt.Sprintf("<@%s>", quote.FreelancerID), Inline: true},
			{Name: "Amount", Value: fmt.Sprintf("%.2f %s", quote.Amount, quote.Currency), Inline: true},
			{Name: "Timeline", Value: safeEmbedValue(quote.Timeline), Inline: true},
			{Name: "Message", Value: safeEmbedValue(quote.Message), Inline: false},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
	_, sendErr := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>", ct.UserID),
		Embeds:  []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "Accept Quote", Style: discordgo.SuccessButton, CustomID: "commission_quote_accept:" + channelID + ":" + quoteID},
			discordgo.Button{Label: "Decline Quote", Style: discordgo.DangerButton, CustomID: "commission_quote_decline:" + channelID + ":" + quoteID},
		}}},
	})
	if sendErr != nil {
		respond(s, i, fmt.Sprintf("Quote saved, but failed to post in ticket: `%s`", sendErr.Error()), true)
		return
	}
	respond(s, i, fmt.Sprintf("Quote sent to <@%s> in <#%s>.", ct.UserID, channelID), true)
}

func handleCommissionQuoteAccept(s *discordgo.Session, i *discordgo.InteractionCreate) {
	channelID, quoteID, ok := parseCommissionQuoteComponent(i.MessageComponentData().CustomID, "commission_quote_accept:")
	if !ok {
		respond(s, i, "Invalid quote action.", true)
		return
	}

	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	ct, exists := gs.CommissionsRuntime.OpenCommissions[channelID]
	var quote config.CommissionQuote
	quoteExists := false
	if exists {
		ensureCommissionTicketRuntime(&ct)
		quote, quoteExists = ct.Quotes[quoteID]
	}
	if exists && quoteExists && i.Member.User.ID == ct.UserID {
		for id, existing := range ct.Quotes {
			if existing.Status == "accepted" {
				existing.Status = "superseded"
				ct.Quotes[id] = existing
			}
		}
		quote.Status = "accepted"
		ct.Quotes[quoteID] = quote
		ct.AcceptedFreelancerID = quote.FreelancerID
		gs.CommissionsRuntime.OpenCommissions[channelID] = ct
	}
	gs.Unlock()

	if !exists {
		respond(s, i, "Could not find this commission ticket.", true)
		return
	}
	if i.Member.User.ID != ct.UserID {
		respond(s, i, "Only the client can accept this quote.", true)
		return
	}
	if !quoteExists {
		respond(s, i, "Could not find that quote.", true)
		return
	}

	allow := int64(discordgo.PermissionViewChannel | discordgo.PermissionSendMessages | discordgo.PermissionAttachFiles | discordgo.PermissionReadMessageHistory)
	if err := s.ChannelPermissionSet(channelID, quote.FreelancerID, discordgo.PermissionOverwriteTypeMember, allow, 0); err != nil {
		respond(s, i, fmt.Sprintf("Quote accepted, but I could not open the ticket for the freelancer: `%s`", err.Error()), true)
		return
	}

	_ = gs.Save()
	_, _ = s.ChannelMessageSend(channelID, fmt.Sprintf("Quote accepted by <@%s>. <@%s> now has access to this ticket.", ct.UserID, quote.FreelancerID))
	if ct.DiscussionThreadID != "" {
		_, _ = s.ChannelMessageSend(ct.DiscussionThreadID, fmt.Sprintf("Quote `%s` was accepted by <@%s>. <@%s> now has access to <#%s>.", quoteID, ct.UserID, quote.FreelancerID, channelID))
	}

	if ct.LogMessageID != "" && ct.LogChannelID != "" {
		components := commissionLogComponents(i.GuildID, channelID, ct.DiscussionThreadID, false, true)
		if components != nil {
			_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
				ID:         ct.LogMessageID,
				Channel:    ct.LogChannelID,
				Components: &components,
			})
		}
	}

	sendCommissionAcceptedDM(s, quote.FreelancerID, &ct, &quote)
	respond(s, i, "Quote accepted. The freelancer can now access this ticket.", true)
}

func handleCommissionQuoteDecline(s *discordgo.Session, i *discordgo.InteractionCreate) {
	channelID, quoteID, ok := parseCommissionQuoteComponent(i.MessageComponentData().CustomID, "commission_quote_decline:")
	if !ok {
		respond(s, i, "Invalid quote action.", true)
		return
	}
	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	ct, exists := gs.CommissionsRuntime.OpenCommissions[channelID]
	gs.Unlock()
	if !exists {
		respond(s, i, "Could not find this commission ticket.", true)
		return
	}
	if i.Member.User.ID != ct.UserID {
		respond(s, i, "Only the client can decline this quote.", true)
		return
	}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "commission_quote_decline_modal:" + channelID + ":" + quoteID,
			Title:    "Decline Quote",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "reason", Label: "Why are you declining?", Style: discordgo.TextInputParagraph, Required: true, MaxLength: 800}}},
			},
		},
	})
}

func handleCommissionQuoteDeclineModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	channelID, quoteID, ok := parseCommissionQuoteComponent(i.ModalSubmitData().CustomID, "commission_quote_decline_modal:")
	if !ok {
		respond(s, i, "Invalid quote action.", true)
		return
	}
	reason := strings.TrimSpace(modalTextValues(i.ModalSubmitData().Components)["reason"])
	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	ct, exists := gs.CommissionsRuntime.OpenCommissions[channelID]
	if exists && i.Member.User.ID == ct.UserID {
		ensureCommissionTicketRuntime(&ct)
		quote := ct.Quotes[quoteID]
		quote.Status = "declined"
		quote.Reason = reason
		ct.Quotes[quoteID] = quote
		gs.CommissionsRuntime.OpenCommissions[channelID] = ct
	}
	gs.Unlock()
	if !exists {
		respond(s, i, "Could not find this commission ticket.", true)
		return
	}
	if i.Member.User.ID != ct.UserID {
		respond(s, i, "Only the client can decline this quote.", true)
		return
	}
	_ = gs.Save()
	msg := fmt.Sprintf("Quote declined by <@%s>. Reason: %s", ct.UserID, reason)
	_, _ = s.ChannelMessageSend(channelID, msg)
	if ct.DiscussionThreadID != "" {
		_, _ = s.ChannelMessageSend(ct.DiscussionThreadID, fmt.Sprintf("Quote `%s` was declined. Reason: %s", quoteID, reason))
	}
	respond(s, i, "Quote declined and your reason was sent.", true)
}

func sendCommissionAcceptedDM(s *discordgo.Session, freelancerID string, ct *config.CommissionTicket, quote *config.CommissionQuote) {
	dm, err := s.UserChannelCreate(freelancerID)
	if err != nil {
		slog.Error("commission failed to create DM for freelancer", "freelancer_id", freelancerID, "error", err)
		return
	}
	msg := fmt.Sprintf("OreoManager: Your quote for Commission #%04d was accepted. You can now access the ticket: <#%s>\nAmount: %.2f %s\nTimeline: %s", ct.Number, ct.ChannelID, quote.Amount, quote.Currency, quote.Timeline)
	if _, err := s.ChannelMessageSend(dm.ID, msg); err != nil {
		slog.Error("commission failed to DM freelancer", "freelancer_id", freelancerID, "error", err)
	}
}

func parseCommissionQuoteComponent(customID, prefix string) (string, string, bool) {
	rest := strings.TrimPrefix(customID, prefix)
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
