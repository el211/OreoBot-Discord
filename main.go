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