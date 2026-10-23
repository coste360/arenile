package desktop

import (
	"errors"
	"path/filepath"
)

// AppDirName is the data directory name on Windows and macOS; Linux uses
// the lowercase form (XDG convention).
const AppDirName = "Arenile"

// ErrNoDataDir: the environment does not allow resolving a data directory.
var ErrNoDataDir = errors.New("desktop: cannot resolve the data directory")

// DataDir returns the per-user data directory of the desktop edition for
// goos (runtime.GOOS), reading the environment through getenv and the home
// directory from home. Pure: tested for every OS on any OS.
//
//	windows: %APPDATA%\Arenile
//	darwin:  $HOME/Library/Application Support/Arenile
//	other:   $XDG_DATA_HOME/arenile, else $HOME/.local/share/arenile
func DataDir(goos string, getenv func(string) string, home string) (string, error) {
	switch goos {
	case "windows":
		if d := getenv("APPDATA"); d != "" {
			return filepath.Join(d, AppDirName), nil
		}
		if home != "" {
			return filepath.Join(home, "AppData", "Roaming", AppDirName), nil
		}
	case "darwin":
		if home != "" {
			return filepath.Join(home, "Library", "Application Support", AppDirName), nil
		}
	default:
		if d := getenv("XDG_DATA_HOME"); d != "" && filepath.IsAbs(d) {
			return filepath.Join(d, "arenile"), nil
		}
		if home != "" {
			return filepath.Join(home, ".local", "share", "arenile"), nil
		}
	}
	return "", ErrNoDataDir
}
