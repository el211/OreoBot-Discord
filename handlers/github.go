package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"discord-bot/config"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

func githubCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:                     "github",
			Description:              "Manage GitHub repository notifications",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "subscribe",
					Description: "Subscribe a channel to a GitHub repository's events",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "repo", Description: "Repository in owner/repo format", Required: true},
						{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel to post notifications in", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "events", Description: "Comma-separated: push,pull_request,create,delete,release (default: all)"},
					},
				},
				{
					Name:        "unsubscribe",
					Description: "Remove a GitHub repository subscription",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "repo", Description: "Repository in owner/repo format", Required: true},
					},
				},
				{
					Name:        "list",
					Description: "List all GitHub repository subscriptions",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
	}
}

func handleGithubCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	sub := i.ApplicationCommandData().Options[0]
	switch sub.Name {
	case "subscribe":
		handleGithubSubscribe(s, i, sub.Options)
	case "unsubscribe":
		handleGithubUnsubscribe(s, i, sub.Options)
	case "list":
		handleGithubList(s, i)
	}
}

func handleGithubSubscribe(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	repo := strings.ToLower(strings.TrimSpace(om["repo"].StringValue()))
	channel := om["channel"].ChannelValue(s)

	if !strings.Contains(repo, "/") {
		respond(s, i, "❌ Repository must be in `owner/repo` format.", true)
		return
	}

	var events []string
	if ev, ok := om["events"]; ok {
		for _, e := range strings.Split(ev.StringValue(), ",") {
			e = strings.TrimSpace(strings.ToLower(e))
			if e != "" {
				events = append(events, e)
			}
		}
	}
	if len(events) == 0 {
		events = []string{"push", "pull_request", "create", "delete", "release"}
	}

	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	// Replace existing subscription for same repo
	subs := gs.GitHubSubscriptions
	for idx, sub := range subs {
		if sub.Repo == repo {
			gs.GitHubSubscriptions = append(subs[:idx], subs[idx+1:]...)
			break
		}