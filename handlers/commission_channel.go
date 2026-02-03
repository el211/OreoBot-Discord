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
						Style:    discordgo.SuccessButton,
						CustomID: "commission_invoice_btn:" + ch.ID,
						Emoji:    &discordgo.ComponentEmoji{Name: "🧾"},
					},
					discordgo.Button{
						Label:    "Close Commission",
						Style:    discordgo.DangerButton,
						CustomID: "commission_close_btn",
						Emoji:    &discordgo.ComponentEmoji{Name: "🔒"},
					},
				},
			},
		},
	})
	if err != nil {
		slog.Error("commission failed to send details message", "channel_id", ch.ID, "error", err)
	} else {
		if pinErr := s.ChannelMessagePin(ch.ID, detailsMsg.ID); pinErr != nil {
			slog.Error("commission failed to pin message", "channel_id", ch.ID, "error", pinErr)
		}
	}

	logCh := config.EffectiveCommissionLogChannel(cfg, gs)
	if logCh != "" {
		logEmbed := &discordgo.MessageEmbed{
			Title: fmt.Sprintf("New Commission for %s", serviceDisplay),
			Color: 0x57F287,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Client", Value: fmt.Sprintf("<@%s>", userID), Inline: true},
				{Name: "Service", Value: serviceDisplay, Inline: true},
				{Name: "Access", Value: "No freelancer selected yet", Inline: true},
				{Name: "Budget", Value: safeEmbedValue(budget), Inline: true},
				{Name: "Timeframe", Value: safeEmbedValue(timeline), Inline: true},
				{Name: "Project Description", Value: safeEmbedValue(details), Inline: false},
				{Name: "Additional Notes", Value: safeEmbedValue(notesField), Inline: false},
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}
		logPing := ""
		for _, roleID := range staffRoles {
			logPing += fmt.Sprintf("<@&%s> ", roleID)
		}
		logMsg, err := s.ChannelMessageSendComplex(logCh, &discordgo.MessageSend{
			Content:    strings.TrimSpace(logPing),
			Embeds:     []*discordgo.MessageEmbed{logEmbed},
			Components: commissionLogComponents(i.GuildID, ch.ID, "", true, false),
		})
		if err != nil {
			slog.Error("commission failed to post log message", "channel_id", ch.ID, "error", err)
		} else {
			ct.LogChannelID = logCh
			ct.LogMessageID = logMsg.ID
			thread, threadErr := s.MessageThreadStartComplex(logCh, logMsg.ID, &discordgo.ThreadStart{
				Name:                fmt.Sprintf("commission-%04d-discussion", num),
				AutoArchiveDuration: 10080,
			})
			if threadErr != nil {
				slog.Error("commission failed to create discussion thread", "channel_id", ch.ID, "error", threadErr)
			} else {
				ct.DiscussionThreadID = thread.ID
				_, _ = s.ChannelMessageSend(thread.ID, formatCommissionInitialThreadMessage(&ct))
				components := commissionLogComponents(i.GuildID, ch.ID, thread.ID, true, false)
				_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: logMsg.ID, Channel: logCh, Components: &components})
			}
			gs.Lock()
			gs.CommissionsRuntime.OpenCommissions[ch.ID] = ct
			gs.Unlock()
			_ = gs.Save()
		}
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("✅ Your commission ticket has been opened: <#%s>\n\nOur team will review your order and get back to you shortly!", ch.ID),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func handleCommissionCloseButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	_, ok := gs.CommissionsRuntime.OpenCommissions[i.ChannelID]
	gs.Unlock()
	if !ok {
		respond(s, i, "❌ This is not an active commission channel.", true)
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Are you sure you want to close this commission? The channel will be deleted.",
			Flags:   discordgo.MessageFlagsEphemeral,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{Label: "Yes, close it", Style: discordgo.DangerButton, CustomID: "commission_close_confirm"},
						discordgo.Button{Label: "Cancel", Style: discordgo.SecondaryButton, CustomID: "commission_close_cancel"},
					},
				},
			},
		},
	})
}

func handleCommissionCloseConfirm(s *discordgo.Session, i *discordgo.InteractionCreate) {
	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	ct, ok := gs.CommissionsRuntime.OpenCommissions[i.ChannelID]
	gs.Unlock()
	if !ok {
		respond(s, i, "❌ Commission not found.", true)
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "🔒 Closing commission..."},
	})

	go closeCommissionChannel(s, i.GuildID, i.ChannelID, i.Member.User, &ct, gs)
}

func handleCommissionCloseCancel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    "Cancelled. Commission is still open.",
			Components: []discordgo.MessageComponent{},
		},
	})
}

func closeCommissionChannel(s *discordgo.Session, guildID, channelID string, closedBy *discordgo.User, ct *config.CommissionTicket, gs *config.GuildState) {
	cfg := storage.Cfg
	transcript := generateTranscript(s, channelID)

	logCh := config.EffectiveCommissionLogChannel(cfg, gs)
	if logCh != "" {
		embed := &discordgo.MessageEmbed{
			Title: fmt.Sprintf("Commission #%04d Closed", ct.Number),
			Color: 0xED4245,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Client", Value: fmt.Sprintf("<@%s>", ct.UserID), Inline: true},
				{Name: "Closed By", Value: fmt.Sprintf("<@%s>", closedBy.ID), Inline: true},
				{Name: "Service", Value: ct.ServiceName, Inline: true},
				{Name: "Budget", Value: ct.Budget, Inline: true},
				{Name: "Timeline", Value: ct.Timeline, Inline: true},
				{Name: "Opened At", Value: ct.CreatedAt, Inline: true},
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}
		_, _ = s.ChannelMessageSendComplex(logCh, &discordgo.MessageSend{
			Embeds: []*discordgo.MessageEmbed{embed},
			Files: []*discordgo.File{
				{
					Name:        fmt.Sprintf("commission-%04d-transcript.txt", ct.Number),
					ContentType: "text/plain",
					Reader:      strings.NewReader(transcript),
				},
			},
		})
	}

	gs.Lock()
	delete(gs.CommissionsRuntime.OpenCommissions, channelID)
	gs.Unlock()
	_ = gs.Save()

	time.Sleep(3 * time.Second)
	_, _ = s.ChannelDelete(channelID)
}
