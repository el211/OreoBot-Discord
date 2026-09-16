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

// scamKeywords are phrases strongly associated with Discord scams. Matched
// case-insensitively as substrings. Admins extend this via
// anti_scam.extra_keywords.
var scamKeywords = []string{
	"free nitro",
	"free discord nitro",
	"nitro giveaway",
	"discord nitro",
	"claim your nitro",
	"get nitro free",
	"steam gift",
	"free steam",
	"gift card",
	"free gift",
	"who is first",
	"first come first serve",
	"airdrop",
	"claim your reward",
	"you have been selected",
	"congratulations you won",
	"@everyone free",
	"@everyone gift",
	"crypto giveaway",
	"double your",
	"presale",
	"connect your wallet",
	"verify your wallet",
}

// scamResult is the outcome of scanning a piece of text.
type scamResult struct {
	score   int
	reasons []string
}

// scanForScam scores text for scam signals. A known phishing domain is an
// instant, high-confidence hit; scam phrases add up; @everyone paired with a
// link is a classic bait pattern.
func scanForScam(text string, mentionsEveryone bool, extraDomains, extraKeywords []string) scamResult {
	res := scamResult{}
	lower := strings.ToLower(text)

	hasLink := false
	for _, match := range urlRe.FindAllString(text, -1) {
		host := extractHost(match)
		if host == "" {
			continue
		}
		hasLink = true
		if isPhishingDomain(host, extraDomains) {
			res.score += 3
			res.reasons = append(res.reasons, "phishing domain: "+host)
		}
	}
	if inv := inviteRe.FindString(text); inv != "" {
		hasLink = true
	}

	keywords := append(scamKeywords, extraKeywords...)
	for _, kw := range keywords {
		kw = strings.ToLower(strings.TrimSpace(kw))
		if kw != "" && strings.Contains(lower, kw) {
			res.score++
			res.reasons = append(res.reasons, "phrase: "+kw)
		}
	}

	if mentionsEveryone && hasLink {
		res.score += 2
		res.reasons = append(res.reasons, "@everyone/@here with a link")
	}
	return res
}

// scanImagesForScam scans image attachments for scam text via OCR and folds any
// findings into res. OCR is optional; when disabled this returns res unchanged.
// The real implementation lives in scam_ocr.go.
func (h *Handler) scanImagesForScam(m *discordgo.Message, res scamResult) scamResult {
	return res
}

// RegisterAntiScam attaches the scam-detection message handler.
func (h *Handler) RegisterAntiScam(s *discordgo.Session) {
	if !h.cfg.AntiScam.Enabled {
		return
	}
	s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		h.handleAntiScamMessage(s, m)
	})
	slog.Info("anti-scam active",
		"threshold", h.cfg.AntiScam.EffectiveScoreThreshold(),
		"ocr", h.cfg.AntiScam.OCR.Enabled)
}

func (h *Handler) handleAntiScamMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot || m.GuildID == "" {
		return
	}
	if h.isScamExempt(s, m) {
		return
	}

	cfg := h.cfg.AntiScam
	res := scanForScam(m.Content, m.MentionEveryone, cfg.ExtraDomains, cfg.ExtraKeywords)

	// Optional: scan image attachments for scam text.
	res = h.scanImagesForScam(m.Message, res)

	if res.score < cfg.EffectiveScoreThreshold() {
		return
	}

	archived := downloadAttachments(m.Message)
	_ = s.ChannelMessageDelete(m.ChannelID, m.ID)

	warn := cfg.Message
	if warn == "" {
		warn = "⚠️ {user}, your message was removed for looking like a scam. If this was a mistake, contact a moderator."
	}
	warn = strings.ReplaceAll(warn, "{user}", "<@"+m.Author.ID+">")
	sendTemp(s, m.ChannelID, warn, 8)

	if cfg.TimeoutMinutes > 0 {
		until := time.Now().Add(time.Duration(cfg.TimeoutMinutes) * time.Minute)
		_ = s.GuildMemberTimeout(m.GuildID, m.Author.ID, &until)
	}

	logScam(s, m.Message, res, archived, cfg.TimeoutMinutes)
}

// isScamExempt reuses the link-filter trust model: members with an allowed role
// and members who can manage messages (mods/admins) are not scanned.
func (h *Handler) isScamExempt(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	gs := storage.GetGuild(m.GuildID)
	allowedRoles := config.MergedLinkFilterAllowedRoles(h.cfg, gs)
	if len(allowedRoles) > 0 {
		roleSet := make(map[string]bool, len(allowedRoles))
		for _, id := range allowedRoles {
			roleSet[strings.TrimSpace(id)] = true
		}
		var roles []string
		if m.Member != nil {
			roles = m.Member.Roles
		} else if member, err := s.GuildMember(m.GuildID, m.Author.ID); err == nil {
			roles = member.Roles
		}
		for _, rid := range roles {
			if roleSet[rid] {
				return true
			}
		}
	}
	if perms, err := s.State.MessagePermissions(m.Message); err == nil {
		if perms&discordgo.PermissionManageMessages != 0 || perms&discordgo.PermissionAdministrator != 0 {
			return true
		}
	}
	return false
}

func logScam(s *discordgo.Session, m *discordgo.Message, res scamResult, archived []archivedAttachment, timeoutMin int) {
	gs := storage.GetGuild(m.GuildID)
	logCh := config.EffectiveModLogChannel(storage.Cfg, gs)
	if logCh == "" {
		return
	}

	content := m.Content
	if content == "" {
		content = "*(no text — see attachment)*"
	} else if len(content) > 1024 {
		content = content[:1021] + "..."
	}
	reasons := strings.Join(res.reasons, "\n")
	if reasons == "" {
		reasons = "*(heuristic score)*"
	}
	action := "Deleted"
	if timeoutMin > 0 {
		action = fmt.Sprintf("Deleted + %dm timeout", timeoutMin)
	}

	embed := &discordgo.MessageEmbed{
		Title: "🚨 Anti-Scam: Message Removed",
		Color: 0xE01B24,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Author", Value: fmt.Sprintf("<@%s> - %s (`%s`)", m.Author.ID, m.Author.Username, m.Author.ID)},
			{Name: "Channel", Value: fmt.Sprintf("<#%s>", m.ChannelID), Inline: true},
			{Name: "Score / Action", Value: fmt.Sprintf("%d — %s", res.score, action), Inline: true},
			{Name: "Signals", Value: reasons},
			{Name: "Message Content", Value: content},
		},
		Footer:    &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Message ID: %s", m.ID)},
		Timestamp: time.Now().Format(time.RFC3339),
	}
	_, _ = s.ChannelMessageSendComplex(logCh, &discordgo.MessageSend{
		Embed: embed,
		Files: asDiscordFiles(archived),
	})
}
