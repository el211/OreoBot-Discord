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

func handleCommissionOrder(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cfg := storage.Cfg
	gs := storage.GetGuild(i.GuildID)

	gs.Lock()
	guildEnabled := gs.CommissionsRuntime.Enabled
	guildConfigured := gs.CommissionsRuntime.PanelChannelOverride != "" ||
		gs.CommissionsRuntime.StaffRolesOverride != "" ||
		len(gs.CommissionsRuntime.OpenCommissions) > 0
	gs.Unlock()

	var enabled bool
	if guildConfigured {
		enabled = guildEnabled
	} else {
		enabled = cfg.Commissions.Enabled
	}

	if !enabled {
		respond(s, i, "🔒 Commissions are currently **closed**. Please check back later!", true)
		return
	}

	services := config.MergedCommissionServices(cfg, gs)
	if len(services) == 0 {
		respond(s, i, "⚠️ No services are available at the moment. Please check back later!", true)
		return
	}

	opts := make([]discordgo.SelectMenuOption, 0, len(services))
	for _, svc := range services {
		opts = append(opts, discordgo.SelectMenuOption{
			Label:       svc.Name,
			Value:       svc.ID,
			Description: svc.Description,
			Emoji:       parseComponentEmoji(svc.Emoji),
		})
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "**What service are you interested in?**\nSelect a service type below to continue your order.",
			Flags:   discordgo.MessageFlagsEphemeral,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{
							MenuType:    discordgo.StringSelectMenu,
							CustomID:    "commission_service_select",
							Placeholder: "Choose a service...",
							Options:     opts,
						},
					},
				},
			},
		},
	}); err != nil {
		slog.Error("commission order respond error", "error", err)
	}
}

func handleCommissionServiceSelect(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	if len(data.Values) == 0 {
		return
	}
	serviceID := data.Values[0]

	cfg := storage.Cfg
	gs := storage.GetGuild(i.GuildID)
	services := config.MergedCommissionServices(cfg, gs)

	var svc *config.CommissionService
	for idx := range services {
		if services[idx].ID == serviceID {
			svc = &services[idx]
			break
		}
	}
	if svc == nil {
		respond(s, i, "❌ That service no longer exists. Please try again.", true)
		return
	}

	modalTitle := fmt.Sprintf("Order — %s %s", svc.Emoji, svc.Name)
	if len(modalTitle) > 45 {
		modalTitle = modalTitle[:45]
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "commission_form:" + serviceID,
			Title:    modalTitle,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    "details",
						Label:       "What do you want commissioned?",
						Style:       discordgo.TextInputParagraph,
						Required:    true,
						Placeholder: "Describe exactly what you need. The more detail the better!",
						MinLength:   20,
						MaxLength:   1000,
					},
				}},
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    "budget",
						Label:       "Your Budget",
						Style:       discordgo.TextInputShort,
						Required:    true,
						Placeholder: "e.g. $50–$100 or open to quote",
						MaxLength:   100,
					},
				}},
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    "timeline",
						Label:       "Timeline / Deadline",
						Style:       discordgo.TextInputShort,
						Required:    true,
						Placeholder: "e.g. 2 weeks, ASAP, by Dec 1",
						MaxLength:   100,
					},
				}},
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    "notes",
						Label:       "Additional Notes (optional)",
						Style:       discordgo.TextInputParagraph,
						Required:    false,
						Placeholder: "Anything else we should know? References, examples, etc.",
						MaxLength:   500,
					},
				}},
			},
		},
	})
}

func handleCommissionFormSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	serviceID := strings.TrimPrefix(data.CustomID, "commission_form:")

	fields := modalTextValues(data.Components)
	details := strings.TrimSpace(fields["details"])
	budget := strings.TrimSpace(fields["budget"])
	timeline := strings.TrimSpace(fields["timeline"])
	notes := strings.TrimSpace(fields["notes"])

	if details == "" || budget == "" || timeline == "" {
		slog.Warn("commission modal empty fields", "user_id", i.Member.User.ID, "service_id", serviceID)
	}

	cfg := storage.Cfg
	gs := storage.GetGuild(i.GuildID)
	services := config.MergedCommissionServices(cfg, gs)

	var svc *config.CommissionService
	for idx := range services {
		if services[idx].ID == serviceID {
			svc = &services[idx]
			break
		}
	}
	serviceName := serviceID
	serviceEmoji := ""
	if svc != nil {
		serviceName = svc.Name
		serviceEmoji = svc.Emoji
	}

	createCommissionChannel(s, i, serviceID, serviceName, serviceEmoji, details, budget, timeline, notes)
}

func createCommissionChannel(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	serviceID, serviceName, serviceEmoji,
	details, budget, timeline, notes string,
) {
	cfg := storage.Cfg
	gs := storage.GetGuild(i.GuildID)
	userID := i.Member.User.ID

	gs.Lock()
	gs.CommissionsRuntime.CommissionCounter++
	num := gs.CommissionsRuntime.CommissionCounter
	gs.Unlock()

	channelName := fmt.Sprintf("commission-%04d", num)
	discordCat := config.EffectiveCommissionCategory(cfg, gs)
	staffRoles := config.EffectiveCommissionStaffRoles(cfg, gs)

	overwrites := []*discordgo.PermissionOverwrite{
		{ID: i.GuildID, Type: discordgo.PermissionOverwriteTypeRole, Deny: discordgo.PermissionViewChannel},
		{
			ID:    userID,
			Type:  discordgo.PermissionOverwriteTypeMember,
			Allow: discordgo.PermissionViewChannel | discordgo.PermissionSendMessages | discordgo.PermissionAttachFiles | discordgo.PermissionReadMessageHistory,
		},
	}

	ch, err := s.GuildChannelCreateComplex(i.GuildID, discordgo.GuildChannelCreateData{
		Name:                 channelName,
		Type:                 discordgo.ChannelTypeGuildText,
		ParentID:             discordCat,
		PermissionOverwrites: overwrites,
	})
	if err != nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("❌ Failed to create commission channel: %s", err.Error()),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	ct := config.CommissionTicket{
		ChannelID:            ch.ID,
		UserID:               userID,
		ServiceID:            serviceID,
		ServiceName:          serviceName,
		Details:              details,
		Budget:               budget,
		Timeline:             timeline,
		Notes:                notes,
		Number:               num,
		CreatedAt:            time.Now().Format(time.RFC3339),
		ClientThreadMessages: make(map[string]string),
		Quotes:               make(map[string]config.CommissionQuote),
	}

	gs.Lock()
	gs.CommissionsRuntime.OpenCommissions[ch.ID] = ct
	gs.Unlock()
	_ = gs.Save()

	serviceDisplay := serviceName
	if serviceEmoji != "" {
		serviceDisplay = serviceEmoji + " " + serviceName
	}

	notesField := "*None provided*"
	if notes != "" {
		notesField = notes
	}

	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("📋 Commission #%04d", num),
		Color: 0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Client", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
			{Name: "Service", Value: serviceDisplay, Inline: true},
			{Name: "Budget", Value: budget, Inline: true},
			{Name: "Timeline", Value: timeline, Inline: true},
			{Name: "Order Details", Value: details, Inline: false},
			{Name: "Additional Notes", Value: notesField, Inline: false},
		},
		Footer:    &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Commission opened • %s", time.Now().Format("Jan 2, 2006 15:04"))},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	pingContent := fmt.Sprintf("<@%s>", userID)
	for _, roleID := range staffRoles {
		pingContent += fmt.Sprintf(" <@&%s>", roleID)
	}

	detailsMsg, err := s.ChannelMessageSendComplex(ch.ID, &discordgo.MessageSend{
		Content: pingContent,
		Embeds:  []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    "Issue Invoice",