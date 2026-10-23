package desktop

import (
	"context"
	"errors"
	"net/url"
	"os/exec"
	"strings"
)

// OpenCommand returns the command that opens target (an http(s) URL or a
// local folder) with the desktop's default handler on goos (pure). The
// target is a single argument, never interpreted by a shell.
func OpenCommand(goos, target string) (name string, args []string) {
	switch goos {
	case "darwin":
		return "open", []string{target}
	case "windows":
		if isWebURL(target) {
			// rundll32 avoids cmd.exe and its "start" quoting rules.
			return "rundll32", []string{"url.dll,FileProtocolHandler", target}
		}
		return "explorer", []string{target}
	default:
		return "xdg-open", []string{target}
	}
}

func isWebURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// ErrUnsafeTarget: OpenTarget refuses anything that is not an http(s) URL
// on loopback/LAN or an absolute folder path.
var ErrUnsafeTarget = errors.New("desktop: refusing to open this target")

// OpenTarget starts the default handler for target without waiting for it.
func OpenTarget(ctx context.Context, goos, target string) error {
	if !isWebURL(target) && (strings.Contains(target, "://") || strings.HasPrefix(target, "-")) {
		return ErrUnsafeTarget
	}
	name, args := OpenCommand(goos, target)
	cmd := exec.CommandContext(context.WithoutCancel(ctx), name, args...) //nolint:gosec // G204: fixed program, single validated argument
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// NotifyCommand returns the command that shows a desktop notification on
// goos (pure). ok is false where there is no dependency-free way (Windows:
// toast notifications need a registered AppUserModelID; the tray tooltip
// and the log are used instead).
func NotifyCommand(goos, title, message string) (name string, args []string, ok bool) {
	switch goos {
	case "darwin":
		q := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
		script := `display notification "` + q.Replace(message) + `" with title "` + q.Replace(title) + `"`
		return "osascript", []string{"-e", script}, true
	case "windows":
		return "", nil, false
	default:
		return "notify-send", []string{"--app-name=Arenile", "--", title, message}, true
	}
}

// Notify shows a desktop notification, best effort.
func Notify(ctx context.Context, goos, title, message string) error {
	name, args, ok := NotifyCommand(goos, title, message)
	if !ok {
		return nil
	}
	cmd := exec.CommandContext(context.WithoutCancel(ctx), name, args...) //nolint:gosec // G204: fixed program, arguments are not interpreted by a shell
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
