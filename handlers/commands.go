package handlers

import (
	"log/slog"
	"strconv"
	"strings"

	"discord-bot/config"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) Commands() []*discordgo.ApplicationCommand {
	cmds := make([]*discordgo.ApplicationCommand, 0)
	cmds = append(cmds, moderationCommands()...)
	cmds = append(cmds, ticketCommands()...)
	cmds = append(cmds, commissionCommands()...)
	cmds = append(cmds, utilityCommands()...)
	cmds = append(cmds, autoroleCommands()...)
	cmds = append(cmds, giveawayCommands()...)
	cmds = append(cmds, inviteCommands()...)
	cmds = append(cmds, nopingCommands()...)
	cmds = append(cmds, linkfilterCommands()...)
	cmds = append(cmds, verifyCommands(h.cfg)...)
	if h.cfg.Minecraft.Enabled {
		cmds = append(cmds, minecraftCommands()...)
	}
	if h.cfg.Music.Enabled {
		cmds = append(cmds, musicCommands()...)
	}
	cmds = append(cmds, githubCommands()...)
	cmds = append(cmds, buildCustomCommands(h.cfg)...)
	return cmds
}

func buildCustomCommands(cfg *config.Config) []*discordgo.ApplicationCommand {
	cmds := make([]*discordgo.ApplicationCommand, 0, len(cfg.CustomCommands))
	for _, cc := range cfg.CustomCommands {
		if cc.Name == "" || cc.Message == "" {
			continue
		}
		desc := cc.Description
		if desc == "" {
			desc = cc.Name
		}
		cmds = append(cmds, &discordgo.ApplicationCommand{
			Name:        strings.ToLower(cc.Name),
			Description: desc,
		})
	}
	return cmds
}

func (h *Handler) Register(s *discordgo.Session) {
	s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		handleCommissionMessageCreate(s, m)
	})

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.GuildID == "" {
			return
		}

		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			h.handleSlashCommand(s, i)
		case discordgo.InteractionMessageComponent:
			handleComponent(s, i)
		case discordgo.InteractionApplicationCommandAutocomplete:
			h.handleAutocomplete(s, i)
		case discordgo.InteractionModalSubmit:
			handleModal(s, i)
		}
	})
}

func (h *Handler) RegisterCustomCommands() {
	h.customCmdsMu.Lock()
	defer h.customCmdsMu.Unlock()
	h.customCmds = make(map[string]config.CustomCommandConfig, len(h.cfg.CustomCommands))
	for _, cc := range h.cfg.CustomCommands {
		if cc.Name != "" && cc.Message != "" {
			h.customCmds[strings.ToLower(cc.Name)] = cc
		}
	}
	slog.Info("custom commands registered", "count", len(h.customCmds))
}

func (h *Handler) handleSlashCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	name := i.ApplicationCommandData().Name

	switch name {
	case "ban":
		handleBan(s, i)
	case "unban":
		handleUnban(s, i)
	case "kick":
		handleKick(s, i)
	case "mute":
		handleMute(s, i)
	case "unmute":
		handleUnmute(s, i)
	case "warn":
		handleWarn(s, i)
	case "warnings":
		handleWarnings(s, i)
	case "clearwarnings":
		handleClearWarnings(s, i)
	case "purge", "clear":
		handlePurge(s, i)
	case "slowmode":
		handleSlowmode(s, i)
	case "lock":
		handleLock(s, i)
	case "unlock":
		handleUnlock(s, i)
	case "modlog":
		handleModlog(s, i)
	case "userinfo":
		handleUserinfo(s, i)

	case "ticket":
		handleTicketCommand(s, i)
	case "close":
		handleCloseCommand(s, i)
	case "add":
		handleAddUser(s, i)
	case "remove":
		handleRemoveUser(s, i)

	case "commission":
		handleCommissionCommand(s, i)
	case "invoice":
		handleInvoiceCommand(s, i)

	case "mc":
		h.handleMinecraftCommand(s, i)

	case "say":
		handleSay(s, i)
	case "embed":
		handleEmbed(s, i)
	case "renamechannel":
		handleRenameChannel(s, i)

	case "play", "skip", "stop", "queue", "volume", "nowplaying", "pause", "resume":
		h.handleMusicCommand(s, i, name)

	case "joinrole":
		h.handleJoinRoleCommand(s, i)
	case "rolemenu":
		h.handleRoleMenuCommand(s, i)
	case "giveaway":
		h.handleGiveawayCommand(s, i)

	case "invites":
		h.handleInvitesCommand(s, i)
	case "resetinvites":
		h.handleResetInvitesCommand(s, i)

	case "verify":
		h.handleVerify(s, i)

	case "noping":
		h.handleNoPingCommand(s, i)

	case "linkfilter":
		h.handleLinkFilterCommand(s, i)

	case "github":
		handleGithubCommand(s, i)

	default:
		if cc, ok := h.lookupCustomCommand(name); ok {
			respond(s, i, resolveCustomPlaceholders(s, i, cc.Message), cc.Ephemeral)
			return
		}
		slog.Warn("unknown command", "name", name)
	}
}

func (h *Handler) handleAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.ApplicationCommandData().Name {
	case "verify":
		h.handleVerifyAutocomplete(s, i)
	case "invoice":
		handleInvoiceCurrencyAutocomplete(s, i)
	}
}

func handleComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	if strings.HasPrefix(customID, "rolemenu:") {
		HandleRoleMenuButton(s, i)
		return
	}
	if strings.HasPrefix(customID, "giveaway_enter:") {
		HandleGiveawayEnter(s, i)
		return
	}
	if strings.HasPrefix(customID, "giveaway_ended_") {
		return
	}
	if strings.HasPrefix(customID, "commission_invoice_btn:") {
		handleCommissionInvoiceButton(s, i)
		return
	}
	if strings.HasPrefix(customID, "commission_quote_btn:") {
		handleCommissionQuoteButton(s, i)
		return
	}
	if strings.HasPrefix(customID, "commission_quote_accept:") {
		handleCommissionQuoteAccept(s, i)
		return
	}
	if strings.HasPrefix(customID, "commission_quote_decline:") {
		handleCommissionQuoteDecline(s, i)
		return
	}

	switch customID {
	case "ticket_category_select":
		handleTicketCategorySelect(s, i)
	case "ticket_subcategory_select":
		handleTicketSubcategorySelect(s, i)
	case "ticket_close_btn":
		handleCloseButton(s, i)
	case "ticket_close_confirm":
		handleCloseConfirm(s, i)
	case "ticket_close_cancel":
		handleCloseCancel(s, i)

	case "commission_order":
		handleCommissionOrder(s, i)
	case "commission_service_select":
		handleCommissionServiceSelect(s, i)
	case "commission_close_btn":
		handleCommissionCloseButton(s, i)
	case "commission_close_confirm":
		handleCommissionCloseConfirm(s, i)
	case "commission_close_cancel":
		handleCommissionCloseCancel(s, i)

	default:
		slog.Warn("unknown component", "custom_id", customID)
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "⚠️ This button is no longer valid. Please use the latest panel.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
}

func handleModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.ModalSubmitData().CustomID
	if strings.HasPrefix(customID, "commission_form:") {
		handleCommissionFormSubmit(s, i)
		return
	}
	if strings.HasPrefix(customID, "commission_invoice_modal:") {
		handleCommissionInvoiceModalSubmit(s, i)
		return
	}
	if strings.HasPrefix(customID, "commission_quote_modal:") {
		handleCommissionQuoteModalSubmit(s, i)
		return
	}
	if strings.HasPrefix(customID, "commission_quote_decline_modal:") {
		handleCommissionQuoteDeclineModalSubmit(s, i)
		return
	}
	slog.Warn("unknown modal", "custom_id", customID)
}

func respond(s *discordgo.Session, i *discordgo.InteractionCreate, content string, ephemeral bool) {
	flags := discordgo.MessageFlags(0)
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   flags,
		},
	})
	if err != nil {
		slog.Error("failed to respond", "error", err)
	}
}

func respondEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, ephemeral bool) {
	flags := discordgo.MessageFlags(0)
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  flags,
		},
	})
}

func followup(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: content,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
}

func optionMap(i *discordgo.InteractionCreate) map[string]*discordgo.ApplicationCommandInteractionDataOption {
	m := make(map[string]*discordgo.ApplicationCommandInteractionDataOption)
	for _, opt := range i.ApplicationCommandData().Options {
		m[opt.Name] = opt
	}
	return m
}

func subOptMap(opts []*discordgo.ApplicationCommandInteractionDataOption) map[string]*discordgo.ApplicationCommandInteractionDataOption {
	m := make(map[string]*discordgo.ApplicationCommandInteractionDataOption)
	for _, opt := range opts {
		m[opt.Name] = opt
	}
	return m
}

func optStr(m map[string]*discordgo.ApplicationCommandInteractionDataOption, key, def string) string {
	if o, ok := m[key]; ok {
		return o.StringValue()
	}
	return def
}

func optInt(m map[string]*discordgo.ApplicationCommandInteractionDataOption, key string, def int64) int64 {
	if o, ok := m[key]; ok {
		return o.IntValue()
	}
	return def
}

func applyPlaceholders(msg, userMention, username, guildName string, memberCount int) string {
	msg = strings.ReplaceAll(msg, "{user}", userMention)
	msg = strings.ReplaceAll(msg, "{username}", username)
	msg = strings.ReplaceAll(msg, "{server}", guildName)
	msg = strings.ReplaceAll(msg, "{member_count}", strconv.Itoa(memberCount))
	return msg
}

func resolveCustomPlaceholders(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) string {
	if i.Member == nil || i.Member.User == nil {
		return msg
	}

	userMention := "<@" + i.Member.User.ID + ">"
	username := i.Member.User.Username
	guildName := ""
	memberCount := 0

	if guild, err := s.Guild(i.GuildID); err == nil {
		guildName = guild.Name
		memberCount = guild.MemberCount
	}

	return applyPlaceholders(msg, userMention, username, guildName, memberCount)
}

func hasConfigRole(s *discordgo.Session, guildID string, member *discordgo.Member, allowedNames []string) bool {
	if member == nil || len(allowedNames) == 0 {
		return false
	}

	roles, err := s.GuildRoles(guildID)
	if err != nil {
		return false
	}

	nameSet := make(map[string]bool, len(allowedNames))
	for _, n := range allowedNames {
		nameSet[strings.ToLower(n)] = true
	}

	for _, role := range roles {
		if nameSet[strings.ToLower(role.Name)] {
			for _, memberRoleID := range member.Roles {
				if memberRoleID == role.ID {
					return true
				}
			}
		}
	}
	return false
}

func (h *Handler) isAdmin(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if i.Member.Permissions&discordgo.PermissionAdministrator != 0 {
		return true
	}
	return hasConfigRole(s, i.GuildID, i.Member, h.cfg.Permissions.AdminRoles)
}

func (h *Handler) isModerator(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if h.isAdmin(s, i) {
		return true
	}
	if i.Member.Permissions&discordgo.PermissionBanMembers != 0 {
		return true
	}
	return hasConfigRole(s, i.GuildID, i.Member, h.cfg.Permissions.ModeratorRoles)
}

func (h *Handler) isDJ(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if h.isModerator(s, i) {
		return true
	}
	return hasConfigRole(s, i.GuildID, i.Member, h.cfg.Permissions.DJRoles)
}
