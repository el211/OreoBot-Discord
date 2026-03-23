package music

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"discord-bot/config"

	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/websocket"
)

type LavalinkBackend struct {
	host     string
	port     int
	password string
	secure   bool
	session  *discordgo.Session

	mu               sync.Mutex
	stopFlag         bool
	volume           int
	currentGuildID   string
	currentChannelID string

	wsMu      sync.RWMutex
	ws        *websocket.Conn
	llSession string
}

func NewLavalinkBackend(cfg *config.LavalinkMusicConfig, s *discordgo.Session) (*LavalinkBackend, error) {
	lb := &LavalinkBackend{
		host:     cfg.Host,
		port:     cfg.Port,
		password: cfg.Password,
		secure:   cfg.Secure,
		session:  s,
		volume:   50,
	}

	var pingErr error
	for attempt := 1; attempt <= 15; attempt++ {
		pingErr = lb.ping()
		if pingErr == nil {
			break
		}
		slog.Warn("waiting for lavalink server", "host", cfg.Host, "port", cfg.Port, "attempt", attempt, "error", pingErr)
		time.Sleep(2 * time.Second)
	}
	if pingErr != nil {
		return nil, fmt.Errorf("cannot reach Lavalink at %s:%d after 30s: %w", cfg.Host, cfg.Port, pingErr)
	}

	if err := lb.connectLLWS(); err != nil {
		return nil, fmt.Errorf("lavalink websocket connect failed: %w", err)
	}

	return lb, nil
}

func (l *LavalinkBackend) Name() string { return "lavalink" }

func (l *LavalinkBackend) SetChannelID(channelID string) {
	l.mu.Lock()
	l.currentChannelID = channelID
	l.mu.Unlock()
}

func (l *LavalinkBackend) baseURL() string {
	scheme := "http"
	if l.secure {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, l.host, l.port)
}

func (l *LavalinkBackend) llWSURL() string {
	scheme := "ws"
	if l.secure {
		scheme = "wss"
	}
	return fmt.Sprintf("%s://%s:%d/v4/websocket", scheme, l.host, l.port)
}

func (l *LavalinkBackend) ping() error {
	req, _ := http.NewRequest("GET", l.baseURL()+"/version", nil)
	req.Header.Set("Authorization", l.password)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)