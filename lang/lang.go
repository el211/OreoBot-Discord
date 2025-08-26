package lang

import (
	"log/slog"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	mu             sync.RWMutex
	messages       map[string]string
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

	mu.Lock()
	messages = m
	activeLanguage = activeLang
	mu.Unlock()

	slog.Info("language loaded", "language", activeLang, "keys", len(m))
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
