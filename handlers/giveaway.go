package handlers

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"discord-bot/config"
	"discord-bot/lang"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

var (
	giveawayTimers   = make(map[string]*time.Timer)
	giveawayTimersMu sync.Mutex
)

func giveawayCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:                     "giveaway",
			Description:              "Giveaway management",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "create",
					Description: "Create a new giveaway",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel to post the giveaway in", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "prize", Description: "What are you giving away?", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "duration", Description: "Duration (e.g. 10m, 2h, 1d)", Required: true},
						{Type: discordgo.ApplicationCommandOptionInteger, Name: "winners", Description: "Number of winners (default: 1)"},
					},
				},
				{
					Name:        "end",
					Description: "End a giveaway early and pick winners now",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "giveaway_id", Description: "Giveaway ID (from /giveaway list)", Required: true},
					},
				},
				{
					Name:        "reroll",
					Description: "Reroll the winners of an ended giveaway",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "giveaway_id", Description: "Giveaway ID", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "keep", Description: "Winners to keep: @mention or slot numbers, space/comma-separated (e.g. @Steve or 1,3)"},
					},
				},
				{
					Name:        "list",
					Description: "List all active giveaways",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
	}
}

func (h *Handler) handleGiveawayCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.isAdmin(s, i) {
		respond(s, i, lang.T("no_permission"), true)
		return
	}
	sub := i.ApplicationCommandData().Options[0]

	switch sub.Name {
	case "create":
		handleGiveawayCreate(s, i, sub.Options)
	case "end":
		handleGiveawayEnd(s, i, sub.Options)
	case "reroll":
		handleGiveawayReroll(s, i, sub.Options)
	case "list":
		handleGiveawayList(s, i)
	}
}

func handleGiveawayCreate(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		return
	}

	om := subOptMap(opts)
	ch := om["channel"].ChannelValue(s)