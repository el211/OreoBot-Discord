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
	gs.Lock()
	var found *config.Giveaway
	for idx := range gs.Giveaways {
		if gs.Giveaways[idx].ID == giveawayID {
			found = &gs.Giveaways[idx]
			break
		}
	}
	if found == nil {
		gs.Unlock()
		respond(s, i, lang.T("giveaway_not_found", "id", giveawayID), true)
		return
	}
	if found.Ended {
		gs.Unlock()
		respond(s, i, lang.T("giveaway_already_ended"), true)
		return
	}
	gs.Unlock()

	cancelGiveawayTimer(i.GuildID, giveawayID)
	endGiveaway(s, i.GuildID, giveawayID)
	respond(s, i, lang.T("giveaway_ended_early", "id", giveawayID), true)
}

func handleGiveawayReroll(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	giveawayID := om["giveaway_id"].StringValue()

	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	var found *config.Giveaway
	for idx := range gs.Giveaways {
		if gs.Giveaways[idx].ID == giveawayID {
			found = &gs.Giveaways[idx]
			break
		}
	}
	if found == nil {
		gs.Unlock()
		respond(s, i, lang.T("giveaway_not_found_short", "id", giveawayID), true)
		return
	}
	if !found.Ended {
		gs.Unlock()
		respond(s, i, lang.T("giveaway_still_active"), true)
		return
	}
	entrants := make([]string, 0, len(found.EntrantIDs))
	for uid := range found.EntrantIDs {
		entrants = append(entrants, uid)
	}
	numWinners := found.Winners
	prevWinners := make([]string, len(found.WinnerIDs))
	copy(prevWinners, found.WinnerIDs)
	channelID := found.ChannelID
	prize := found.Prize
	gs.Unlock()

	if len(entrants) == 0 {
		respond(s, i, lang.T("giveaway_no_entrants"), true)
		return
	}

	keptWinners := []string{}
	keptSet := map[string]bool{}
	if keepOpt, ok := om["keep"]; ok {
		raw := strings.ReplaceAll(keepOpt.StringValue(), ",", " ")
		prevWinnersSet := map[string]bool{}
		for _, uid := range prevWinners {
			prevWinnersSet[uid] = true
		}
		for _, token := range strings.Fields(raw) {
			uid := ""
			if strings.HasPrefix(token, "<@") && strings.HasSuffix(token, ">") {
				uid = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(token, "<@!"), "<@"), ">")
			} else if _, err := strconv.ParseUint(token, 10, 64); err == nil {
				uid = token
			} else if n, err := strconv.Atoi(token); err == nil && n >= 1 && n <= numWinners {
				if n-1 < len(prevWinners) {
					uid = prevWinners[n-1]
				}
			}
			if uid != "" && prevWinnersSet[uid] && !keptSet[uid] {
				keptWinners = append(keptWinners, uid)
				keptSet[uid] = true
			}
		}
	}

	if len(keptWinners) >= numWinners {
		respond(s, i, lang.T("giveaway_reroll_nothing_to_reroll"), true)
		return
	}

	filteredEntrants := make([]string, 0, len(entrants))
	for _, uid := range entrants {
		if !keptSet[uid] {
			filteredEntrants = append(filteredEntrants, uid)
		}
	}

	numNew := numWinners - len(keptWinners)
	if len(filteredEntrants) == 0 {
		respond(s, i, lang.T("giveaway_no_entrants"), true)
		return
	}

	newWinners := pickWinners(filteredEntrants, numNew)
	allWinners := append(keptWinners, newWinners...)

	gs.Lock()
	for idx := range gs.Giveaways {
		if gs.Giveaways[idx].ID == giveawayID {
			gs.Giveaways[idx].WinnerIDs = allWinners
			break
		}
	}
	gs.Unlock()
	_ = gs.Save()

	newMentions := make([]string, len(newWinners))
	for idx, w := range newWinners {
		newMentions[idx] = fmt.Sprintf("<@%s>", w)
	}
	newMentionsStr := strings.Join(newMentions, ", ")

	if len(keptWinners) == 0 {
		_, _ = s.ChannelMessageSend(channelID, lang.T("giveaway_reroll_announce", "prize", prize, "mentions", newMentionsStr))
		respond(s, i, lang.T("giveaway_rerolled", "mentions", newMentionsStr), true)
	} else {
		keptMentions := make([]string, len(keptWinners))
		for idx, w := range keptWinners {
			keptMentions[idx] = fmt.Sprintf("<@%s>", w)
		}
		keptMentionsStr := strings.Join(keptMentions, ", ")
		_, _ = s.ChannelMessageSend(channelID, lang.T("giveaway_reroll_partial_announce",
			"prize", prize,
			"kept", keptMentionsStr,
			"new", newMentionsStr,
		))
		respond(s, i, lang.T("giveaway_rerolled_partial",
			"kept", keptMentionsStr,
			"new", newMentionsStr,
		), true)
	}
}

func handleGiveawayList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	giveaways := make([]config.Giveaway, len(gs.Giveaways))
	copy(giveaways, gs.Giveaways)
	gs.Unlock()

	var active []config.Giveaway
	for _, gw := range giveaways {
		if !gw.Ended {
			active = append(active, gw)
		}
	}

	if len(active) == 0 {
		respond(s, i, lang.T("giveaway_none_active"), true)
		return
	}

	var sb strings.Builder
	sb.WriteString(lang.T("giveaway_list_header"))
	for _, gw := range active {
		endsAt, _ := time.Parse(time.RFC3339, gw.EndsAt)
		sb.WriteString(lang.T("giveaway_list_entry",
			"id", gw.ID,
			"prize", gw.Prize,
			"winners", fmt.Sprintf("%d", gw.Winners),
			"entries", fmt.Sprintf("%d", len(gw.EntrantIDs)),
			"timestamp", fmt.Sprintf("%d", endsAt.Unix()),
			"channel_id", gw.ChannelID,
		))
	}
	respond(s, i, sb.String(), true)
}

func HandleGiveawayEnter(s *discordgo.Session, i *discordgo.InteractionCreate) {
	parts := strings.SplitN(i.MessageComponentData().CustomID, ":", 2)
	if len(parts) != 2 {
		return
	}
	giveawayID := parts[1]
	userID := i.Member.User.ID

	gs := storage.GetGuild(i.GuildID)
	gs.Lock()

	var gw *config.Giveaway
	for idx := range gs.Giveaways {