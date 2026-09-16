package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// feedDomains holds phishing domains fetched from community lists. It is merged
// with the built-in blocklist by isPhishingDomain. Guarded by feedMu.
var (
	feedMu      sync.RWMutex
	feedDomains = map[string]struct{}{}
)

// feedHTTPClient fetches phishing feeds.
var feedHTTPClient = &http.Client{Timeout: 30 * time.Second}

// feedContains reports whether host (or a parent domain) is in the synced feed.
func feedContains(host string) bool {
	feedMu.RLock()
	defer feedMu.RUnlock()
	if len(feedDomains) == 0 {
		return false
	}
	return matchesDomain(host, feedDomains, nil)
}

// StartPhishingFeed begins periodic syncing of community phishing lists in the
// background. It does one immediate fetch, then refreshes on the configured
// interval. A no-op unless anti-scam and the feed are both enabled.
func (h *Handler) StartPhishingFeed() {
	if !h.cfg.AntiScam.Enabled || !h.cfg.AntiScam.Feed.Enabled {
		return
	}
	feed := h.cfg.AntiScam.Feed
	go func() {
		interval := time.Duration(feed.EffectiveRefreshMinutes()) * time.Minute
		for {
			refreshPhishingFeed(feed.EffectiveURLs())
			time.Sleep(interval)
		}
	}()
}

// refreshPhishingFeed fetches every URL, unions the results, and atomically
// replaces the in-memory feed set. A URL that fails is skipped so one bad
// source doesn't wipe the whole list.
func refreshPhishingFeed(urls []string) {
	merged := map[string]struct{}{}
	for _, u := range urls {
		domains := fetchFeed(u)
		for _, d := range domains {
			if h := extractHost(d); h != "" {
				merged[h] = struct{}{}
			}
		}
	}
	if len(merged) == 0 {
		slog.Warn("phishing feed refresh returned no domains; keeping previous set")
		return
	}
	feedMu.Lock()
	feedDomains = merged
	feedMu.Unlock()
	slog.Info("phishing feed synced", "domains", len(merged), "sources", len(urls))
}

// fetchFeed downloads one list, accepting either a JSON array of strings or a
// newline-separated plain-text list.
func fetchFeed(url string) []string {
	resp, err := feedHTTPClient.Get(url)
	if err != nil {
		slog.Debug("phishing feed fetch failed", "url", url, "err", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Debug("phishing feed bad status", "url", url, "status", resp.StatusCode)
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil
	}

	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "[") {
		var arr []string
		if err := json.Unmarshal([]byte(trimmed), &arr); err == nil {
			return arr
		}
	}
	// Fall back to newline-separated text.
	var out []string
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}
