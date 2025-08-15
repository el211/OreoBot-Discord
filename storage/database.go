package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"discord-bot/config"
)

var DB Database

type Database interface {
	Init() error
	Close() error

	AddWarning(guildID, userID string, w config.Warning) error
	GetWarnings(guildID, userID string) ([]config.Warning, error)
	ClearWarnings(guildID, userID string) error

	AddModCase(guildID string, c ModCase) error
	GetModCases(guildID, userID string, limit int) ([]ModCase, error)
}

type ModCase struct {
	ID        int    `json:"id"`
	GuildID   string `json:"guild_id"`
	UserID    string `json:"user_id"`
	ModID     string `json:"mod_id"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
	Duration  string `json:"duration,omitempty"`
	Timestamp string `json:"timestamp"`
}

func InitDB(cfg *config.DatabaseConfig) error {
	switch cfg.Driver {
	case "sqlite":
		db := &SQLiteDB{Path: cfg.SQLite.Path}
		if err := db.Init(); err != nil {
			return err
		}
		DB = db
		return nil

	case "mongodb":
		db := &MongoDB{URI: cfg.MongoDB.URI, DBName: cfg.MongoDB.Database}
		if err := db.Init(); err != nil {
			return err
		}
		DB = db
		return nil

	default:
		return fmt.Errorf("unsupported database driver: %s (use \"sqlite\" or \"mongodb\")", cfg.Driver)
	}
}

type SQLiteDB struct {
	Path string
	db   *sql.DB
}

func (s *SQLiteDB) Init() error {
	_ = os.MkdirAll(filepath.Dir(s.Path), 0755)

	db, err := sql.Open("sqlite", s.Path)
	if err != nil {
		return fmt.Errorf("sqlite open: %w", err)
	}
	s.db = db

	schema := `
	CREATE TABLE IF NOT EXISTS warnings (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		guild_id    TEXT NOT NULL,
		user_id     TEXT NOT NULL,
		mod_id      TEXT NOT NULL,
		reason      TEXT NOT NULL,
		timestamp   TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_warnings_guild_user ON warnings(guild_id, user_id);

	CREATE TABLE IF NOT EXISTS mod_cases (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		guild_id    TEXT NOT NULL,
		user_id     TEXT NOT NULL,
		mod_id      TEXT NOT NULL,
		action      TEXT NOT NULL,
		reason      TEXT NOT NULL DEFAULT '',
		duration    TEXT NOT NULL DEFAULT '',
		timestamp   TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_mod_cases_guild_user ON mod_cases(guild_id, user_id);
	`