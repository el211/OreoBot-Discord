package handlers

import (
	"testing"

	"discord-bot/config"
)

func TestBuildCustomCommands(t *testing.T) {
	cfg := &config.Config{
		CustomCommands: []config.CustomCommandConfig{
			{Name: "hello", Message: "Hello, world!", Description: "Say hello"},
			{Name: "UPPER", Message: "uppercase name"},
			{Name: "", Message: "no name, should be skipped"},
			{Name: "noMessage", Message: ""},
		},
	}

	cmds := buildCustomCommands(cfg)

	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}

	for _, cmd := range cmds {
		if cmd.Name != "hello" && cmd.Name != "upper" {
			t.Errorf("unexpected command name: %q", cmd.Name)
		}
	}
}

func TestLookupCustomCommand(t *testing.T) {
	h := &Handler{
		customCmds: map[string]config.CustomCommandConfig{
			"hello": {Name: "hello", Message: "Hello!", Ephemeral: true},
		},
	}

	cc, ok := h.lookupCustomCommand("hello")
	if !ok {
		t.Fatal("expected to find command 'hello'")
	}
	if cc.Message != "Hello!" {
		t.Errorf("expected message 'Hello!', got %q", cc.Message)
	}

	_, ok = h.lookupCustomCommand("notfound")
	if ok {
		t.Error("expected command 'notfound' not to exist")
	}
}

func TestRegisterCustomCommands(t *testing.T) {
	h := NewHandler(&config.Config{
		CustomCommands: []config.CustomCommandConfig{
			{Name: "ping", Message: "Pong!"},
			{Name: "help", Message: "Help text"},
			{Name: "", Message: "skipped"},
		},
	}, nil)

	h.RegisterCustomCommands()

	if len(h.customCmds) != 2 {
		t.Errorf("expected 2 custom commands, got %d", len(h.customCmds))
	}

	if _, ok := h.customCmds["ping"]; !ok {
		t.Error("expected 'ping' command to be registered")
	}
	if _, ok := h.customCmds["help"]; !ok {
		t.Error("expected 'help' command to be registered")
	}
}

func TestApplyPlaceholders(t *testing.T) {
	tests := []struct {
		msg         string
		mention     string
		username    string
		guildName   string
		memberCount int
		want        string
	}{
		{"{user} joined!", "<@123>", "Alice", "MyServer", 42, "<@123> joined!"},
		{"Hello {username}", "<@123>", "Bob", "MyServer", 1, "Hello Bob"},
		{"{server} has {member_count} members", "<@1>", "u", "Cool", 99, "Cool has 99 members"},
		{"no placeholders", "<@1>", "u", "s", 0, "no placeholders"},
		{"{user} {user}", "<@5>", "x", "s", 0, "<@5> <@5>"},
	}
	for _, tt := range tests {
		got := applyPlaceholders(tt.msg, tt.mention, tt.username, tt.guildName, tt.memberCount)
		if got != tt.want {
			t.Errorf("applyPlaceholders() = %q, want %q", got, tt.want)
		}
	}
}
