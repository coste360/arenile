//go:build desktop

package main

import (
	"context"
	"log/slog"
	"runtime"
	"sync"

	"fyne.io/systray"

	"github.com/coste360/arenile/internal/desktop"
	"github.com/coste360/arenile/internal/desktop/shell"
)

// newTrayUI returns the tray UI (fyne.io/systray, Apache-2.0: CGO on macOS,
// D-Bus StatusNotifierItem on Linux, Win32 on Windows), or the headless UI
// with -no-tray. The panel opens in the default browser.
func newTrayUI(log *slog.Logger, o uiOptions) desktop.UI {
	h := newHeadlessUI(log, o.browser)
	if o.noTray {
		return h
	}
	t := &trayUI{headlessUI: h}
	// A click on a native notification (macOS) opens its panel page in the
	// browser, where this build shows the panel.
	shell.SetNotificationHandler(h.openPage)
	shell.InitNotifications()
	return t
}

// trayUI shows the menu in the system tray. The menu items are created
// once and updated in place by SetMenu (title, check mark, visibility):
// systray cannot rebuild a menu without leaking the click channels.
// Browser, folder and notifications are inherited from headlessUI.
type trayUI struct {
	*headlessUI

	mu       sync.Mutex
	items    map[string]*systray.MenuItem
	handlers map[string]func()
	pending  []desktop.MenuItem
	ready    bool
	// stopped: the event loop is ending; systray calls would wait forever
	// for a main-thread run loop that no longer runs (macOS).
	stopped bool
}

// stop makes SetMenu a no-op (before the window event loop returns).
func (t *trayUI) stop() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
}

func (t *trayUI) Run(ctx context.Context, menu []desktop.MenuItem) error {
	t.mu.Lock()
	t.pending = menu
	t.mu.Unlock()
	stop := context.AfterFunc(ctx, systray.Quit)
	defer stop()
	go func() {
		<-t.quit
		systray.Quit()
	}()
	// Blocks on the main thread (systray locks it in its init).
	systray.Run(t.onReady, func() {})
	return nil
}

// onReady sets the icon and builds the pending menu (systray callback).
func (t *trayUI) onReady() {
	if runtime.GOOS == "windows" {
		systray.SetIcon(desktop.ICOFromPNG(desktop.TrayIconPNG(16, false), desktop.TrayIconPNG(32, false)))
	} else {
		systray.SetTemplateIcon(desktop.TrayIconPNG(44, true), desktop.TrayIconPNG(44, false))
	}
	systray.SetTooltip("Arenile")
	t.mu.Lock()
	t.ready = true
	menu := t.pending
	t.mu.Unlock()
	t.SetMenu(menu)
}

// SetMenu implements desktop.UI.
func (t *trayUI) SetMenu(menu []desktop.MenuItem) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	if !t.ready {
		t.pending = menu
		return
	}
	if t.items == nil {
		t.items, t.handlers = map[string]*systray.MenuItem{}, map[string]func(){}
	}
	seen := map[string]bool{}
	for _, it := range menu {
		seen[it.ID] = true
		t.handlers[it.ID] = it.OnClick
		mi, ok := t.items[it.ID]
		if !ok {
			if it.ID == desktop.MenuRestore || it.ID == desktop.MenuQuit {
				systray.AddSeparator()
			}
			if it.Checkbox {
				mi = systray.AddMenuItemCheckbox(it.Label, "", it.Checked)
			} else {
				mi = systray.AddMenuItem(it.Label, "")
			}
			t.items[it.ID] = mi
			go t.listen(it.ID, mi)
		}
		mi.SetTitle(it.Label)
		if it.Checkbox {
			if it.Checked {
				mi.Check()
			} else {
				mi.Uncheck()
			}
		}
		if it.Disabled {
			mi.Disable()
		} else {
			mi.Enable()
		}
		mi.Show()
	}
	for id, mi := range t.items {
		if !seen[id] {
			mi.Hide()
		}
	}
}

func (t *trayUI) listen(id string, mi *systray.MenuItem) {
	for range mi.ClickedCh {
		t.mu.Lock()
		fn := t.handlers[id]
		t.mu.Unlock()
		if fn != nil {
			fn()
		}
	}
}

// menuAction runs the handler of the tray menu item id: the macOS app
// menu reuses the launcher's entries ("Verifica aggiornamenti").
func (t *trayUI) menuAction(id string) {
	t.mu.Lock()
	fn := t.handlers[id]
	if fn == nil {
		for _, it := range t.pending {
			if it.ID == id {
				fn = it.OnClick
			}
		}
	}
	t.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Quit implements desktop.UI.
func (t *trayUI) Quit() { t.headlessUI.Quit() }
