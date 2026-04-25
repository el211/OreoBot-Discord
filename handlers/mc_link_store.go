package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"discord-bot/config"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MCLink struct {
	DiscordID string `json:"discord_id" bson:"discord_id"`
	UUID      string `json:"uuid"       bson:"uuid"`
	Username  string `json:"username"   bson:"username"`
	LinkedAt  string `json:"linked_at"  bson:"linked_at"`
}

type MCLinkConfirmation struct {
	Code      string
	DiscordID string
	UUID      string
	Username  string
}

type MCLinkStore interface {
	SavePendingCode(code, discordID, guildID string, expiresAt time.Time) error

	PopConfirmed() ([]MCLinkConfirmation, error)

	SaveLink(link MCLink) error

	LoadLink(discordID string) (*MCLink, error)

	DeleteLink(discordID string) error

	ListLinks() ([]MCLink, error)
}

const (
	mcLinkDir        = "data/mc_links"
	mcLinkPendingDir = "data/mc_links/pending"
)

type fileLinkStore struct {
	pending map[string]filePendingEntry
}

type filePendingEntry struct {
	DiscordID string    `json:"discord_id"`
	GuildID   string    `json:"guild_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

func newFileLinkStore() *fileLinkStore {
	_ = os.MkdirAll(mcLinkDir, 0755)
	_ = os.MkdirAll(mcLinkPendingDir, 0755)
	return &fileLinkStore{pending: make(map[string]filePendingEntry)}
}

func (f *fileLinkStore) SavePendingCode(code, discordID, guildID string, expiresAt time.Time) error {
	entry := filePendingEntry{
		DiscordID: discordID,
		GuildID:   guildID,
		ExpiresAt: expiresAt,
	}
	pendingLinksMu.Lock()
	pendingLinks[code] = pendingLink{discordID: discordID, expiresAt: expiresAt}
	pendingLinksMu.Unlock()

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fmt.Sprintf("%s/%s.json", mcLinkPendingDir, code), data, 0644)
}

func (f *fileLinkStore) PopConfirmed() ([]MCLinkConfirmation, error) {
	entries, err := os.ReadDir(mcLinkPendingDir)
	if err != nil {
		return nil, nil
	}

	var results []MCLinkConfirmation
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		code := strings.TrimSuffix(e.Name(), ".json")
		filePath := mcLinkPendingDir + "/" + e.Name()

		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var payload struct {
			UUID      string `json:"uuid"`
			Username  string `json:"username"`
			Confirmed bool   `json:"confirmed"`
		}
		if err := json.Unmarshal(data, &payload); err != nil || !payload.Confirmed {
			pendingLinksMu.Lock()
			p, ok := pendingLinks[code]
			if ok && time.Now().After(p.expiresAt) {
				delete(pendingLinks, code)
				_ = os.Remove(filePath)
			}
			pendingLinksMu.Unlock()
			continue
		}

		discordID, valid := ConsumeLinkCode(code)
		if !valid {
			_ = os.Remove(filePath)
			continue
		}

		_ = os.Remove(filePath)
		results = append(results, MCLinkConfirmation{
			Code:      code,
			DiscordID: discordID,
			UUID:      payload.UUID,
			Username:  payload.Username,
		})
	}
	return results, nil
}

func (f *fileLinkStore) SaveLink(link MCLink) error {
	_ = os.MkdirAll(mcLinkDir, 0755)
	data, err := json.MarshalIndent(link, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fmt.Sprintf("%s/%s.json", mcLinkDir, link.DiscordID), data, 0644)
}

func (f *fileLinkStore) LoadLink(discordID string) (*MCLink, error) {
	data, err := os.ReadFile(fmt.Sprintf("%s/%s.json", mcLinkDir, discordID))
	if err != nil {
		return nil, err
	}
	var link MCLink
	if err := json.Unmarshal(data, &link); err != nil {
		return nil, err
	}
	return &link, nil
}

func (f *fileLinkStore) DeleteLink(discordID string) error {
	return os.Remove(fmt.Sprintf("%s/%s.json", mcLinkDir, discordID))
}

func (f *fileLinkStore) ListLinks() ([]MCLink, error) {
	entries, err := os.ReadDir(mcLinkDir)
	if err != nil {
		return nil, err
	}
	var links []MCLink
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		discordID := strings.TrimSuffix(e.Name(), ".json")
		link, err := f.LoadLink(discordID)
		if err != nil {
			continue
		}
		links = append(links, *link)
	}
	return links, nil
}

type mongoLinkStore struct {
	pending   *mongo.Collection
	confirmed *mongo.Collection
	links     *mongo.Collection
}

func newMongoLinkStore(cfg *config.Config) (*mongoLinkStore, error) {
	uri := cfg.Database.MongoDB.URI
	db := cfg.Database.MongoDB.Database
	if uri == "" || db == "" {
		return nil, fmt.Errorf("database.mongodb.uri and database.mongodb.database must be set in config.json to use link_backend=mongodb")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
