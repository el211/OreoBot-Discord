package handlers

import "testing"

func TestFindBlockedLink(t *testing.T) {
	whitelist := []string{"github.com", "youtube.com"}

	cases := []struct {
		name        string
		content     string
		blockInvite bool
		wantBlocked bool
	}{
		{"plain text", "hello everyone how are you", true, false},
		{"decimal not a link", "the price is 3.50 dollars", true, false},
		{"eg abbreviation", "e.g. this is fine", true, false},
		{"discord invite", "join my server discord.gg/abc123", true, true},
		{"discord invite full", "https://discord.com/invite/xyz", true, true},
		{"http ad link", "buy now at http://spam-site.xyz/deal", true, true},
		{"bare domain ad", "check spammy.shop for deals", true, true},
		{"whitelisted github", "my repo https://github.com/me/proj", true, false},
		{"whitelisted subdomain", "see gist.github.com/abc", true, false},
		{"whitelisted youtube", "watch youtube.com/watch?v=1", true, false},
		{"whitelisted plus bad", "github.com is fine but evil.biz is not", true, true},
		{"invite ignored when disabled but still a link", "discord.gg/abc", false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blocked, sample := findBlockedLink(tc.content, whitelist, tc.blockInvite)
			if blocked != tc.wantBlocked {
				t.Fatalf("content %q: got blocked=%v (sample=%q), want %v", tc.content, blocked, sample, tc.wantBlocked)
			}
		})
	}
}

func TestExtractHost(t *testing.T) {
	cases := map[string]string{
		"https://github.com/foo":     "github.com",
		"http://www.example.com":     "example.com",
		"gist.github.com/abc":        "gist.github.com",
		"example.com:8080/path":      "example.com",
		"user@host.com":              "host.com",
		"WWW.UPPER.COM/Path?q=1":     "upper.com",
	}
	for in, want := range cases {
		if got := extractHost(in); got != want {
			t.Errorf("extractHost(%q) = %q, want %q", in, got, want)
		}
	}
}
