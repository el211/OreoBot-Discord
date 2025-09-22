package handlers

import "github.com/bwmarrin/discordgo"

func hasRoleInGuild(guild *discordgo.Guild, member *discordgo.Member, allowedNames []string) bool {
	if guild == nil || member == nil || len(allowedNames) == 0 {
		return false
	}

	nameSet := make(map[string]bool, len(allowedNames))
	for _, n := range allowedNames {
		if n != "" {
			nameSet[n] = true
		}
	}

	memberRoles := make(map[string]bool, len(member.Roles))
	for _, id := range member.Roles {
		memberRoles[id] = true
	}

	for _, role := range guild.Roles {
		if nameSet[role.Name] && memberRoles[role.ID] {
			return true
		}
	}
	return false
}
