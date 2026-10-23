//go:build !desktop

package main

import (
	"log/slog"

	"github.com/coste360/arenile/internal/desktop"
)

const uiKind = "headless"

func newUI(log *slog.Logger, o uiOptions) desktop.UI { return newHeadlessUI(log, o.browser) }
