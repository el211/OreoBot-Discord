package music

import "fmt"

func FormatDuration(seconds int) string {
	if seconds <= 0 {
		return "0:00"
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}
