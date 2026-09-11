package lang

import (
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	mu             sync.RWMutex
	messages       map[string]string
	allMessages    = map[string]map[string]string{}
	activeLanguage = "en"
)

var builtInMessages = map[string]map[string]string{
	"en": {
		"channel_rename_invalid": "The channel name cannot be empty.",
		"channel_rename_failed":  "Failed to rename channel: {error}",
		"channel_renamed":        "Channel renamed to `{name}`.",
	},
	"fr": {
		"channel_rename_invalid": "Le nom du salon ne peut pas être vide.",
		"channel_rename_failed":  "Échec du renommage du salon : {error}",
		"channel_renamed":        "Salon renommé en `{name}`.",
	},
}

func Load(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		slog.Warn("could not read lang file, using empty translations", "path", path, "error", err)
		mu.Lock()
		messages = make(map[string]string)
		activeLanguage = "en"
		mu.Unlock()
		return
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		slog.Error("failed to parse lang file", "path", path, "error", err)
	}

	activeLang := "en"
	if v, ok := raw["active_language"]; ok {
		if s, ok := v.(string); ok && s != "" {
			activeLang = s
		}
	}

	block, ok := raw[activeLang]
	if !ok {
		slog.Warn("language not found in file, falling back to en", "language", activeLang, "path", path)
		activeLang = "en"
		block, ok = raw[activeLang]
		if !ok {
			slog.Warn("fallback en also missing, using empty translations")
			mu.Lock()
			messages = make(map[string]string)
			activeLanguage = "en"
			mu.Unlock()
			return
		}
	}

	blockMap, ok := block.(map[string]interface{})
	if !ok {
		slog.Error("language block is not a map", "language", activeLang)
	}

	m := make(map[string]string, len(blockMap))
	for k, v := range blockMap {
		if s, ok := v.(string); ok {
			m[k] = s
		}
	}

	// Parse every language block (not just the active one) so callers can render
	// in any available language on demand.
	all := map[string]map[string]string{}
	for lk, lv := range raw {
		if lk == "active_language" {
			continue
		}
		bm, ok := lv.(map[string]interface{})
		if !ok {
			continue
		}
		lm := make(map[string]string, len(bm))
		for k, v := range bm {
			if s, ok := v.(string); ok {
				lm[k] = s
			}
		}
		if len(lm) > 0 {
			all[lk] = lm
		}
	}

	mu.Lock()
	messages = m
	allMessages = all
	activeLanguage = activeLang
	mu.Unlock()

	slog.Info("language loaded", "language", activeLang, "keys", len(m), "languages", len(all))
}

// ActiveLanguage returns the currently active language code.
func ActiveLanguage() string {
	mu.RLock()
	defer mu.RUnlock()
	return activeLanguage
}

// AvailableLanguages returns the language codes present in the loaded lang file,
// sorted alphabetically.
func AvailableLanguages() []string {
	mu.RLock()
	defer mu.RUnlock()
	codes := make([]string, 0, len(allMessages))
	for c := range allMessages {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	return codes
}

// TL translates a key in a specific language, falling back to English, then the
// built-in defaults, then the raw key.
func TL(langCode, key string, pairs ...string) string {
	mu.RLock()
	s, ok := "", false
	if lm, exists := allMessages[langCode]; exists {
		s, ok = lm[key]
	}
	if !ok {
		if lm, exists := allMessages["en"]; exists {
			s, ok = lm[key]
		}
	}
	if !ok {
		if defaults, exists := builtInMessages[langCode]; exists {
			s, ok = defaults[key]
		}
	}
	if !ok {
		if defaults, exists := builtInMessages["en"]; exists {
			s, ok = defaults[key]
		}
	}
	mu.RUnlock()

	if !ok {
		return "{" + key + "}"
	}
	for j := 0; j+1 < len(pairs); j += 2 {
		s = strings.ReplaceAll(s, "{"+pairs[j]+"}", pairs[j+1])
	}
	return s
}

func T(key string, pairs ...string) string {
	mu.RLock()
	s, ok := messages[key]
	if !ok {
		if defaults, exists := builtInMessages[activeLanguage]; exists {
			s, ok = defaults[key]
		}
	}
	mu.RUnlock()

	if !ok {
		return "{" + key + "}"
	}

	if len(pairs) == 0 {
		return s
	}

	for j := 0; j+1 < len(pairs); j += 2 {
		s = strings.ReplaceAll(s, "{"+pairs[j]+"}", pairs[j+1])
	}
	return s
}

func Reload(path string) {
	Load(path)
}
