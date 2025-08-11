package storage

import (
	"log/slog"
	"os"
	"strings"
	"sync"

	"discord-bot/config"
)

var Cfg *config.Config

var (
	guilds = make(map[string]*config.GuildState)
	mu     sync.RWMutex
)

func GetGuild(guildID string) *config.GuildState {
	mu.RLock()
	gs, ok := guilds[guildID]
	mu.RUnlock()
	if ok {
		return gs
	}

	mu.Lock()
	defer mu.Unlock()
	if gs, ok = guilds[guildID]; ok {
		return gs
	}
	gs = config.LoadGuildState(guildID)
	guilds[guildID] = gs
	return gs
}

func GetAllGuildStates() []*config.GuildState {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]*config.GuildState, 0, len(guilds))
	for _, gs := range guilds {
		result = append(result, gs)
	}
	return result
}

func LoadAllGuildStates() {
	dir := "data/guilds"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		guildID := strings.TrimSuffix(entry.Name(), ".json")
		GetGuild(guildID)
		count++
	}
	if count > 0 {
		slog.Info("storage pre-loaded guild states", "count", count)
	}
}
