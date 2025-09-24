package handlers

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestHasRoleInGuild(t *testing.T) {
	guild := &discordgo.Guild{
		Roles: []*discordgo.Role{
			{ID: "r1", Name: "Admin"},
			{ID: "r2", Name: "Moderator"},
			{ID: "r3", Name: "Member"},
		},
	}

	tests := []struct {
		name         string
		member       *discordgo.Member
		allowedNames []string
		want         bool
	}{
		{
			name:         "nil member returns false",
			member:       nil,
			allowedNames: []string{"Admin"},
			want:         false,
		},
		{
			name:         "empty allowed names returns false",
			member:       &discordgo.Member{Roles: []string{"r1"}},
			allowedNames: nil,
			want:         false,
		},
		{
			name:         "member has matching role",
			member:       &discordgo.Member{Roles: []string{"r1"}},
			allowedNames: []string{"Admin"},
			want:         true,
		},
		{
			name:         "member does not have matching role",
			member:       &discordgo.Member{Roles: []string{"r3"}},
			allowedNames: []string{"Admin"},
			want:         false,
		},
		{
			name:         "member has one of several allowed roles",
			member:       &discordgo.Member{Roles: []string{"r2"}},
			allowedNames: []string{"Admin", "Moderator"},
			want:         true,
		},
		{
			name:         "nil guild returns false",
			member:       &discordgo.Member{Roles: []string{"r1"}},
			allowedNames: []string{"Admin"},
			want:         false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := guild
			if tc.name == "nil guild returns false" {
				g = nil
			}
			got := hasRoleInGuild(g, tc.member, tc.allowedNames)
			if got != tc.want {
				t.Errorf("hasRoleInGuild() = %v, want %v", got, tc.want)
			}
		})
	}
}
