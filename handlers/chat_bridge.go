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