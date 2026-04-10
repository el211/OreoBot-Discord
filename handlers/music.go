package handlers

import (
	"fmt"
	"strings"

	"discord-bot/lang"
	"discord-bot/music"

	"github.com/bwmarrin/discordgo"
)

func musicCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name: "play", Description: "Play a song or add it to the queue",
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "query", Description: "Song name or YouTube URL", Required: true},
			},
		},
		{Name: "skip", Description: "Skip the current song"},
		{Name: "stop", Description: "Stop playback and clear the queue"},
		{Name: "queue", Description: "Show the current song queue"},
		{
			Name: "volume", Description: "Set the playback volume",
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionInteger, Name: "level", Description: "Volume 0-100", Required: true},
			},
		},
		{Name: "nowplaying", Description: "Show the currently playing song"},
		{Name: "pause", Description: "Pause playback"},
		{Name: "resume", Description: "Resume playback"},
	}
}

func (h *Handler) handleMusicCommand(s *discordgo.Session, i *discordgo.InteractionCreate, name string) {
	if !h.cfg.Music.Enabled {
		respond(s, i, lang.T("music_disabled"), true)
		return
	}
	if h.musicMgr == nil {
		respond(s, i, lang.T("music_init_failed"), true)
		return
	}

	switch name {
	case "play":
		h.handlePlay(s, i)
	case "skip":
		h.handleSkip(s, i)
	case "stop":
		h.handleStop(s, i)
	case "queue":
		h.handleQueue(s, i)
	case "volume":
		h.handleVolume(s, i)
	case "nowplaying":
		h.handleNowPlaying(s, i)
	case "pause":
		h.handlePause(s, i)
	case "resume":
		h.handleResume(s, i)
	}
}

func (h *Handler) handlePlay(s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := optionMap(i)
	query := opts["query"].StringValue()

	voiceChID := music.GetVoiceChannelOfUser(s, i.GuildID, i.Member.User.ID)
	if voiceChID == "" {
		respond(s, i, lang.T("music_not_in_vc"), true)
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	player := h.musicMgr.GetPlayer(i.GuildID)

	song, err := h.musicMgr.ResolveSong(query)
	if err != nil {
		followup(s, i, lang.T("music_song_not_found", "error", err.Error()))
		return
	}

	if h.cfg.Music.MaxSongDuration > 0 && song.Duration > h.cfg.Music.MaxSongDuration {
		dur := music.FormatDuration(song.Duration)
		maxDur := music.FormatDuration(h.cfg.Music.MaxSongDuration)
		followup(s, i, lang.T("music_song_too_long", "duration", dur, "max_duration", maxDur))
		return
	}

	song.AddedBy = i.Member.User.Username

	if err := player.JoinChannel(i.GuildID, voiceChID); err != nil {
		followup(s, i, lang.T("music_vc_join_failed", "error", err.Error()))