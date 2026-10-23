package desktop

import (
	"path/filepath"
	"testing"
)

// abs builds an absolute path from components on the current OS.
func abs(parts ...string) string {
	return filepath.Join(append([]string{string(filepath.Separator)}, parts...)...)
}

func TestDataDir_PerOS(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	tests := []struct {
		name, goos, home string
		env              map[string]string
		want             string
		wantErr          bool
	}{
		{"windows appdata", "windows", abs("h"), map[string]string{"APPDATA": abs("appdata")}, abs("appdata", "Arenile"), false},
		{"windows fallback", "windows", abs("h"), nil, abs("h", "AppData", "Roaming", "Arenile"), false},
		{"macos", "darwin", abs("Users", "anna"), nil, abs("Users", "anna", "Library", "Application Support", "Arenile"), false},
		{"linux xdg", "linux", abs("home", "anna"), map[string]string{"XDG_DATA_HOME": abs("xdg")}, abs("xdg", "arenile"), false},
		{"linux relative xdg ignored", "linux", abs("home", "anna"), map[string]string{"XDG_DATA_HOME": "rel"}, abs("home", "anna", ".local", "share", "arenile"), false},
		{"no home", "darwin", "", nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DataDir(tt.goos, env(tt.env), tt.home)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("DataDir = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
