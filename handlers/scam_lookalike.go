package handlers

import "strings"

// protectedDomains are the legitimate hosts that scams most often impersonate.
// A domain that closely resembles one of these — but isn't it — is treated as a
// lookalike (typo-squat / homoglyph) and flagged.
var protectedDomains = []string{
	"discord.com",
	"discord.gg",
	"discordapp.com",
	"discord.gift",
	"steamcommunity.com",
	"steampowered.com",
	"steamgames.com",
}

// protectedBrands are brand tokens whose presence in a host, absent an official
// domain, strongly suggests impersonation (e.g. "free-discord-nitro.xyz").
var protectedBrands = []string{"discord", "discordapp", "steamcommunity", "nitro"}

// isOfficialDomain reports whether host is exactly a protected domain or a
// subdomain of one (i.e. genuinely legitimate).
func isOfficialDomain(host string) bool {
	for _, d := range protectedDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// isLookalikeDomain reports whether host impersonates a protected brand without
// being an official domain. Two signals: (1) it is within a small edit distance
// of a protected domain (typo-squat / homoglyph), or (2) it contains a brand
// token but is not an official domain.
func isLookalikeDomain(host string) bool {
	host = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "www.")
	if host == "" || isOfficialDomain(host) {
		return false
	}

	// Signal 1: near-miss of an official domain (e.g. "dlscord.com", "d1scord.gg").
	norm := deHomoglyph(host)
	for _, d := range protectedDomains {
		if levenshtein(norm, d) <= 2 {
			return true
		}
	}

	// Signal 2: a protected brand token appears in the host, but it's not legit.
	for _, brand := range protectedBrands {
		if strings.Contains(norm, brand) {
			return true
		}
	}
	return false
}

// deHomoglyph maps common look-alike characters to the letters they mimic so a
// homoglyph swap (0→o, 1→l, rn→m) collapses to the brand it imitates.
func deHomoglyph(s string) string {
	r := strings.NewReplacer(
		"0", "o",
		"1", "l",
		"3", "e",
		"4", "a",
		"5", "s",
		"rn", "m",
		"vv", "w",
	)
	return r.Replace(s)
}

// levenshtein returns the edit distance between two strings.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur := make([]int, lb+1)
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
