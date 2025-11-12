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

	case number != expected:
		result = outcomeWrong
		savedCount = counting.Count
		newHigh = counting.HighScore
		if cfg.FailResets {
			if savedCount > counting.HighScore {
				counting.HighScore = savedCount
				newHigh = savedCount
			}
			counting.Count = 0
			counting.LastUserID = ""
		}

	default:
		result = outcomeCorrect
		counting.Count = number
		counting.LastUserID = m.Author.ID
		newHigh = counting.HighScore
		if number > counting.HighScore {
			counting.HighScore = number
			newHigh = number
			isNewHigh = true
		}
	}
	counting.mu.Unlock()

	if result == outcomeSameUser {
		if cfg.DeleteWrong {
			_ = s.ChannelMessageDelete(m.ChannelID, m.ID)
		}
		sendTemp(s, m.ChannelID,
			fmt.Sprintf("<@%s> You wrote the last number! Wait for someone else to count before you go again.", m.Author.ID),
			7,
		)
		return
	}

	if result == outcomeWrong {
		if cfg.DeleteWrong {
			_ = s.ChannelMessageDelete(m.ChannelID, m.ID)
		}

		if cfg.FailResets {
			go saveCountingState()

			msg := fmt.Sprintf(
				"<@%s> wrote **%d** but the next number was **%d**. The count resets to **0**!\n"+
					"Count got to **%d**. High score: **%d**. Start again from **1**!",
				m.Author.ID, number, expected, savedCount, newHigh,
			)
			_, _ = s.ChannelMessageSend(m.ChannelID, msg)
		} else {
			sendTemp(s, m.ChannelID,
				fmt.Sprintf("<@%s> Wrong number! The next number is **%d**, not %d.", m.Author.ID, expected, number),
				7,
			)
		}
		return
	}

	go saveCountingState()

	_ = s.MessageReactionAdd(m.ChannelID, m.ID, "✅")

	if number%100 == 0 {
		_, _ = s.ChannelMessageSend(m.ChannelID,
			fmt.Sprintf("🎉 **%d!** Amazing counting everyone!", number),
		)
	} else if isNewHigh && number > 1 && number%10 == 0 {
		_, _ = s.ChannelMessageSend(m.ChannelID,
			fmt.Sprintf("📈 New high score: **%d**!", number),
		)
	}
}

func sendTemp(s *discordgo.Session, channelID, content string, seconds int) {
	msg, err := s.ChannelMessageSend(channelID, content)
	if err != nil {
		return
	}
	go func() {
		time.Sleep(time.Duration(seconds) * time.Second)
		_ = s.ChannelMessageDelete(channelID, msg.ID)
	}()
}
