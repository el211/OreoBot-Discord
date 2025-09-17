package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"discord-bot/bot"
	"discord-bot/config"
	"discord-bot/handlers"
	"discord-bot/lang"
	"discord-bot/minecraft"
	"discord-bot/music"
	"discord-bot/payments"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic", "error", r, "stack", string(debug.Stack()))
			fmt.Fprintf(os.Stderr, "PANIC: %v\n%s\n", r, debug.Stack())
			os.Exit(1)
		}
	}()

	run()
}

const exitCodeScheduledRestart = 2

func run() {
	configPath := flag.String("config", "config.json", "Path to config file")
	cleanup := flag.Bool("cleanup", false, "Remove slash commands on shutdown")
	restartFlag := flag.Int("restart", -1, "Auto-restart interval in minutes (0 = off, -1 = use config value)")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	if cfg.Discord.Token == "" || cfg.Discord.Token == "YOUR_DISCORD_BOT_TOKEN_HERE" {
		slog.Error("set your bot token in config.json → discord.token")
		os.Exit(1)
	}

	lang.Load("lang.yml")

	storage.Cfg = cfg
	storage.LoadAllGuildStates()

	if err := storage.InitDB(&cfg.Database); err != nil {
		slog.Warn("database init failed, falling back to JSON-only storage", "error", err)
	} else {
		defer storage.DB.Close()
	}

	h := handlers.NewHandler(cfg, storage.DB)

	if cfg.Minecraft.Enabled {
		h.InitMCLinkStore()

		rcon := minecraft.NewClient(cfg.Minecraft.RCONAddress, cfg.Minecraft.RCONPort, cfg.Minecraft.RCONPassword)
		if err := rcon.Connect(); err != nil {
			slog.Warn("minecraft rcon connection failed", "error", err)
		} else {
			slog.Info("minecraft rcon connected")
		}
		h.SetRCON(rcon)
	}

	b, err := bot.New(cfg)
	if err != nil {
		slog.Error("failed to create bot", "error", err)
		os.Exit(1)
	}

	h.Register(b.Session)
	h.RegisterWelcomeLeave(b.Session)
	h.RegisterNoPing(b.Session)
	h.RegisterLinkFilter(b.Session)
	h.RegisterCounting(b.Session)
	h.RegisterCustomCommands()
	h.RegisterInviteTracker(b.Session)

	if cfg.ChatBridge.Enabled {
		bridge, err := handlers.NewChatBridge(b.Session, &cfg.ChatBridge, h.GetMCStore(), h.GetRCON())
		if err != nil {
			slog.Warn("chatbridge init failed", "error", err)
		} else {
			bridge.Start()
			defer bridge.Stop()
		}
	}

	b.Session.AddHandler(func(s *discordgo.Session, e *discordgo.VoiceStateUpdate) {
		if s.State != nil && s.State.User != nil && e.UserID == s.State.User.ID {
			music.UpdateVoiceState(e.GuildID, e.SessionID)
		}
	})

	b.Session.AddHandler(func(s *discordgo.Session, e *discordgo.VoiceServerUpdate) {
		endpoint := ""
		if e.Endpoint != nil {
			endpoint = *e.Endpoint
		}
		music.UpdateVoiceServer(e.GuildID, e.Token, endpoint)
	})

	if err := b.Start(); err != nil {
		slog.Error("failed to start bot", "error", err)
		os.Exit(1)
	}
	defer b.Stop()

	var guildID string
	if cfg.Discord.GuildID != "" {
		guildID = cfg.Discord.GuildID
	} else if b.Session.State != nil && len(b.Session.State.Guilds) > 0 {
		guildID = b.Session.State.Guilds[0].ID
	}

	if guildID != "" {
		gs := storage.GetGuild(guildID)
		h.RestoreGiveawayTimers(b.Session, gs)
		slog.Info("giveaway timers restored")
	}

	if cfg.Minecraft.Enabled {
		h.StartLinkPoller(b.Session, guildID)
	}

	if cfg.Music.Enabled {
		mgr, err := music.NewManager(b.Session, &cfg.Music)
		if err != nil {
			slog.Warn("music system init failed", "error", err)
		} else {
			h.SetMusic(mgr)
			defer mgr.Cleanup()
		}
	}

	payments.Start(cfg, b.Session)

	if cfg.GitHub.Enabled {
		port := cfg.GitHub.WebhookPort
		if port == 0 {
			port = 8080
		}
		handlers.StartGitHubWebhookServer(b.Session, &cfg.GitHub, port)
	}

	registered := b.RegisterCommands(h.Commands())

	restartMinutes := cfg.AutoRestartMinutes
	if *restartFlag >= 0 {
		restartMinutes = *restartFlag
	}
	if restartMinutes > 0 {
		slog.Info("auto-restart enabled", "interval_minutes", restartMinutes)
		go func() {
			time.Sleep(time.Duration(restartMinutes) * time.Minute)
			slog.Info("scheduled restart", "interval_minutes", restartMinutes)
			if *cleanup {
				b.CleanupCommands(registered)
			}
			b.Stop()
			os.Exit(exitCodeScheduledRestart)
		}()
	}

	slog.Info("bot is running, press ctrl+c to exit")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down")
	if *cleanup {
		b.CleanupCommands(registered)
	}
}
