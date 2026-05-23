package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"discord-bot/config"
	"discord-bot/minecraft"

	"github.com/bwmarrin/discordgo"
	amqp "github.com/rabbitmq/amqp091-go"
)

const bridgeOriginID = "DISCORD"

var ActiveBridge *ChatBridge

type ChatBridge struct {
	session *discordgo.Session
	cfg     *config.ChatBridgeConfig
	mcStore MCLinkStore
	rcon    *minecraft.Client

	pubConn *amqp.Connection
	pubCh   *amqp.Channel

	subConn *amqp.Connection
	subCh   *amqp.Channel
}

func NewChatBridge(s *discordgo.Session, cfg *config.ChatBridgeConfig, mcStore MCLinkStore, rcon *minecraft.Client) (*ChatBridge, error) {
	b := &ChatBridge{session: s, cfg: cfg, mcStore: mcStore, rcon: rcon}
	if err := b.connectPublisher(); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *ChatBridge) Start() {
	ActiveBridge = b

	b.session.AddHandler(b.onDiscordMessage)

	if b.cfg.BanSync {
		b.session.AddHandler(b.onGuildBanAdd)
		b.session.AddHandler(b.onGuildBanRemove)
	}

	go b.consumeLoop()
	slog.Info("chatbridge started", "channel_id", b.cfg.ChannelID)
}

func (b *ChatBridge) Stop() {
	for _, ch := range []*amqp.Channel{b.pubCh, b.subCh} {
		if ch != nil {
			_ = ch.Close()
		}
	}
	for _, conn := range []*amqp.Connection{b.pubConn, b.subConn} {
		if conn != nil {
			_ = conn.Close()
		}
	}
}

func (b *ChatBridge) onDiscordMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot || m.WebhookID != "" || m.ChannelID != b.cfg.ChannelID {
		return
	}
	if b.mcStore == nil {
		return
	}

	link, err := b.mcStore.LoadLink(m.Author.ID)
	if err != nil || link == nil {
		return
	}

	content := strings.TrimSpace(m.Content)
	if content == "" {
		return
	}

	payload := b.buildMCPayload(link.UUID, link.Username, content)

	if err := b.publish(payload); err != nil {
		slog.Error("chatbridge publish error", "error", err)
		if err2 := b.connectPublisher(); err2 == nil {
			_ = b.publish(payload)
		}
	}
}

func (b *ChatBridge) buildMCPayload(uuid, playerName, content string) string {
	jsonComp := fmt.Sprintf(`{"text":"[Discord] %s: %s"}`, escapeJSON(playerName), escapeJSON(content))

	if b.cfg.MCChannelID != "" {
		return strings.Join([]string{
			"CHANMSG",
			bridgeOriginID,
			uuid,
			b64enc("Discord"),
			b64enc(playerName),
			b64enc(b.cfg.MCChannelID),
			b64enc(jsonComp),
		}, ";;")
	}

	return strings.Join([]string{
		bridgeOriginID,
		uuid,
		b64enc(playerName),
		b64enc(jsonComp),
	}, ";;")
}

func (b *ChatBridge) publish(payload string) error {
	return b.pubCh.Publish("chat_sync", "", false, false, amqp.Publishing{
		ContentType: "text/plain",
		Body:        []byte(payload),
	})
}

func (b *ChatBridge) consumeLoop() {
	for {
		if err := b.connectSubscriber(); err != nil {
			slog.Error("chatbridge sub connect error", "error", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if err := b.consume(); err != nil {
			slog.Error("chatbridge consumer closed", "error", err)
		}
		time.Sleep(5 * time.Second)
	}
}

func (b *ChatBridge) consume() error {
	q, err := b.subCh.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		return fmt.Errorf("queue declare: %w", err)
	}
	if err := b.subCh.QueueBind(q.Name, "", "chat_sync", false, nil); err != nil {
		return fmt.Errorf("queue bind: %w", err)
	}
	msgs, err := b.subCh.Consume(q.Name, "", true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	for msg := range msgs {
		b.handleMCMessage(string(msg.Body))
	}
	return fmt.Errorf("consumer channel closed")
}

func (b *ChatBridge) handleMCMessage(raw string) {
	if strings.HasPrefix(raw, "CTRL;;") || strings.HasPrefix(raw, "CHANSYS;;") {
		return
	}

	var playerName, message string

	if strings.HasPrefix(raw, "CHANMSG;;") {
		parts := strings.SplitN(raw, ";;", 7)
		if len(parts) < 7 {
			return
		}
		if parts[1] == bridgeOriginID {
			return
		}
		channelID := b64dec(parts[5])
		if b.cfg.MCChannelID != "" && !strings.EqualFold(channelID, b.cfg.MCChannelID) {
			return
		}
		playerName = b64dec(parts[4])
		message = adventureToPlain(b64dec(parts[6]))

	} else {
		parts := strings.SplitN(raw, ";;", 6)
		if len(parts) < 4 {
			return
		}
		if parts[0] == bridgeOriginID {
			return
		}
		playerName = b64dec(parts[2])

		if len(parts) == 6 {
			prefix := strings.TrimSpace(b64dec(parts[4]))
			rawMsg := b64dec(parts[5])
			if prefix != "" {
				message = prefix + " " + rawMsg
			} else {
				message = rawMsg
			}
		} else {
			message = adventureToPlain(b64dec(parts[3]))
		}
	}

	if playerName == "" || message == "" {
		return
	}

	discordMention := ""
	if b.mcStore != nil {
		if links, err := b.mcStore.ListLinks(); err == nil {
			for _, l := range links {
				if l.Username == playerName {
					discordMention = fmt.Sprintf(" (<@%s>)", l.DiscordID)
					break
				}
			}
		}
	}

	text := fmt.Sprintf("**%s**%s: %s", playerName, discordMention, message)
	if _, err := b.session.ChannelMessageSend(b.cfg.ChannelID, text); err != nil {
		slog.Error("chatbridge discord send error", "error", err)
	}
}

func (b *ChatBridge) onGuildBanAdd(s *discordgo.Session, e *discordgo.GuildBanAdd) {
	if b.mcStore == nil || b.rcon == nil {
		return
	}
	link, err := b.mcStore.LoadLink(e.User.ID)
	if err != nil || link == nil {
		return
	}
	cmd := fmt.Sprintf("ban %s Banned from Discord", link.Username)
	if _, err := b.rcon.Command(cmd); err != nil {
		slog.Error("chatbridge rcon ban failed", "username", link.Username, "error", err)
	} else {
		slog.Info("chatbridge banned from minecraft", "username", link.Username)
	}
}

func (b *ChatBridge) onGuildBanRemove(s *discordgo.Session, e *discordgo.GuildBanRemove) {
	if b.mcStore == nil || b.rcon == nil {
		return
	}
	link, err := b.mcStore.LoadLink(e.User.ID)
	if err != nil || link == nil {
		return
	}
	cmd := fmt.Sprintf("pardon %s", link.Username)
	if _, err := b.rcon.Command(cmd); err != nil {
		slog.Error("chatbridge rcon pardon failed", "username", link.Username, "error", err)
	} else {
		slog.Info("chatbridge pardoned in minecraft", "username", link.Username)
	}
}

func (b *ChatBridge) SyncMuteToMC(discordUserID string, until time.Time, reason, byUsername string) {
	if !b.cfg.ModSync || b.mcStore == nil {
		return
	}
	link, err := b.mcStore.LoadLink(discordUserID)
	if err != nil || link == nil {
		return
	}
	payload := fmt.Sprintf("CTRL;;MUTE;;%s;;%s;;%d;;%s;;%s",
		bridgeOriginID,
		link.UUID,
		until.UnixMilli(),
		b64enc(reason),
		b64enc(byUsername),
	)
	if err := b.publish(payload); err != nil {
		slog.Error("chatbridge sync mute publish error", "error", err)
	} else {
		slog.Info("chatbridge synced mute to minecraft", "username", link.Username, "until", until.Format(time.RFC3339))
	}
}

func (b *ChatBridge) SyncUnmuteToMC(discordUserID string) {
	if !b.cfg.ModSync || b.mcStore == nil {
		return
	}
	link, err := b.mcStore.LoadLink(discordUserID)
	if err != nil || link == nil {
		return
	}
	payload := fmt.Sprintf("CTRL;;UNMUTE;;%s;;%s", bridgeOriginID, link.UUID)
	if err := b.publish(payload); err != nil {
		slog.Error("chatbridge sync unmute publish error", "error", err)
	} else {
		slog.Info("chatbridge synced unmute to minecraft", "username", link.Username)
	}
}

func (b *ChatBridge) connectPublisher() error {
	conn, err := amqp.Dial(b.cfg.RabbitMQURI)
	if err != nil {
		return fmt.Errorf("pub dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("pub channel: %w", err)
	}
	if err := ch.ExchangeDeclare("chat_sync", "fanout", false, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("exchange declare: %w", err)
	}
	b.pubConn = conn
	b.pubCh = ch
	return nil
}

func (b *ChatBridge) connectSubscriber() error {
	conn, err := amqp.Dial(b.cfg.RabbitMQURI)
	if err != nil {
		return fmt.Errorf("sub dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("sub channel: %w", err)
	}
	if err := ch.ExchangeDeclare("chat_sync", "fanout", false, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("exchange declare: %w", err)
	}
	b.subConn = conn
	b.subCh = ch
	return nil
}

func adventureToPlain(jsonStr string) string {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &obj); err != nil {
		return stripColors(jsonStr)
	}
	return extractAdventureText(obj)
}

func extractAdventureText(obj map[string]interface{}) string {
	var sb strings.Builder
	if t, ok := obj["text"].(string); ok {
		sb.WriteString(t)
	}
	if extra, ok := obj["extra"].([]interface{}); ok {
		for _, e := range extra {
			if child, ok := e.(map[string]interface{}); ok {
				sb.WriteString(extractAdventureText(child))
			}
		}
	}
	return sb.String()
}

func stripColors(s string) string {
	var sb strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '§' {
			i++
			continue
		}
		if runes[i] == '<' {
			if end := strings.IndexRune(string(runes[i:]), '>'); end != -1 {
				i += end
				continue
			}
		}
		sb.WriteRune(runes[i])
	}
	return sb.String()
}

func b64enc(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func b64dec(s string) string {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(b)
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
