package handlers

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"discord-bot/config"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

func verifyCommands(cfg *config.Config) []*discordgo.ApplicationCommand {
	if len(cfg.Verify.Products) == 0 {
		return nil
	}
	return []*discordgo.ApplicationCommand{
		{
			Name:        "verify",
			Description: "Grant a verified product role to a user",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "The Discord user to verify",
					Required:    true,
				},
				{
					Type:         discordgo.ApplicationCommandOptionString,
					Name:         "product",
					Description:  "The product they purchased",
					Required:     true,
					Autocomplete: true,
				},
			},
		},
	}
}

func (h *Handler) handleVerifyAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var query string
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "product" && opt.Focused {
			query = strings.ToLower(opt.StringValue())
			break
		}
	}

	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0, len(h.cfg.Verify.Products))
	for _, p := range h.cfg.Verify.Products {
		if query == "" || strings.Contains(strings.ToLower(p.Name), query) {
			choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
				Name:  p.Name,
				Value: p.Name,
			})
		}
		if len(choices) >= 25 {
			break
		}
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{
			Choices: choices,
		},
	})
}

func (h *Handler) handleVerify(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.isAdmin(s, i) {
		respond(s, i, "You do not have permission to use this command.", true)
		return
	}

	opts := optionMap(i)

	targetUser := opts["user"].UserValue(s)
	productName := optStr(opts, "product", "")

	var product *config.VerifyProduct
	for idx := range h.cfg.Verify.Products {
		if strings.EqualFold(h.cfg.Verify.Products[idx].Name, productName) {
			product = &h.cfg.Verify.Products[idx]
			break
		}
	}
	if product == nil {
		respond(s, i, fmt.Sprintf("Unknown product: **%s**. Check your `verify.products` config.", productName), true)
		return
	}

	if err := s.GuildMemberRoleAdd(i.GuildID, targetUser.ID, product.RoleID); err != nil {
		slog.Error("verify failed to assign role", "role_id", product.RoleID, "user_id", targetUser.ID, "error", err)
		respond(s, i, fmt.Sprintf("Failed to assign role: %v", err), true)
		return
	}

	gs := storage.GetGuild(i.GuildID)
	logCh := config.EffectiveModLogChannel(h.cfg, gs)
	if logCh != "" {
		embed := &discordgo.MessageEmbed{
			Title: "Verify: Role Assigned",
			Color: 0x00CC66,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "User", Value: fmt.Sprintf("<@%s> (`%s`)", targetUser.ID, targetUser.Username), Inline: true},
				{Name: "Product", Value: product.Name, Inline: true},
				{Name: "Role", Value: fmt.Sprintf("<@&%s>", product.RoleID), Inline: true},
				{Name: "Verified by", Value: fmt.Sprintf("<@%s>", i.Member.User.ID), Inline: true},
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}
		_, _ = s.ChannelMessageSendEmbed(logCh, embed)
	}

	respond(s, i, fmt.Sprintf("<@%s> has been verified for **%s** and granted <@&%s>!", targetUser.ID, product.Name, product.RoleID), false)
}
