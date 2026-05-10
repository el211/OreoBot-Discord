package handlers

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"time"

	"discord-bot/lang"

	"github.com/bwmarrin/discordgo"
)

type pendingLink struct {
	discordID string
	expiresAt time.Time
}

var (
	pendingLinks   = make(map[string]pendingLink)
	pendingLinksMu sync.Mutex
)

func generateLinkCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 6)
	for i := range code {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		code[i] = chars[n.Int64()]
	}
	return string(code)
}

func ConsumeLinkCode(code string) (discordID string, valid bool) {
	pendingLinksMu.Lock()
	defer pendingLinksMu.Unlock()

	p, ok := pendingLinks[code]
	if !ok {
		return "", false
	}
	if time.Now().After(p.expiresAt) {
		delete(pendingLinks, code)
		return "", false
	}
	delete(pendingLinks, code)
	return p.discordID, true
}

func (h *Handler) StartLinkPoller(s *discordgo.Session, guildID string) {
	poll := func() {
		confirmations, err := h.mcStore.PopConfirmed()
		if err != nil {
			slog.Error("mc pop confirmed error", "error", err)
			return
		}
		for _, c := range confirmations {
			link := MCLink{
				DiscordID: c.DiscordID,
				UUID:      c.UUID,
				Username:  c.Username,
				LinkedAt:  time.Now().Format("2006-01-02 15:04"),
			}
			if err := h.mcStore.SaveLink(link); err != nil {
				slog.Error("mc save link failed", "discord_id", c.DiscordID, "error", err)
				continue
			}

			if s != nil && guildID != "" {
				if err := s.GuildMemberNickname(guildID, c.DiscordID, c.Username); err != nil {
					slog.Warn("mc could not rename member", "discord_id", c.DiscordID, "username", c.Username, "error", err)
				}
			}

			if s != nil {
				if ch, err := s.UserChannelCreate(c.DiscordID); err == nil {
					_, _ = s.ChannelMessageSend(ch.ID, lang.T("mc_link_poller_dm", "username", c.Username))
				}
			}

			slog.Info("mc account linked", "discord_id", c.DiscordID, "username", c.Username, "uuid", c.UUID)
		}
	}

	poll()
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		for range ticker.C {
			poll()
		}
	}()
}

func minecraftCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "mc",
			Description: "Minecraft server management & player profile",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name: "status", Description: "Check if the Minecraft server is reachable",
					Type: discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name: "command", Description: "Execute an RCON command on the Minecraft server",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "cmd", Description: "The command to run (e.g. list, whitelist add Steve)", Required: true},
					},
				},
				{
					Name: "players", Description: "List online players",
					Type: discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name: "say", Description: "Broadcast a message in-game",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "message", Description: "Message to broadcast", Required: true},
					},
				},
				{
					Name: "whitelist", Description: "Add or remove a player from the whitelist",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type: discordgo.ApplicationCommandOptionString, Name: "action", Description: "add / remove", Required: true,
							Choices: []*discordgo.ApplicationCommandOptionChoice{
								{Name: "add", Value: "add"},
								{Name: "remove", Value: "remove"},
							},
						},
						{Type: discordgo.ApplicationCommandOptionString, Name: "player", Description: "Player name", Required: true},
					},
				},
				{
					Name: "link", Description: "Link your Discord account to your Minecraft account",
					Type: discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name: "unlink", Description: "Unlink your Minecraft account from Discord",
					Type: discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name: "profile", Description: "View your linked Minecraft profile (balance, homes, inventory)",
					Type: discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "Discord user to check (admin only for others)"},
					},
				},
				{
					Name: "linked", Description: "(Admin) List all linked Discord ↔ Minecraft accounts",
					Type: discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
	}
}

func (h *Handler) handleMinecraftCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.cfg.Minecraft.Enabled {
		respond(s, i, lang.T("mc_disabled"), true)
		return
	}

	sub := i.ApplicationCommandData().Options[0]

	switch sub.Name {
	case "link":
		h.handleMCLink(s, i)
		return
	case "unlink":
		h.handleMCUnlink(s, i)
		return
	case "profile":
		h.handleMCProfile(s, i, sub.Options)
		return
	case "linked":
		h.handleMCLinked(s, i)
		return
	}

	if !h.isAdmin(s, i) {
		respond(s, i, lang.T("no_permission_subcommand"), true)
		return
	}
	if h.rcon == nil {
		respond(s, i, lang.T("mc_rcon_not_init"), true)
		return
	}

	switch sub.Name {
	case "status":
		h.handleMCStatus(s, i)
	case "command":
		h.handleMCCommand(s, i, sub.Options)
	case "players":
		h.handleMCPlayers(s, i)
	case "say":
		h.handleMCSay(s, i, sub.Options)
	case "whitelist":
		h.handleMCWhitelist(s, i, sub.Options)
	}
}

func (h *Handler) handleMCStatus(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.rcon.IsConnected() {
		if err := h.rcon.Connect(); err != nil {
			respond(s, i, lang.T("mc_rcon_unreachable", "error", err.Error()), true)
			return
		}
	}
	respond(s, i, lang.T("mc_online"), true)
}

func (h *Handler) handleMCCommand(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	cmd := om["cmd"].StringValue()

	resp, err := h.rcon.Command(cmd)
	if err != nil {
		respond(s, i, lang.T("mc_rcon_error", "error", err.Error()), true)
		return
	}
	if resp == "" {
		resp = "(no output)"
	}
	if len(resp) > 1900 {
		resp = resp[:1900] + "..."
	}
	respond(s, i, fmt.Sprintf("```\n> %s\n%s\n```", cmd, resp), true)
}

func (h *Handler) handleMCPlayers(s *discordgo.Session, i *discordgo.InteractionCreate) {
	resp, err := h.rcon.Command("list")
	if err != nil {
		respond(s, i, lang.T("mc_rcon_error", "error", err.Error()), true)
		return
	}
	respondEmbed(s, i, &discordgo.MessageEmbed{
		Title:       lang.T("mc_players_embed_title"),
		Description: resp,
		Color:       0x55FF55,
	}, true)
}

func (h *Handler) handleMCSay(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	message := om["message"].StringValue()

	_, err := h.rcon.Command(fmt.Sprintf("say [Discord] %s: %s", i.Member.User.Username, message))
	if err != nil {
		respond(s, i, lang.T("mc_rcon_error", "error", err.Error()), true)
		return
	}
	respond(s, i, lang.T("mc_say_sent", "message", message), false)
}

func sanitizeMCPlayerName(name string) (string, bool) {
	name = strings.ReplaceAll(name, " ", "")
	if len(name) == 0 || len(name) >= 16 {
		return "", false
	}
	return name, true
}

func (h *Handler) handleMCWhitelist(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	action := om["action"].StringValue()
	rawPlayer := om["player"].StringValue()

	player, ok := sanitizeMCPlayerName(rawPlayer)
	if !ok {
		respond(s, i, lang.T("mc_invalid_player"), true)
		return
	}

	resp, err := h.rcon.Command(fmt.Sprintf("whitelist %s %s", action, player))
	if err != nil {
		respond(s, i, lang.T("mc_rcon_error", "error", err.Error()), true)
		return
	}
	respond(s, i, lang.T("mc_whitelist_result", "action", action, "player", player, "result", resp), true)
}

func (h *Handler) handleMCLink(s *discordgo.Session, i *discordgo.InteractionCreate) {
	discordID := i.Member.User.ID

	if link, err := h.mcStore.LoadLink(discordID); err == nil {
		respond(s, i, lang.T("mc_already_linked", "username", link.Username), true)
		return
	}

	code := generateLinkCode()
	expiresAt := time.Now().Add(10 * time.Minute)

	pendingLinksMu.Lock()
	for k, p := range pendingLinks {
		if p.discordID == discordID || time.Now().After(p.expiresAt) {
			delete(pendingLinks, k)
		}
	}
	pendingLinks[code] = pendingLink{discordID: discordID, expiresAt: expiresAt}
	pendingLinksMu.Unlock()

	if err := h.mcStore.SavePendingCode(code, discordID, i.GuildID, expiresAt); err != nil {
		slog.Error("mc save pending code error", "error", err)
		respond(s, i, lang.T("mc_link_code_failed"), true)
		return
	}

	respondEmbed(s, i, &discordgo.MessageEmbed{
		Title:       lang.T("mc_link_embed_title"),
		Description: lang.T("mc_link_embed_description", "code", code),
		Color:       0x5865F2,
		Footer:      &discordgo.MessageEmbedFooter{Text: lang.T("mc_link_embed_footer")},
	}, true)
}

func (h *Handler) handleMCUnlink(s *discordgo.Session, i *discordgo.InteractionCreate) {
	discordID := i.Member.User.ID

	link, err := h.mcStore.LoadLink(discordID)
	if err != nil {
		respond(s, i, lang.T("mc_not_linked"), true)
		return
	}

	if err := h.mcStore.DeleteLink(discordID); err != nil {
		respond(s, i, lang.T("mc_unlink_failed", "error", err.Error()), true)
		return
	}

	if i.GuildID != "" {
		if err := s.GuildMemberNickname(i.GuildID, discordID, ""); err != nil {
			slog.Warn("mc could not clear nickname", "discord_id", discordID, "error", err)
		}
	}

	respond(s, i, lang.T("mc_unlinked", "username", link.Username), true)
}

func (h *Handler) handleMCProfile(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	targetDiscordID := i.Member.User.ID
	targetName := i.Member.User.Username

	if len(opts) > 0 {
		om := subOptMap(opts)
		if u, ok := om["user"]; ok {
			user := u.UserValue(s)
			if user.ID != i.Member.User.ID && !h.isAdmin(s, i) {
				respond(s, i, lang.T("mc_profile_admin_only"), true)
				return
			}
			targetDiscordID = user.ID
			targetName = user.Username
		}
	}

	link, err := h.mcStore.LoadLink(targetDiscordID)
	if err != nil {
		if targetDiscordID == i.Member.User.ID {
			respond(s, i, lang.T("mc_profile_self_not_linked"), true)
		} else {
			respond(s, i, lang.T("mc_profile_other_not_linked", "user", targetName), true)
		}
		return
	}

	if h.rcon == nil {
		respond(s, i, lang.T("mc_profile_rcon_unavailable"), true)
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})

	balance := h.rconQuery(fmt.Sprintf("oe-discord balance %s", link.UUID))
	homes := h.rconQuery(fmt.Sprintf("oe-discord homes %s", link.UUID))
	onlineStatus := h.rconQuery(fmt.Sprintf("oe-discord online %s", link.UUID))

	embed := &discordgo.MessageEmbed{
		Title: lang.T("mc_profile_embed_title", "username", link.Username),
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: fmt.Sprintf("https://mc-heads.net/avatar/%s/64", link.UUID),
		},
		Fields: []*discordgo.MessageEmbedField{
			{Name: lang.T("mc_profile_field_username"), Value: link.Username, Inline: true},
			{Name: lang.T("mc_profile_field_status"), Value: onlineStatus, Inline: true},
			{Name: lang.T("mc_profile_field_linked_since"), Value: link.LinkedAt, Inline: true},
			{Name: lang.T("mc_profile_field_balance"), Value: balance, Inline: true},
			{Name: lang.T("mc_profile_field_homes"), Value: homes, Inline: false},
		},
		Color:  0x55FF55,
		Footer: &discordgo.MessageEmbedFooter{Text: "UUID: " + link.UUID},
	}

	_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
		Flags:  discordgo.MessageFlagsEphemeral,
	})
}

func (h *Handler) handleMCLinked(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.isAdmin(s, i) {
		respond(s, i, lang.T("admin_only"), true)
		return
	}

	links, err := h.mcStore.ListLinks()
	if err != nil || len(links) == 0 {
		respond(s, i, lang.T("mc_linked_none"), true)
		return
	}

	var sb strings.Builder
	sb.WriteString(lang.T("mc_linked_header", "count", fmt.Sprintf("%d", len(links))))
	for _, link := range links {
		sb.WriteString(lang.T("mc_linked_entry",
			"discord_id", link.DiscordID,
			"username", link.Username,
			"linked_at", link.LinkedAt,
		))
	}
	respond(s, i, sb.String(), true)
}

func (h *Handler) rconQuery(cmd string) string {
	if h.rcon == nil {
		return "*(RCON unavailable)*"
	}
	resp, err := h.rcon.Command(cmd)
	if err != nil {
		slog.Error("mc rcon query failed", "cmd", cmd, "error", err)
		return "*(fetch failed)*"
	}
	if resp == "" {
		return "*(no data)*"
	}
	return resp
}

func (h *Handler) InitMCLinkStore() {
	backend := h.cfg.Minecraft.LinkBackend
	if backend == "" {
		backend = "file"
	}

	switch backend {
	case "mongodb":
		s, err := newMongoLinkStore(h.cfg)
		if err != nil {
			slog.Warn("[MC] Failed to initialise MongoDB link store, falling back to file store", "error", err)
			h.mcStore = newFileLinkStore()
			slog.Info("mc link backend", "backend", "file (fallback)")
		} else {
			h.mcStore = s
			slog.Info("mc link backend", "backend", "mongodb")
		}
	default:
		h.mcStore = newFileLinkStore()
		slog.Info("mc link backend", "backend", "file")
	}
}

func (h *Handler) GetMCStore() MCLinkStore {
	return h.mcStore
}
