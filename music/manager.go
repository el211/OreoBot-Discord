package music

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"discord-bot/config"

	"github.com/bwmarrin/discordgo"
)

type Song struct {
	Title     string
	URL       string
	StreamURL string
	Duration  int
	AddedBy   string
}

type Backend interface {
	Name() string

	ResolveSong(query string) (*Song, error)

	Play(vc *discordgo.VoiceConnection, song *Song, volume int, done chan<- struct{})
	Stop()
	SetVolume(vol int)
	Cleanup()
}

type GuildPlayer struct {
	mu      sync.Mutex
	GuildID string

	Queue      []*Song
	NowPlaying *Song
	Playing    bool
	Paused     bool
	Volume     int

	VoiceConn      *discordgo.VoiceConnection
	VoiceChannelID string
	backend        Backend
	session        *discordgo.Session
	stopCh         chan struct{}
}

type Manager struct {
	mu      sync.RWMutex
	players map[string]*GuildPlayer
	backend Backend
	session *discordgo.Session
	cfg     *config.MusicConfig
}

func NewManager(s *discordgo.Session, cfg *config.MusicConfig) (*Manager, error) {
	var b Backend
	var err error

	switch cfg.Backend {
	case "direct":
		b, err = NewDirectBackend(&cfg.Direct)
		if err != nil {
			return nil, fmt.Errorf("direct backend: %w", err)
		}
		slog.Info("using direct backend", "ytdlp", cfg.Direct.YTDLPPath, "ffmpeg", cfg.Direct.FFmpegPath)

	case "lavalink":
		b, err = NewLavalinkBackend(&cfg.Lavalink, s)
		if err != nil {
			return nil, fmt.Errorf("lavalink backend: %w", err)
		}
		slog.Info("using lavalink backend", "host", cfg.Lavalink.Host, "port", cfg.Lavalink.Port)

	default:
		return nil, fmt.Errorf("unknown music backend: %q (use \"direct\" or \"lavalink\")", cfg.Backend)
	}

	return &Manager{
		players: make(map[string]*GuildPlayer),
		backend: b,
		session: s,
		cfg:     cfg,
	}, nil
}

func (m *Manager) GetPlayer(guildID string) *GuildPlayer {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.players[guildID]
	if !ok {
		p = &GuildPlayer{
			GuildID: guildID,
			Volume:  m.cfg.DefaultVolume,