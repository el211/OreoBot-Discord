package handlers

import "testing"

func TestSanitizeMCPlayerName(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"Steve", "Steve", true},
		{"Steve Jobs", "SteveJobs", true},
		{"", "", false},
		{"thisnameiswaytooolong", "", false},
		{"exactly16chars!!", "", false},
		{"valid_name_", "valid_name_", true},
	}
	for _, tt := range tests {
		got, ok := sanitizeMCPlayerName(tt.in)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("sanitizeMCPlayerName(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestGenerateLinkCode(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code := generateLinkCode()
		if len(code) != 6 {
			t.Errorf("expected length 6, got %d: %q", len(code), code)
		}
		const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
		for _, c := range code {
			found := false
			for _, valid := range charset {
				if c == valid {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("invalid character %q in code %q", c, code)
			}
		}
		seen[code] = true
	}
	if len(seen) < 90 {
		t.Errorf("expected high uniqueness, only got %d unique codes in 100", len(seen))
	}
}
