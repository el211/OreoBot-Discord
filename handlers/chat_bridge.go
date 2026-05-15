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