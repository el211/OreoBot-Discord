package music

import (
	"bytes"
	"discord-bot/config"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/ogg"
)

type DirectBackend struct {
	ytdlpPath  string
	ffmpegPath string

	mu        sync.Mutex
	ffmpegCmd *exec.Cmd
	stopFlag  bool
	volume    int
}

func NewDirectBackend(cfg *config.DirectMusicConfig) (*DirectBackend, error) {
	if _, err := exec.LookPath(cfg.YTDLPPath); err != nil {
		return nil, fmt.Errorf("yt-dlp not found at %q: %w", cfg.YTDLPPath, err)
	}
	if _, err := exec.LookPath(cfg.FFmpegPath); err != nil {
		return nil, fmt.Errorf("ffmpeg not found at %q: %w", cfg.FFmpegPath, err)
	}

	return &DirectBackend{
		ytdlpPath:  cfg.YTDLPPath,
		ffmpegPath: cfg.FFmpegPath,
		volume:     50,
	}, nil
}

func (d *DirectBackend) Name() string { return "direct" }

func (d *DirectBackend) ResolveSong(query string) (*Song, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("empty query")
	}

	if isHTTPURL(q) && looksLikeDirectAudioURL(q) {
		title := deriveTitleFromURL(q)
		return &Song{Title: title, URL: q, StreamURL: q, Duration: 0}, nil
	}

	provider, term := splitProviderPrefix(q)

	if provider == "bc" {
		term = strings.TrimSpace(term)
		if !isHTTPURL(term) {
			return nil, fmt.Errorf("bc: expects a Bandcamp URL (example: bc: https://artist.bandcamp.com/track/...)")
		}
		return d.resolveWithYTDLP(term)
	}

	var candidates []string
	if isHTTPURL(q) {
		candidates = []string{q}
	} else {
		term = strings.TrimSpace(term)
		if term == "" {
			return nil, fmt.Errorf("missing search query")
		}

		switch provider {
		case "sc":
			candidates = []string{"scsearch5:" + term}
		case "yt":
			candidates = []string{"ytsearch1:" + term}
		case "":
			candidates = []string{"scsearch5:" + term}
		default:
			return nil, fmt.Errorf("unknown provider %q (use yt:, sc:, bc:)", provider)
		}
	}

	var lastErr error
	for _, cand := range candidates {
		s, err := d.resolveWithYTDLP(cand)
		if err == nil {
			return s, nil
		}
		lastErr = err
	}
	return nil, lastErr