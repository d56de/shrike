// Package notify sends desktop notifications on macOS.
package notify

import "os/exec"

// Notification is one OS notification.
type Notification struct {
	Title    string
	Subtitle string // optional
	Message  string
	Group    string // terminal-notifier -group (replaces prior in the group); ignored by osascript
}

// Notifier sends notifications.
type Notifier interface {
	Notify(n Notification) error
}

// System is the production Notifier: terminal-notifier if on PATH, else osascript.
type System struct {
	lookPath func(string) (string, error)            // injected; default exec.LookPath
	run      func(name string, args ...string) error // injected; default exec.Command(...).Run
}

// NewSystem returns a System backed by exec.LookPath and exec.Command.
func NewSystem() *System {
	return &System{
		lookPath: exec.LookPath,
		run: func(name string, args ...string) error {
			return exec.Command(name, args...).Run() //nolint:gosec // fixed command names, values passed as args
		},
	}
}

// Notify sends n via terminal-notifier when available, else osascript.
func (s *System) Notify(n Notification) error {
	if _, err := s.lookPath("terminal-notifier"); err == nil {
		return s.run("terminal-notifier", terminalNotifierArgs(n)...)
	}
	return s.run("osascript", osascriptArgs(n)...)
}

func terminalNotifierArgs(n Notification) []string {
	args := []string{"-title", n.Title, "-message", n.Message}
	if n.Subtitle != "" {
		args = append(args, "-subtitle", n.Subtitle)
	}
	if n.Group != "" {
		args = append(args, "-group", n.Group)
	}
	return args
}

// osascriptArgs builds an AppleScript that reads its values from argv, so the
// Title/Message/Subtitle are never interpolated into the script text (no
// quoting or injection issues). ASCII "> 2" means "3 or more args".
func osascriptArgs(n Notification) []string {
	args := []string{
		"-e", "on run argv",
		"-e", "if (count of argv) > 2 then",
		"-e", "display notification (item 1 of argv) with title (item 2 of argv) subtitle (item 3 of argv)",
		"-e", "else",
		"-e", "display notification (item 1 of argv) with title (item 2 of argv)",
		"-e", "end if",
		"-e", "end run",
		"--", n.Message, n.Title,
	}
	if n.Subtitle != "" {
		args = append(args, n.Subtitle)
	}
	return args
}
