package shell

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// Window geometry limits (logical points on macOS, pixels at 100% on Windows).
const (
	DefaultWidth  = 1280
	DefaultHeight = 820
	// MinWidth x MinHeight keeps the panel layout usable (sidebar plus a
	// table) and fits a 1366x768 laptop and a Surface at 200%.
	MinWidth  = 960
	MinHeight = 600
	maxSide   = 16384

	stateFileName = "window.json"
)

// ZoomLevels are the page zoom steps of the Vista menu (Cmd+, Cmd-, Cmd0).
var ZoomLevels = []float64{0.5, 0.67, 0.75, 0.8, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2}

// WindowState is the panel window geometry remembered between runs in
// <data dir>/window.json. X and Y are in the platform's screen coordinates
// (Cocoa: bottom-left origin; Win32: workspace coordinates) and are used
// only when Placed is true; the native layer moves a window that would end
// up off every screen back to the centre.
type WindowState struct {
	X         int     `json:"x"`
	Y         int     `json:"y"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Placed    bool    `json:"placed"`
	Maximized bool    `json:"maximized"`
	Zoom      float64 `json:"zoom"`
}

// DefaultState is the geometry of the first start: centred, default size,
// 100% zoom.
func DefaultState() WindowState {
	return WindowState{Width: DefaultWidth, Height: DefaultHeight, Zoom: 1}
}

// Normalize clamps the state to sane values (pure): size between the
// minimum and maxSide (a missing size takes the default), zoom on one of
// ZoomLevels, coordinates kept only when the window was placed.
func (s WindowState) Normalize() WindowState {
	if s.Width <= 0 || s.Height <= 0 {
		s.Width, s.Height = DefaultWidth, DefaultHeight
	}
	s.Width = clamp(s.Width, MinWidth, maxSide)
	s.Height = clamp(s.Height, MinHeight, maxSide)
	if !s.Placed || abs(s.X) > maxSide*4 || abs(s.Y) > maxSide*4 {
		s.X, s.Y, s.Placed = 0, 0, false
	}
	s.Zoom = nearestZoom(s.Zoom)
	return s
}

// NextZoom returns the zoom step after current in direction dir (>0 in,
// <0 out, 0 reset to 100%) (pure).
func NextZoom(current float64, dir int) float64 {
	if dir == 0 {
		return 1
	}
	i := zoomIndex(nearestZoom(current))
	if dir > 0 && i < len(ZoomLevels)-1 {
		i++
	} else if dir < 0 && i > 0 {
		i--
	}
	return ZoomLevels[i]
}

func nearestZoom(z float64) float64 {
	if z <= 0 || math.IsNaN(z) || math.IsInf(z, 0) {
		return 1
	}
	best := ZoomLevels[0]
	for _, l := range ZoomLevels {
		if math.Abs(l-z) < math.Abs(best-z) {
			best = l
		}
	}
	return best
}

func zoomIndex(z float64) int {
	for i, l := range ZoomLevels {
		if l == z {
			return i
		}
	}
	return zoomIndex(1)
}

// LoadState reads <dataDir>/window.json; a missing or corrupted file gives
// DefaultState (the window must always open).
func LoadState(dataDir string) WindowState {
	if dataDir == "" {
		return DefaultState()
	}
	b, err := os.ReadFile(filepath.Join(dataDir, stateFileName))
	if err != nil {
		return DefaultState()
	}
	s := DefaultState()
	if err := json.Unmarshal(b, &s); err != nil {
		return DefaultState()
	}
	return s.Normalize()
}

// SaveState writes <dataDir>/window.json atomically (0600).
func SaveState(dataDir string, s WindowState) error {
	if dataDir == "" {
		return errors.New("shell: no data dir")
	}
	b, err := json.MarshalIndent(s.Normalize(), "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dataDir, stateFileName)
	if err := os.WriteFile(path+".tmp", b, 0o600); err != nil {
		return fmt.Errorf("shell: save window state: %w", err)
	}
	return os.Rename(path+".tmp", path)
}

func clamp(v, lo, hi int) int {
	return max(lo, min(v, hi))
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
