package music

import "testing"

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0:00"},
		{-1, "0:00"},
		{60, "1:00"},
		{90, "1:30"},
		{3661, "61:01"},
		{3600, "60:00"},
	}
	for _, tt := range tests {
		if got := FormatDuration(tt.in); got != tt.want {
			t.Errorf("FormatDuration(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
