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