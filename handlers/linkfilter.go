package handlers

import (
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"discord-bot/config"
	"discord-bot/lang"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

// urlRe matches explicit http(s):// links as well as bare domains like
// "example.com/path" and "www.example.com". It intentionally errs towards
// catching things: the whitelist decides what is actually allowed.
var urlRe = regexp.MustCompile(`(?i)\b((?:https?://)?(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,24})(?::\d{1,5})?(?:/[^\s]*)?`)

// inviteRe matches Discord invite links specifically.
var inviteRe = regexp.MustCompile(`(?i)(?:discord\.gg|discord(?:app)?\.com/invite|discord\.com/invite|discord\.gg)/\S+`)

func linkfilterCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:                     "linkfilter",
			Description:              "Manage the link/invite filter",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "adddomain",
					Description: "Whitelist a domain so anyone can post links to it (e.g. github.com)",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "domain", Description: "Domain to allow (e.g. github.com)", Required: true},
					},
				},
				{
					Name:        "removedomain",
					Description: "Remove a domain from the whitelist",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "domain", Description: "Domain to remove", Required: true},
					},
				},
				{
					Name:        "addrole",
					Description: "Allow a role to post any link",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionRole, Name: "role", Description: "Role allowed to post links", Required: true},
					},
				},
				{
					Name:        "removerole",
					Description: "Stop allowing a role to post links",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionRole, Name: "role", Description: "Role to remove", Required: true},
					},
				},
				{
					Name:        "message",
					Description: "Set the warning message shown when a link is removed ({user} = mention)",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "text", Description: "Warning text. Use {user} for the mention. Empty resets to default.", Required: false},
					},
				},
				{
					Name:        "list",
					Description: "Show the current link-filter whitelist and allowed roles",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
	}
}

func (h *Handler) RegisterLinkFilter(s *discordgo.Session) {
	if !h.cfg.LinkFilter.Enabled {
		return
	}
	s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		h.handleLinkFilterMessage(s, m)
	})
	slog.Info("link filter active",
		"allowed_roles", len(h.cfg.LinkFilter.AllowedRoles),
		"whitelist_domains", len(h.cfg.LinkFilter.WhitelistDomains))
}

func (h *Handler) handleLinkFilterMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot || m.GuildID == "" || m.Content == "" {
		return
	}

	gs := storage.GetGuild(m.GuildID)

	// Members with an allowed role may post anything.
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
				return
			}
		}
	}

	// Members who can manage messages (mods/admins) are exempt.
	if perms, err := s.State.MessagePermissions(m.Message); err == nil {
		if perms&discordgo.PermissionManageMessages != 0 || perms&discordgo.PermissionAdministrator != 0 {
			return
		}
	}

	whitelist := config.MergedLinkFilterWhitelist(h.cfg, gs)
	blocked, sample := findBlockedLink(m.Content, whitelist, h.cfg.LinkFilter.BlockInvites)
	if !blocked {
		return
	}

	if h.cfg.LinkFilter.DeleteMessage {
		_ = s.ChannelMessageDelete(m.ChannelID, m.ID)
	}

	warn := config.EffectiveLinkFilterMessage(h.cfg, gs)
	if warn == "" {
		warn = lang.T("linkfilter_warning", "user", "<@"+m.Author.ID+">")
	} else {
		warn = strings.ReplaceAll(warn, "{user}", "<@"+m.Author.ID+">")
	}
	sendTemp(s, m.ChannelID, warn, 8)

	logLinkFilter(s, m.Message, sample)
}

// findBlockedLink reports whether content contains a link that is not permitted.
// A link is permitted only if its host matches a whitelisted domain. Discord
// invites are always blocked when blockInvites is true, regardless of whitelist.
func findBlockedLink(content string, whitelist []string, blockInvites bool) (bool, string) {
	if blockInvites {
		if inv := inviteRe.FindString(content); inv != "" {
			return true, inv
		}
	}

	wl := make([]string, 0, len(whitelist))
	for _, d := range whitelist {
		if n := normalizeDomain(d); n != "" {
			wl = append(wl, n)
		}
	}

	for _, match := range urlRe.FindAllString(content, -1) {
		host := extractHost(match)
		if host == "" {
			continue
		}
		if !hostWhitelisted(host, wl) {
			return true, match
		}
	}
	return false, ""
}

// extractHost pulls the lowercase host out of a raw URL/domain token.
func extractHost(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.Index(s, "://"); i != -1 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i != -1 {
		s = s[:i]
	}
	if i := strings.Index(s, "@"); i != -1 {
		s = s[i+1:]
	}
	if i := strings.Index(s, ":"); i != -1 {
		s = s[:i]
	}
	s = strings.TrimPrefix(s, "www.")
	return strings.Trim(s, ".")
}

// normalizeDomain reduces a whitelist entry to a bare lowercase host.
func normalizeDomain(d string) string {
	return extractHost(d)
}

// hostWhitelisted returns true if host equals or is a subdomain of any entry.
func hostWhitelisted(host string, whitelist []string) bool {
	for _, w := range whitelist {
		if host == w || strings.HasSuffix(host, "."+w) {
			return true
		}
	}
	return false
}

func logLinkFilter(s *discordgo.Session, m *discordgo.Message, link string) {
	gs := storage.GetGuild(m.GuildID)
	logCh := config.EffectiveModLogChannel(storage.Cfg, gs)
	if logCh == "" {
		return
	}

	content := m.Content
	if len(content) > 1024 {
		content = content[:1021] + "..."
	}

	embed := &discordgo.MessageEmbed{
		Title: "Link Filter: Message Deleted",
		Color: 0xFFA500,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Author", Value: fmt.Sprintf("<@%s> - %s (`%s`)", m.Author.ID, m.Author.Username, m.Author.ID)},
			{Name: "Channel", Value: fmt.Sprintf("<#%s>", m.ChannelID), Inline: true},
			{Name: "Blocked Link", Value: link, Inline: true},
			{Name: "Message Content", Value: content},
		},
		Footer:    &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Message ID: %s", m.ID)},
		Timestamp: time.Now().Format(time.RFC3339),
	}
	_, _ = s.ChannelMessageSendEmbed(logCh, embed)
}

// ──────────────────────────────────────────
// /linkfilter command
// ──────────────────────────────────────────

func (h *Handler) handleLinkFilterCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !h.isAdmin(s, i) {
		respond(s, i, "You do not have permission to use this command.", true)
		return
	}

	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		respond(s, i, "Use `/linkfilter adddomain`, `removedomain`, `addrole`, `removerole`, or `list`.", true)
		return
	}

	sub := data.Options[0]
	om := subOptMap(sub.Options)
	gs := storage.GetGuild(i.GuildID)

	switch sub.Name {
	case "adddomain":
		domain := normalizeDomain(om["domain"].StringValue())
		if domain == "" {
			respond(s, i, "That doesn't look like a valid domain.", true)
			return
		}
		gs.Lock()
		if containsStr(gs.LinkFilter.ExtraWhitelistDomains, domain) {
			gs.Unlock()
			respond(s, i, fmt.Sprintf("`%s` is already whitelisted.", domain), true)
			return
		}
		gs.LinkFilter.ExtraWhitelistDomains = append(gs.LinkFilter.ExtraWhitelistDomains, domain)
		gs.Unlock()
		_ = gs.Save()
		respond(s, i, fmt.Sprintf("✅ Links to `%s` are now allowed for everyone.", domain), true)

	case "removedomain":
		domain := normalizeDomain(om["domain"].StringValue())
		gs.Lock()
		before := len(gs.LinkFilter.ExtraWhitelistDomains)
		gs.LinkFilter.ExtraWhitelistDomains = removeStr(gs.LinkFilter.ExtraWhitelistDomains, domain)
		removed := len(gs.LinkFilter.ExtraWhitelistDomains) != before
		gs.Unlock()
		if !removed {
			respond(s, i, fmt.Sprintf("`%s` was not in the runtime whitelist. (Domains set in config.json must be removed there.)", domain), true)
			return
		}
		_ = gs.Save()
		respond(s, i, fmt.Sprintf("✅ Removed `%s` from the whitelist.", domain), true)

	case "addrole":
		role := om["role"].RoleValue(s, i.GuildID)
		gs.Lock()
		if containsStr(gs.LinkFilter.ExtraAllowedRoles, role.ID) {
			gs.Unlock()
			respond(s, i, fmt.Sprintf("<@&%s> is already allowed to post links.", role.ID), true)
			return
		}
		gs.LinkFilter.ExtraAllowedRoles = append(gs.LinkFilter.ExtraAllowedRoles, role.ID)
		gs.Unlock()
		_ = gs.Save()
		respond(s, i, fmt.Sprintf("✅ <@&%s> can now post links freely.", role.ID), true)

	case "removerole":
		role := om["role"].RoleValue(s, i.GuildID)
		gs.Lock()
		before := len(gs.LinkFilter.ExtraAllowedRoles)
		gs.LinkFilter.ExtraAllowedRoles = removeStr(gs.LinkFilter.ExtraAllowedRoles, role.ID)
		removed := len(gs.LinkFilter.ExtraAllowedRoles) != before
		gs.Unlock()
		if !removed {
			respond(s, i, fmt.Sprintf("<@&%s> was not in the runtime allowed-roles list. (Roles set in config.json must be removed there.)", role.ID), true)
			return
		}
		_ = gs.Save()
		respond(s, i, fmt.Sprintf("✅ <@&%s> can no longer post links.", role.ID), true)

	case "message":
		text := ""
		if o, ok := om["text"]; ok {
			text = strings.TrimSpace(o.StringValue())
		}
		gs.Lock()
		gs.LinkFilter.MessageOverride = text
		gs.Unlock()
		_ = gs.Save()
		if text == "" {
			respond(s, i, "✅ Reset the warning message to the default.", true)
			return
		}
		preview := strings.ReplaceAll(text, "{user}", "<@"+i.Member.User.ID+">")
		respond(s, i, fmt.Sprintf("✅ Warning message updated. Preview:\n%s", preview), true)

	case "list":
		handleLinkFilterList(s, i, h.cfg, gs)

	default:
		respond(s, i, "Unknown linkfilter subcommand.", true)
	}
}

func handleLinkFilterList(s *discordgo.Session, i *discordgo.InteractionCreate, cfg *config.Config, gs *config.GuildState) {
	roles := config.MergedLinkFilterAllowedRoles(cfg, gs)
	domains := config.MergedLinkFilterWhitelist(cfg, gs)
	sort.Strings(domains)

	var sb strings.Builder
	sb.WriteString("**Link Filter**\n\n**Allowed roles:**\n")
	if len(roles) == 0 {
		sb.WriteString("*(none — nobody is exempt except admins/mods)*\n")
	} else {
		for _, r := range roles {
			sb.WriteString(fmt.Sprintf("• <@&%s>\n", r))
		}
	}
	sb.WriteString("\n**Whitelisted domains:**\n")
	if len(domains) == 0 {
		sb.WriteString("*(none)*\n")
	} else {
		for _, d := range domains {
			sb.WriteString(fmt.Sprintf("• `%s`\n", d))
		}
	}
	msg := config.EffectiveLinkFilterMessage(cfg, gs)
	if msg == "" {
		msg = lang.T("linkfilter_warning")
	}
	sb.WriteString(fmt.Sprintf("\n**Warning message:**\n%s", msg))
	respond(s, i, sb.String(), true)
}

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func removeStr(list []string, v string) []string {
	out := list[:0]
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
