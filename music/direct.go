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
}

func (d *DirectBackend) resolveWithYTDLP(query string) (*Song, error) {

	isSC := strings.HasPrefix(query, "scsearch") || strings.Contains(query, "soundcloud.com")

	args := []string{
		"--no-playlist",
		"--dump-json",
		"--no-warnings",
		"--no-check-certificates",
	}

	if isSC {
		args = append(args,
			"-f", "http_mp3_128/http_mp3_64/bestaudio/best",
		)
	} else {
		args = append(args,
			"-f", "bestaudio/best",
			"--format-sort", "proto:https,ext:m4a:mp3:opus:ogg,aext:m4a:mp3:opus:ogg,acodec:opus:aac",
			"--extractor-args", "youtube:player_client=android",
			"--extractor-args", "youtube:player_skip=webpage,configs,js",
		)
	}

	args = append(args, query)

	cmd := exec.Command(d.ytdlpPath, args...)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return nil, fmt.Errorf("yt-dlp failed: %s", errMsg)
	}

	output := stdout.String()
	if output == "" {
		return nil, fmt.Errorf("yt-dlp returned empty output (stderr: %s)", strings.TrimSpace(stderr.String()))
	}

	var info struct {
		Title        string  `json:"title"`
		URL          string  `json:"url"`
		Duration     float64 `json:"duration"`
		WebPage      string  `json:"webpage_url"`
		Extractor    string  `json:"extractor"`
		ExtractorKey string  `json:"extractor_key"`
	}
	if err := json.Unmarshal([]byte(output), &info); err != nil {
		preview := output
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		return nil, fmt.Errorf("yt-dlp JSON parse error: %w (output preview: %s)", err, preview)
	}

	streamLower := strings.ToLower(info.URL)
	if strings.Contains(streamLower, "cf-preview-media.sndcdn.com") || int(info.Duration) > 0 && int(info.Duration) < 45 {
		return nil, fmt.Errorf("soundcloud returned preview stream (%ds). Try another result / different query.", int(info.Duration))
	}

	return &Song{
		Title:     info.Title,
		URL:       info.WebPage,
		StreamURL: info.URL,
		Duration:  int(info.Duration),
	}, nil
}

func splitProviderPrefix(q string) (provider string, term string) {
	lower := strings.ToLower(strings.TrimSpace(q))
	for _, p := range []string{"yt:", "sc:", "bc:"} {
		if strings.HasPrefix(lower, p) {
			return strings.TrimSuffix(p, ":"), strings.TrimSpace(q[len(p):])
		}
	}
	return "", q
}

func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func looksLikeDirectAudioURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	p := strings.ToLower(u.Path)
	switch {
	case strings.HasSuffix(p, ".mp3"),