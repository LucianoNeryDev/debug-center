package agent

import (
	"fmt"
	"strconv"
	"strings"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

// action is a catalogue entry. Command may reference parameters as {{1}},
// {{2}}…; each is quoted for the action's shell before substitution.
type action struct {
	proto.Action
	Command string
}

func (a action) render(args []string) (string, error) {
	if len(args) != len(a.Params) {
		if len(a.Params) == 0 {
			return "", fmt.Errorf("action %s takes no arguments", a.Name)
		}
		return "", fmt.Errorf("action %s needs %d argument(s): %s", a.Name, len(a.Params), strings.Join(a.Params, " "))
	}
	line := a.Command
	for i, arg := range args {
		q, err := quoteArg(a.Shell, arg)
		if err != nil {
			return "", err
		}
		line = strings.ReplaceAll(line, "{{"+strconv.Itoa(i+1)+"}}", q)
	}
	return line, nil
}

// Actions lists the catalogue for this OS.
func Actions() []proto.Action {
	out := make([]proto.Action, len(catalogue))
	for i, a := range catalogue {
		out[i] = a.Action
		if out[i].Shell == "" {
			out[i].Shell = defaultShell
		}
	}
	return out
}

func findAction(name string) (action, bool) {
	for _, a := range catalogue {
		if a.Name == name {
			return a, true
		}
	}
	return action{}, false
}

func replaceAll(s, old, new string) string { return strings.ReplaceAll(s, old, new) }

// def is shorthand for catalogue entries.
func def(name, desc, command string, opts ...func(*action)) action {
	a := action{Action: proto.Action{Name: name, Description: desc}, Command: command}
	for _, o := range opts {
		o(&a)
	}
	return a
}

func dangerous(a *action) { a.Dangerous = true }
func slow(a *action)      { a.Slow = true }
func shell(s string) func(*action) {
	return func(a *action) { a.Shell = s }
}
func params(p ...string) func(*action) {
	return func(a *action) { a.Params = p }
}
