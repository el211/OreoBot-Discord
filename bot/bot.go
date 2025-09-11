package bot

import (
	"log/slog"
	"os"
	"sync"
	"time"

	"discord-bot/config"

	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	Session *discordgo.Session
	Config  *config.Config
	ready   chan struct{}

	mu             sync.Mutex
	disconnectAt   time.Time
	seenDisconnect bool
	stopping       bool
}

func New(cfg *config.Config) (*Bot, error) {
	s, err := discordgo.New("Bot " + cfg.Discord.Token)
	if err != nil {
		return nil, err
	}
	s.Identify.Intents = discordgo.IntentsAll
	s.ShouldReconnectOnError = true
	return &Bot{
		Session: s,
		Config:  cfg,
		ready:   make(chan struct{}),
	}, nil
}

func (b *Bot) Start() error {
	b.Session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		if !b.consumeDisconnect("Gateway reconnected with a fresh READY") && b.readyOpen() {
			slog.Info("bot online", "username", r.User.Username, "discriminator", r.User.Discriminator)
		}
		select {
		case <-b.ready:
		default:
			close(b.ready)
		}
	})

	b.Session.AddHandler(func(s *discordgo.Session, _ *discordgo.Disconnect) {
		b.markDisconnected()
	})

	b.Session.AddHandler(func(s *discordgo.Session, r *discordgo.Resumed) {
		b.consumeDisconnect("Gateway session resumed")
	})

	openResult := make(chan error, 1)
	go func() {
		openResult <- b.Session.Open()
	}()

	select {
	case err := <-openResult:
		if err != nil {
			return err
		}
	case <-time.After(60 * time.Second):
		slog.Error("timed out connecting to Discord gateway after 60s")
		os.Exit(1)
	}

	go b.watchdog()
	return nil
}

func (b *Bot) watchdog() {
	const maxReconnectAttempts = 5

	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		b.mu.Lock()
		stopping := b.stopping
		b.mu.Unlock()
		if stopping {
			return
		}

		if b.Session.DataReady {
			last := b.Session.LastHeartbeatAck
			if !last.IsZero() && time.Since(last) > 3*time.Minute {
				slog.Warn("no heartbeat ACK for 3+ minutes, forcing reconnect")
				_ = b.Session.Close()

				var err error
				for attempt := 1; attempt <= maxReconnectAttempts; attempt++ {