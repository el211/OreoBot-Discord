package handlers

import (
	"strconv"
	"strings"

	"discord-bot/lang"

	"github.com/bwmarrin/discordgo"
)

func utilityCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:                     "say",
			Description:              "Send a message as the bot in a specific channel",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel to send the message in", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "message", Description: "Message content", Required: true},
			},
		},
		{
			Name:                     "embed",
			Description:              "Create and send a custom embed in a specific channel",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Channel to send the embed in", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "title", Description: "Embed title", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "description", Description: "Embed description (use \\n for new lines)", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "colour", Description: "Hex colour (e.g. #ff0000)"},
				{Type: discordgo.ApplicationCommandOptionString, Name: "image", Description: "Image URL"},
				{Type: discordgo.ApplicationCommandOptionString, Name: "thumbnail", Description: "Thumbnail URL"},
				{Type: discordgo.ApplicationCommandOptionString, Name: "footer", Description: "Footer text"},
				{Type: discordgo.ApplicationCommandOptionString, Name: "author", Description: "Author name"},
				{Type: discordgo.ApplicationCommandOptionString, Name: "author-icon", Description: "Author icon URL"},
				{Type: discordgo.ApplicationCommandOptionString, Name: "url", Description: "Title hyperlink URL"},
			},
		},
		{
			Name:                     "renamechannel",
			Description:              "Rename the current channel",
			DefaultMemberPermissions: &adminPerm,
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "New channel name", Required: true, MaxLength: 100},
			},
		},
	}
}

func handleRenameChannel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	name := strings.TrimSpace(optionMap(i)["name"].StringValue())
	if name == "" {
		respond(s, i, lang.T("channel_rename_invalid"), true)
		return
	}

	if _, err := s.ChannelEdit(i.ChannelID, &discordgo.ChannelEdit{Name: name}); err != nil {
		respond(s, i, lang.T("channel_rename_failed", "error", err.Error()), true)
		return
	}

	respond(s, i, lang.T("channel_renamed", "name", name), false)
}

func handleSay(s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := optionMap(i)
	ch := opts["channel"].ChannelValue(s)
	msg := opts["message"].StringValue()

	msg = strings.ReplaceAll(msg, "\\n", "\n")

	_, err := s.ChannelMessageSend(ch.ID, msg)
	if err != nil {
		respond(s, i, lang.T("say_failed", "error", err.Error()), true)
		return
	}

	respond(s, i, lang.T("say_success", "channel_id", ch.ID), true)
}

func handleEmbed(s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := optionMap(i)
	ch := opts["channel"].ChannelValue(s)
	title := opts["title"].StringValue()
	desc := opts["description"].StringValue()

	desc = strings.ReplaceAll(desc, "\\n", "\n")
	title = strings.ReplaceAll(title, "\\n", "\n")

	colour := 0
	if c, ok := opts["colour"]; ok {
		hex := strings.TrimPrefix(c.StringValue(), "#")
		if v, err := strconv.ParseInt(hex, 16, 64); err == nil {
			colour = int(v)
		}
	}
