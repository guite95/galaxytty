package tui

import (
	"fmt"
	"strings"
)

type Command struct {
	Name string
	Args []string
}

var commands = map[string]bool{"help": true, "exit": true, "quit": true, "search": true, "unread": true, "new": true, "notify": true, "status": true, "reconnect": true}

func ParseCommand(v string) (Command, error) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "/") {
		return Command{}, fmt.Errorf("slash command must start with /")
	}
	f := strings.Fields(strings.TrimPrefix(v, "/"))
	if len(f) == 0 || !commands[f[0]] {
		return Command{}, fmt.Errorf("unknown command")
	}
	return Command{Name: f[0], Args: f[1:]}, nil
}
