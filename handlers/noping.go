package handlers

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"discord-bot/config"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

func nopingCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:                     "noping",
			Description:              "Manage who can ping protected users/roles",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "whitelist",
					Description: "Allow a user to ping protected users/roles",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to allow", Required: true},
					},
				},
				{
					Name:        "blacklist",
					Description: "Remove a user from the no-ping whitelist",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to block again", Required: true},
					},
				},
				{
					Name:        "list",
					Description: "List users currently allowed to ping protected users/roles",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
	}
}

func (h *Handler) RegisterNoPing(s *discordgo.Session) {
	if !h.cfg.NoPing.Enabled || len(h.cfg.NoPing.ProtectedRoles) == 0 {
		return
	}

	protected := make(map[string]bool, len(h.cfg.NoPing.ProtectedRoles))
	for _, id := range h.cfg.NoPing.ProtectedRoles {
		protected[strings.TrimSpace(id)] = true
	}

	bypass := make(map[string]bool, len(h.cfg.NoPing.BypassRoles))
	for _, id := range h.cfg.NoPing.BypassRoles {
		bypass[strings.TrimSpace(id)] = true
	}

	s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		handleNoPing(s, m, &h.cfg.NoPing, protected, bypass)
	})

	slog.Info("noping active", "protected_roles", len(protected), "bypass_roles", len(bypass))
}

func (h *Handler) handleNoPingCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.isAdmin(s, i) {
		respond(s, i, "You do not have permission to use this command.", true)
		return
	}

	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		respond(s, i, "Use `/noping whitelist`, `/noping blacklist`, or `/noping list`.", true)
		return
	}

	sub := data.Options[0]
	switch sub.Name {
	case "whitelist":
		handleNoPingWhitelist(s, i, sub.Options)
	case "blacklist":
		handleNoPingBlacklist(s, i, sub.Options)
	case "list":
		handleNoPingList(s, i)
	default:
		respond(s, i, "Unknown no-ping subcommand.", true)
	}
}

func handleNoPingWhitelist(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	target := om["user"].UserValue(s)
	gs := storage.GetGuild(i.GuildID)

	gs.Lock()
	if gs.NoPing.BypassUsers == nil {
		gs.NoPing.BypassUsers = make(map[string]bool)
	}
	alreadyAllowed := gs.NoPing.BypassUsers[target.ID]
	gs.NoPing.BypassUsers[target.ID] = true
	gs.Unlock()

	if err := gs.Save(); err != nil {
		respond(s, i, fmt.Sprintf("Failed to save no-ping whitelist: `%s`", err.Error()), true)
		return
	}

	if alreadyAllowed {
		respond(s, i, fmt.Sprintf("<@%s> is already allowed to ping protected users/roles.", target.ID), true)
		return
	}
	respond(s, i, fmt.Sprintf("<@%s> can now ping protected users/roles.", target.ID), true)
}

func handleNoPingBlacklist(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	target := om["user"].UserValue(s)
	gs := storage.GetGuild(i.GuildID)

	gs.Lock()
	wasAllowed := false
	if gs.NoPing.BypassUsers != nil {
		wasAllowed = gs.NoPing.BypassUsers[target.ID]
		delete(gs.NoPing.BypassUsers, target.ID)
	}
	gs.Unlock()

	if err := gs.Save(); err != nil {
		respond(s, i, fmt.Sprintf("Failed to save no-ping whitelist: `%s`", err.Error()), true)
		return
	}

	if !wasAllowed {
		respond(s, i, fmt.Sprintf("<@%s> was not on the no-ping whitelist.", target.ID), true)
		return
	}
	respond(s, i, fmt.Sprintf("<@%s> can no longer ping protected users/roles.", target.ID), true)
}

func handleNoPingList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	gs := storage.GetGuild(i.GuildID)

	gs.Lock()
	ids := make([]string, 0, len(gs.NoPing.BypassUsers))
	for userID, allowed := range gs.NoPing.BypassUsers {
		if allowed {
			ids = append(ids, userID)
		}
	}
	gs.Unlock()

	if len(ids) == 0 {
		respond(s, i, "No users are currently whitelisted for no-ping.", true)
		return
	}

	sort.Strings(ids)
	mentions := make([]string, 0, len(ids))
	for _, id := range ids {
		mentions = append(mentions, "<@"+id+">")
	}
	respond(s, i, "No-ping whitelist:\n"+strings.Join(mentions, "\n"), true)
}

func handleNoPing(s *discordgo.Session, m *discordgo.MessageCreate, cfg *config.NoPingConfig, protected map[string]bool, bypass map[string]bool) {
	if m.Author == nil || m.Author.Bot || m.GuildID == "" {
		return
	}

	if isNoPingUserBypassed(m.GuildID, m.Author.ID) {
		return
	}

	if len(bypass) > 0 {
		member, err := s.GuildMember(m.GuildID, m.Author.ID)
		if err == nil {
			for _, roleID := range member.Roles {
				if bypass[roleID] {
					return
				}
			}
		}
	}

	for _, roleID := range m.MentionRoles {
		if !protected[roleID] {
			continue
		}
		roleName := roleID
		if r, err := s.State.Role(m.GuildID, roleID); err == nil {
			roleName = r.Name
		}
		triggerNoPing(s, m, cfg, roleName)
		return
	}

	for _, mentioned := range m.Mentions {
		if mentioned.ID == m.Author.ID {
			continue
		}
		member, err := s.GuildMember(m.GuildID, mentioned.ID)
		if err != nil {
			continue
		}
		for _, roleID := range member.Roles {
			if !protected[roleID] {
				continue
			}
			roleName := roleID
			if r, err := s.State.Role(m.GuildID, roleID); err == nil {
				roleName = r.Name
			}
			triggerNoPing(s, m, cfg, roleName)
			return
		}
	}
}

func isNoPingUserBypassed(guildID, userID string) bool {
	gs := storage.GetGuild(guildID)
	gs.Lock()
	defer gs.Unlock()
	return gs.NoPing.BypassUsers != nil && gs.NoPing.BypassUsers[userID]
}

func triggerNoPing(s *discordgo.Session, m *discordgo.MessageCreate, cfg *config.NoPingConfig, roleName string) {
	if cfg.DeleteMessage {
		_ = s.ChannelMessageDelete(m.ChannelID, m.ID)
	}
	msg := buildNoPingMessage(cfg.Message, m.Author.ID, roleName)
	sendTemp(s, m.ChannelID, msg, 8)
	logNoPing(s, m.Message, roleName)
}

func logNoPing(s *discordgo.Session, m *discordgo.Message, roleName string) {
	gs := storage.GetGuild(m.GuildID)
	logCh := config.EffectiveModLogChannel(storage.Cfg, gs)
	if logCh == "" {
		return
	}

	content := m.Content
	if content == "" {
		content = "*(no text content)*"
	}
	if len(content) > 1024 {
		content = content[:1021] + "..."
	}

	embed := &discordgo.MessageEmbed{
		Title: "NoPing: Message Deleted",
		Color: 0xFFA500,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Author",
				Value:  fmt.Sprintf("<@%s> - %s (`%s`)", m.Author.ID, m.Author.Username, m.Author.ID),
				Inline: false,
			},
			{
				Name:   "Channel",
				Value:  fmt.Sprintf("<#%s>", m.ChannelID),
				Inline: true,
			},
			{
				Name:   "Protected Role Pinged",
				Value:  roleName,
				Inline: true,
			},
			{
				Name:  "Message Content",
				Value: content,
			},
		},
		Footer:    &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Message ID: %s", m.ID)},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	_, _ = s.ChannelMessageSendEmbed(logCh, embed)
}

func buildNoPingMessage(template, userID, roleName string) string {
	if template == "" {
		template = "{user} You are not allowed to ping **{role}**!"
	}
	msg := strings.ReplaceAll(template, "{user}", "<@"+userID+">")
	msg = strings.ReplaceAll(msg, "{role}", roleName)
	return msg
}
