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
	}
	gs.GitHubSubscriptions = append(gs.GitHubSubscriptions, config.GitHubSubscription{
		Repo:      repo,
		ChannelID: channel.ID,
		Events:    events,
	})
	gs.Unlock()
	_ = gs.Save()

	respond(s, i, fmt.Sprintf("✅ Subscribed to `%s` → <#%s>\nEvents: `%s`", repo, channel.ID, strings.Join(events, ", ")), true)
}

func handleGithubUnsubscribe(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	repo := strings.ToLower(strings.TrimSpace(om["repo"].StringValue()))

	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	found := false
	subs := gs.GitHubSubscriptions
	for idx, sub := range subs {
		if sub.Repo == repo {
			gs.GitHubSubscriptions = append(subs[:idx], subs[idx+1:]...)
			found = true
			break
		}
	}
	gs.Unlock()
	_ = gs.Save()

	if !found {
		respond(s, i, fmt.Sprintf("❌ No subscription found for `%s`.", repo), true)
		return
	}
	respond(s, i, fmt.Sprintf("✅ Unsubscribed from `%s`.", repo), true)
}

func handleGithubList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	subs := make([]config.GitHubSubscription, len(gs.GitHubSubscriptions))
	copy(subs, gs.GitHubSubscriptions)
	gs.Unlock()

	if len(subs) == 0 {
		respond(s, i, "No GitHub subscriptions configured. Use `/github subscribe` to add one.", true)
		return
	}

	var sb strings.Builder
	sb.WriteString("**GitHub Subscriptions**\n\n")
	for _, sub := range subs {
		sb.WriteString(fmt.Sprintf("• `%s` → <#%s>\n  Events: `%s`\n\n", sub.Repo, sub.ChannelID, strings.Join(sub.Events, ", ")))
	}
	respond(s, i, sb.String(), true)
}

// ── Webhook server ────────────────────────────────────────────────────────────

func StartGitHubWebhookServer(session *discordgo.Session, cfg *config.GitHubConfig, port int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/github/webhook", func(w http.ResponseWriter, r *http.Request) {
		handleGitHubWebhook(session, cfg, w, r)
	})
	addr := fmt.Sprintf(":%d", port)
	slog.Info("github webhook server listening", "addr", addr)
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			slog.Error("github webhook server stopped", "error", err)
		}
	}()
}

func handleGitHubWebhook(session *discordgo.Session, cfg *config.GitHubConfig, w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	if cfg.WebhookSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifyGitHubSignature(cfg.WebhookSecret, sig, body) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	event := r.Header.Get("X-GitHub-Event")
	if event == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	repo := ghRepoName(payload)
	if repo == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	embed := buildGitHubEmbed(event, payload)
	if embed == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	repoLower := strings.ToLower(repo)

	// Config-file subscriptions (no slash commands needed)
	for _, sub := range cfg.Subscriptions {
		if strings.ToLower(sub.Repo) != repoLower {
			continue
		}
		if !ghEventAllowed(event, sub.Events) {
			continue
		}
		if _, err := session.ChannelMessageSendEmbed(sub.ChannelID, embed); err != nil {
			slog.Warn("github: failed to send config notification", "channel", sub.ChannelID, "error", err)
		}
	}

	// Per-guild subscriptions added via /github subscribe
	for _, gs := range storage.GetAllGuildStates() {
		gs.Lock()
		subs := make([]config.GitHubSubscription, len(gs.GitHubSubscriptions))
		copy(subs, gs.GitHubSubscriptions)
		gs.Unlock()

		for _, sub := range subs {
			if sub.Repo != repoLower {
				continue
			}
			if !ghEventAllowed(event, sub.Events) {
				continue
			}
			if _, err := session.ChannelMessageSendEmbed(sub.ChannelID, embed); err != nil {
				slog.Warn("github: failed to send notification", "channel", sub.ChannelID, "error", err)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func verifyGitHubSignature(secret, sigHeader string, body []byte) bool {
	if !strings.HasPrefix(sigHeader, "sha256=") {
		return false
	}
	got := strings.TrimPrefix(sigHeader, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(got), []byte(want))
}

func ghRepoName(p map[string]interface{}) string {
	if r, ok := p["repository"].(map[string]interface{}); ok {
		if name, ok := r["full_name"].(string); ok {
			return name
		}
	}
	return ""
}

func ghEventAllowed(event string, allowed []string) bool {
	for _, a := range allowed {
		if a == event {
			return true
		}
	}
	return false
}

func ghStr(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func ghSender(p map[string]interface{}) (login, avatar, url string) {
	if s, ok := p["sender"].(map[string]interface{}); ok {
		login = ghStr(s, "login")
		avatar = ghStr(s, "avatar_url")
		url = ghStr(s, "html_url")
	}
	return
}

func buildGitHubEmbed(event string, p map[string]interface{}) *discordgo.MessageEmbed {
	switch event {
	case "push":
		return ghPushEmbed(p)
	case "pull_request":
		return ghPREmbed(p)
	case "create":
		return ghCreateEmbed(p)
	case "delete":
		return ghDeleteEmbed(p)
	case "release":
		return ghReleaseEmbed(p)
	}
	return nil
}

func ghPushEmbed(p map[string]interface{}) *discordgo.MessageEmbed {
	repo := ghRepoName(p)
	repoURL, _ := p["repository"].(map[string]interface{})
	var repoHTMLURL string
	if repoURL != nil {
		repoHTMLURL, _ = repoURL["html_url"].(string)
	}
	ref, _ := p["ref"].(string)
	branch := strings.TrimPrefix(ref, "refs/heads/")
	compareURL, _ := p["compare"].(string)
	login, avatar, userURL := ghSender(p)

	commits, _ := p["commits"].([]interface{})

	var sb strings.Builder
	shown := 0
	for _, c := range commits {
		if shown >= 5 {
			sb.WriteString(fmt.Sprintf("*...and %d more*\n", len(commits)-5))
			break
		}
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		msg, _ := cm["message"].(string)
		msg = strings.SplitN(msg, "\n", 2)[0]
		if len(msg) > 72 {
			msg = msg[:72] + "…"
		}
		id, _ := cm["id"].(string)
		cURL, _ := cm["url"].(string)
		if len(id) > 7 {
			id = id[:7]
		}
		sb.WriteString(fmt.Sprintf("[`%s`](%s) %s\n", id, cURL, msg))
		shown++
	}

	url := compareURL
	if url == "" {
		url = repoHTMLURL
	}

	return &discordgo.MessageEmbed{
		Author:      &discordgo.MessageEmbedAuthor{Name: login, URL: userURL, IconURL: avatar},
		Title:       fmt.Sprintf("[%s:%s] %d new commit(s)", repo, branch, len(commits)),
		URL:         url,
		Description: sb.String(),
		Color:       0x7289DA,
		Footer:      &discordgo.MessageEmbedFooter{Text: repo},
	}
}

func ghPREmbed(p map[string]interface{}) *discordgo.MessageEmbed {
	action, _ := p["action"].(string)
	pr, ok := p["pull_request"].(map[string]interface{})
	if !ok {
		return nil
	}
	repo := ghRepoName(p)
	title, _ := pr["title"].(string)
	prURL, _ := pr["html_url"].(string)
	body, _ := pr["body"].(string)
	number, _ := pr["number"].(float64)
	merged, _ := pr["merged"].(bool)
	login, avatar, userURL := ghSender(p)

	color := 0xFEE75C
	emoji := "🔄"
	label := action

	switch {
	case action == "opened":
		color = 0x57F287
		emoji = "🟢"
	case action == "closed" && merged:
		color = 0x6F42C1
		emoji = "🔀"
		label = "merged"
	case action == "closed":
		color = 0xED4245
		emoji = "🔴"
		label = "closed"
	}