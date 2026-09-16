package handlers

import (
	"fmt"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"
)

// discordEpoch is the Discord snowflake epoch (2015-01-01) in milliseconds.
const discordEpoch = 1420070400000

// accountCreated derives an account's creation time from its snowflake ID.
func accountCreated(userID string) (time.Time, bool) {
	id, err := strconv.ParseInt(userID, 10, 64)
	if err != nil || id == 0 {
		return time.Time{}, false
	}
	ms := (id >> 22) + discordEpoch
	return time.UnixMilli(ms), true
}

// accountRiskSignals returns extra scam score and reasons based on how risky the
// author's account looks: a very new account and/or a default (no) avatar are
// classic throwaway-scammer traits. These are meant to be added ONLY when a
// message already shows scam signals, so genuine new members aren't punished.
func accountRiskSignals(author *discordgo.User, newAccountDays int) (int, []string) {
	if author == nil {
		return 0, nil
	}
	score := 0
	var reasons []string

	if created, ok := accountCreated(author.ID); ok {
		age := time.Since(created)
		if age < time.Duration(newAccountDays)*24*time.Hour {
			score++
			reasons = append(reasons, fmt.Sprintf("new account (%dd old)", int(age.Hours()/24)))
		}
	}
	if author.Avatar == "" {
		score++
		reasons = append(reasons, "no avatar")
	}
	return score, reasons
}
