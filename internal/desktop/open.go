package desktop

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OpenCommand returns the command that opens target (a web link or a
// local folder) with the desktop's default handler on goos (pure). The
// target is a single argument, never interpreted by a shell.
func OpenCommand(goos, target string) (name string, args []string) {
	switch goos {
	case "darwin":
		return "open", []string{target}
	case "windows":
		if IsWebLink(target) {
			// rundll32 avoids cmd.exe and its "start" quoting rules.
			return "rundll32", []string{"url.dll,FileProtocolHandler", target}
		}
		return "explorer", []string{target}
	default:
		return "xdg-open", []string{target}
	}
}

// IsWebLink reports whether s is an absolute http(s) URL with a host or a
// mailto: address (pure). Everything else (file:, javascript:, custom
// schemes, relative or absolute paths, UNC paths, flags) is not a link
// that may be handed to the operating system by untrusted code.
func IsWebLink(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "http", "https":
		return u.Host != "" && u.User == nil
	case "mailto":
		return u.Opaque != "" && u.Host == ""
	}
	return false
}

// ErrUnsafeTarget: OpenURL and OpenFolder refuse anything that is not,
// respectively, a web link or an existing absolute directory.
var ErrUnsafeTarget = errors.New("desktop: refusing to open this target")

// OpenURL opens a web link (IsWebLink) in the default browser or mail
// client without waiting for it. It is the only opener reachable from
// JavaScript in the remote-mode window: a page can never start a local
// program or file through it.
func OpenURL(ctx context.Context, goos, link string) error {
	if !IsWebLink(link) {
		return ErrUnsafeTarget
	}
	name, args := OpenCommand(goos, link)
	return start(ctx, name, args)
}

// OpenFolder opens an existing absolute directory in the file manager. It
// is called only from the tray menu (never from the window).
func OpenFolder(ctx context.Context, goos, dir string) error {
	if err := checkFolder(dir); err != nil {
		return err
	}
	name, args := OpenCommand(goos, filepath.Clean(dir))
	return start(ctx, name, args)
}

// checkFolder accepts an existing absolute directory: no relative path,
// UNC path (\\host\share, which explorer would run), flag or URL.
func checkFolder(dir string) error {
	if !filepath.IsAbs(dir) || strings.HasPrefix(dir, `\\`) || strings.HasPrefix(dir, "-") || strings.Contains(dir, "://") {
		return ErrUnsafeTarget
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return ErrUnsafeTarget
	}
	return nil
}

func start(ctx context.Context, name string, args []string) error {
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
