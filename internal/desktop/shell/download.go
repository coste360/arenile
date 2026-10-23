package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	fallbackFileName = "download"
	maxFileNameBytes = 200
	maxUniqueTries   = 1000
)

// DownloadsDir is the folder where the window saves downloads (pure):
// ~/Downloads on every OS (the native layer of macOS uses it; WebView2 on
// Windows saves to the user's Downloads known folder by itself).
func DownloadsDir(home string) string {
	if home == "" {
		return ""
	}
	return filepath.Join(home, "Downloads")
}

// SanitizeFileName turns the name suggested by the page or by the server
// (Content-Disposition) into a safe base name (pure): no directory part, no
// path separators, control characters or characters reserved on Windows, no
// leading dots (hidden files) or trailing dots and spaces, no reserved
// device names, at most maxFileNameBytes bytes with the extension kept.
// An empty result becomes "download".
func SanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		// Cf: bidi overrides and zero-width characters that disguise an extension.
		case r == utf8.RuneError, unicode.IsControl(r), unicode.Is(unicode.Cf, r), strings.ContainsRune(`<>:"|?*`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	name = strings.TrimLeft(strings.TrimSpace(b.String()), ".")
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return fallbackFileName
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if isReservedDeviceName(stem) {
		stem = "_" + stem
	}
	if len(ext) > 20 {
		ext = ""
		stem = name
	}
	for len(stem)+len(ext) > maxFileNameBytes {
		_, size := utf8.DecodeLastRuneInString(stem)
		stem = stem[:len(stem)-size]
	}
	if stem == "" {
		stem = fallbackFileName
	}
	return stem + ext
}

func isReservedDeviceName(stem string) bool {
	s := strings.ToUpper(stem)
	switch s {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(s) == 4 && (strings.HasPrefix(s, "COM") || strings.HasPrefix(s, "LPT")) && s[3] >= '1' && s[3] <= '9' {
		return true
	}
	return false
}

// UniquePath returns dir/name, or "name (1).ext", "name (2).ext", ... when
// the file exists (exists is os.Stat in production, a fake in tests): a
// download never overwrites a file. name must be sanitized already.
func UniquePath(dir, name string, exists func(string) bool) (string, error) {
	p := filepath.Join(dir, name)
	if !exists(p) {
		return p, nil
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; i <= maxUniqueTries; i++ {
		p = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if !exists(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("shell: no free file name for %q in %s", name, dir)
}

// DownloadPath chooses where a download suggested as name is saved in dir:
// sanitized and unique. The folder is created if missing.
func DownloadPath(dir, suggested string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("shell: no downloads folder")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: the user's Downloads folder
		return "", err
	}
	return UniquePath(dir, SanitizeFileName(suggested), func(p string) bool {
		_, err := os.Lstat(p)
		return err == nil
	})
}
