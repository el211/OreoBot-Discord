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