package handlers

import (
	"sync"

	"discord-bot/config"
	"discord-bot/minecraft"
	"discord-bot/music"
	"discord-bot/storage"
)

type Handler struct {
	cfg          *config.Config
	db           storage.Database
	musicMgr     *music.Manager
	rcon         *minecraft.Client
	mcStore      MCLinkStore
	customCmds   map[string]config.CustomCommandConfig
	customCmdsMu sync.RWMutex
}

func NewHandler(cfg *config.Config, db storage.Database) *Handler {
	return &Handler{cfg: cfg, db: db}
}

func (h *Handler) SetMusic(m *music.Manager)    { h.musicMgr = m }
func (h *Handler) SetRCON(r *minecraft.Client)  { h.rcon = r }
func (h *Handler) SetMCStore(s MCLinkStore)      { h.mcStore = s }
func (h *Handler) GetRCON() *minecraft.Client   { return h.rcon }

func (h *Handler) lookupCustomCommand(name string) (config.CustomCommandConfig, bool) {
	h.customCmdsMu.RLock()
	defer h.customCmdsMu.RUnlock()
	cc, ok := h.customCmds[name]
	return cc, ok
}
