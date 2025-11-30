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
	prize := om["prize"].StringValue()
	durStr := om["duration"].StringValue()

	winners := int64(1)
	if w, ok := om["winners"]; ok {
		winners = w.IntValue()
		if winners < 1 {
			winners = 1
		}
		if winners > 20 {
			winners = 20
		}
	}

	dur, err := parseDuration(durStr)
	if err != nil || dur <= 0 {
		followup(s, i, lang.T("giveaway_invalid_duration"))
		return
	}

	endsAt := time.Now().Add(dur)
	hostID := i.Member.User.ID

	gs := storage.GetGuild(i.GuildID)

	embed := buildGiveawayEmbed(prize, hostID, int(winners), endsAt, 0)
	msg, err := s.ChannelMessageSendComplex(ch.ID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    lang.T("giveaway_embed_enter_btn"),
						Style:    discordgo.PrimaryButton,
						CustomID: "giveaway_enter:pending",
					},
				},
			},
		},
	})
	if err != nil {
		followup(s, i, lang.T("giveaway_post_failed", "error", err.Error()))
		return
	}

	giveawayID := msg.ID

	_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel: ch.ID,
		ID:      msg.ID,
		Embeds:  &[]*discordgo.MessageEmbed{embed},
		Components: &[]discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    lang.T("giveaway_embed_enter_btn"),
						Style:    discordgo.PrimaryButton,
						CustomID: "giveaway_enter:" + giveawayID,
					},
				},
			},
		},
	})

	gw := config.Giveaway{
		ID:         giveawayID,
		GuildID:    i.GuildID,
		ChannelID:  ch.ID,
		MessageID:  msg.ID,
		Prize:      prize,
		Winners:    int(winners),
		EndsAt:     endsAt.Format(time.RFC3339),
		HostID:     hostID,
		Ended:      false,
		EntrantIDs: map[string]bool{},
	}

	gs.Lock()
	gs.Giveaways = append(gs.Giveaways, gw)
	gs.Unlock()
	_ = gs.Save()

	scheduleGiveaway(s, i.GuildID, giveawayID, dur)

	followup(s, i, lang.T("giveaway_started",
		"prize", prize,
		"id", giveawayID,
		"channel_id", ch.ID,
		"timestamp", fmt.Sprintf("%d", endsAt.Unix()),
		"winners", fmt.Sprintf("%d", winners),
	))
}

func handleGiveawayEnd(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	giveawayID := om["giveaway_id"].StringValue()

	gs := storage.GetGuild(i.GuildID)