package handlers

import "strings"

// phishingDomains is a built-in blocklist of domains commonly used in Discord
// scams (fake Nitro/gift/Steam/airdrop pages and known phishing hosts). These
// are ALWAYS blocked by the anti-scam system, regardless of the link-filter
// whitelist. Admins can extend this list via anti_scam.extra_domains.
//
// Entries are bare, lowercase hosts. Subdomains match automatically, so
// "steamcommunity.ru" also blocks "login.steamcommunity.ru".
var phishingDomains = []string{
	// Fake Discord / Nitro
	"discordnitro.info",
	"discord-nitro.info",
	"discordnitro.gift",
	"discord-gift.ru",
	"discordgift.ru",
	"discord-gifts.com",
	"discordgifts.com",
	"dlscord.gift",
	"dlscord.com",
	"dlscord-nitro.com",
	"discocrd.gift",
	"discrod.gift",
	"discordapp.gift",
	"discord-app.online",
	"discordc.gift",
	"discrodgifts.com",
	"discordnitro.com.ru",
	"nitro-discord.info",
	"discords.gift",
	"discordgive.com",
	"discordgiveaway.com",
	// Fake Steam
	"steamcommunity.ru",
	"steamcommunnity.com",
	"steamcomminuty.com",
	"steamcommmunity.com",
	"steancommunity.com",
	"steam-community.ru",
	"stearmcommunity.com",
	"steampowered.ru",
	"steamgift.com",
	"steamnitro.com",
	// Generic gift/airdrop/crypto scam hosts
	"free-nitro.ru",
	"free-discord.com",
	"nitro-gift.ru",
	"gift-nitro.com",
	"click-nitro.com",
	"airdrop-discord.com",
	"crypto-airdrop.info",
}

// phishingSet is the blocklist keyed for O(1) lookups, built once at init.
var phishingSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(phishingDomains))
	for _, d := range phishingDomains {
		m[d] = struct{}{}
	}
	return m
}()

// isPhishingDomain reports whether host is on the built-in blocklist or is a
// subdomain of a blocked host. Extra holds admin-configured additions.
func isPhishingDomain(host string, extra []string) bool {
	host = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "www.")
	if host == "" {
		return false
	}
	if matchesDomain(host, phishingSet, nil) {
		return true
	}
	if len(extra) > 0 {
		norm := make([]string, 0, len(extra))
		for _, e := range extra {
			if h := extractHost(e); h != "" {
				norm = append(norm, h)
			}
		}
		return matchesDomain(host, nil, norm)
	}
	return false
}

// matchesDomain checks host against a set and/or a slice of bare hosts,
// matching exact hosts and subdomains of any entry.
func matchesDomain(host string, set map[string]struct{}, list []string) bool {
	if set != nil {
		if _, ok := set[host]; ok {
			return true
		}
		if i := strings.IndexByte(host, '.'); i != -1 {
			for h := host; i != -1; {
				h = h[i+1:]
				if _, ok := set[h]; ok {
					return true
				}
				i = strings.IndexByte(h, '.')
			}
		}
	}
	for _, w := range list {
		if host == w || strings.HasSuffix(host, "."+w) {
			return true
		}
	}
	return false
}
