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
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Lavalink returned status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

type llReady struct {
	Op        string `json:"op"`
	SessionID string `json:"sessionId"`
}

func (l *LavalinkBackend) connectLLWS() error {
	u := l.llWSURL()

	headers := http.Header{}
	headers.Set("Authorization", l.password)

	if l.session == nil || l.session.State == nil || l.session.State.User == nil {
		return fmt.Errorf("discord session not ready (State.User is nil)")
	}
	headers.Set("User-Id", l.session.State.User.ID)
	headers.Set("Client-Name", "OreoBot2-Go")

	d := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := d.Dial(u, headers)
	if err != nil {
		return fmt.Errorf("ws dial %s: %w", u, err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var ready llReady
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			_ = conn.Close()
			return fmt.Errorf("ws read: %w", err)
		}
		if err := json.Unmarshal(msg, &ready); err == nil && strings.EqualFold(ready.Op, "ready") && ready.SessionID != "" {
			break
		}
	}
	_ = conn.SetReadDeadline(time.Time{})

	l.wsMu.Lock()
	oldConn := l.ws
	l.ws = conn
	l.llSession = ready.SessionID
	l.wsMu.Unlock()

	if oldConn != nil {
		_ = oldConn.Close()
	}

	slog.Info("lavalink WS connected", "sessionId", ready.SessionID)

	go l.llWSReadLoop()
	return nil
}

func (l *LavalinkBackend) llWSReadLoop() {
	l.wsMu.RLock()
	myConn := l.ws
	l.wsMu.RUnlock()

	if myConn == nil {
		return
	}

	for {
		l.wsMu.RLock()
		currentConn := l.ws
		l.wsMu.RUnlock()
		if currentConn != myConn {
			return
		}

		_, msg, err := myConn.ReadMessage()
		if err == nil {
			l.handleLLEvent(msg)
			continue
		}

		l.wsMu.Lock()
		if l.ws == myConn {
			l.ws = nil
			l.llSession = ""
		}
		l.wsMu.Unlock()

		_ = myConn.Close()
		slog.Warn("lavalink WS disconnected", "error", err)
