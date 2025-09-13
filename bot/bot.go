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
					wait := time.Duration(attempt*5) * time.Second
					slog.Info("reconnect attempt", "attempt", attempt, "of", maxReconnectAttempts, "wait", wait)
					time.Sleep(wait)
					if err = b.Session.Open(); err == nil {
						slog.Info("watchdog reconnect successful")
						break
					}
					slog.Warn("reconnect attempt failed", "attempt", attempt, "error", err)
				}

				if err != nil {
					slog.Error("all reconnect attempts failed, exiting", "attempts", maxReconnectAttempts)
					os.Exit(1)
				}
			}
		}
	}
}

func (b *Bot) Stop() {
	b.mu.Lock()
	b.stopping = true
	b.mu.Unlock()
	_ = b.Session.Close()
}

func (b *Bot) readyOpen() bool {
	select {
	case <-b.ready:
		return false
	default:
		return true
	}
}

func (b *Bot) markDisconnected() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.seenDisconnect || b.stopping {
		return
	}

	b.seenDisconnect = true
	b.disconnectAt = time.Now()
	slog.Warn("gateway disconnected, waiting for automatic reconnect")
}

func (b *Bot) consumeDisconnect(prefix string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.seenDisconnect {
		return false
	}

	elapsed := time.Since(b.disconnectAt).Round(time.Second)
	b.seenDisconnect = false
	b.disconnectAt = time.Time{}
	slog.Info("gateway reconnected", "event", prefix, "downtime", elapsed)
	return true
}

func (b *Bot) RegisterCommands(cmds []*discordgo.ApplicationCommand) []*discordgo.ApplicationCommand {
	select {
	case <-b.ready:
	case <-time.After(60 * time.Second):
		slog.Error("timed out waiting for Discord Ready event after 60s")
		os.Exit(1)
	}

	appID := b.Session.State.User.ID
	guildID := b.Config.Discord.GuildID

	slog.Info("registering commands", "count", len(cmds), "app", appID, "guild", guildID)

	registered, err := b.Session.ApplicationCommandBulkOverwrite(appID, guildID, cmds)
	if err != nil {
		slog.Error("failed to bulk-overwrite commands", "error", err)
		return nil
	}

	slog.Info("slash commands registered", "count", len(registered))
	return registered
}

func (b *Bot) CleanupCommands(_ []*discordgo.ApplicationCommand) {
	<-b.ready
	appID := b.Session.State.User.ID
	guildID := b.Config.Discord.GuildID
	if _, err := b.Session.ApplicationCommandBulkOverwrite(appID, guildID, []*discordgo.ApplicationCommand{}); err != nil {
		slog.Error("failed to clean up commands", "error", err)
		return
	}
	slog.Info("cleaned up all slash commands")
}
