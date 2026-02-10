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