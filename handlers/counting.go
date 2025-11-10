package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"discord-bot/config"

	"github.com/bwmarrin/discordgo"
)

type countingState struct {
	mu sync.Mutex

	Count      int    `json:"count"`
	LastUserID string `json:"last_user_id"`
	HighScore  int    `json:"high_score"`
}

var counting = &countingState{}

const countingStatePath = "data/counting.json"

func loadCountingState() {
	data, err := os.ReadFile(countingStatePath)
	if err != nil {
		return
	}
	counting.mu.Lock()
	defer counting.mu.Unlock()
	_ = json.Unmarshal(data, counting)
}

func saveCountingState() {
	counting.mu.Lock()
	snapshot, _ := json.MarshalIndent(counting, "", "  ")
	counting.mu.Unlock()

	_ = os.MkdirAll("data", 0755)
	if err := os.WriteFile(countingStatePath, snapshot, 0644); err != nil {
		slog.Error("counting failed to save state", "error", err)
	}
}

func (h *Handler) RegisterCounting(s *discordgo.Session) {
	if !h.cfg.CountingGame.Enabled || h.cfg.CountingGame.ChannelID == "" {
		return
	}
	loadCountingState()
	s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		handleCountingMessage(s, m, &h.cfg.CountingGame)
	})
	slog.Info("counting game active",
		"channel_id", h.cfg.CountingGame.ChannelID,
		"count", counting.Count,
		"high_score", counting.HighScore,
	)
}

func handleCountingMessage(s *discordgo.Session, m *discordgo.MessageCreate, cfg *config.CountingGameConfig) {
	if m.Author.Bot || m.ChannelID != cfg.ChannelID {
		return
	}

	content := strings.TrimSpace(m.Content)

	number, err := strconv.Atoi(content)
	if err != nil {
		if cfg.DeleteNonNumbers {
			_ = s.ChannelMessageDelete(m.ChannelID, m.ID)
			sendTemp(s, m.ChannelID, fmt.Sprintf("<@%s> Only numbers allowed in this channel!", m.Author.ID), 5)
		}
		return
	}

	type outcome int
	const (
		outcomeSameUser outcome = iota
		outcomeWrong
		outcomeCorrect
	)

	var result outcome
	var expected, savedCount, newHigh int
	var isNewHigh bool

	counting.mu.Lock()
	expected = counting.Count + 1

	switch {
	case m.Author.ID == counting.LastUserID:
		result = outcomeSameUser