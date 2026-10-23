//go:build !windows

package desktop

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// platformLocaleTags returns the macOS AppleLocale ("it_IT") when the
// environment carries no language (apps started from the Finder have no
// LANG); nothing on other systems, where LANG is the source of truth.
func platformLocaleTags() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleLocale").Output()
	if err != nil {
		return nil
	}
	return []string{strings.TrimSpace(string(out))}
}
