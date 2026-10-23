package shell

import "github.com/coste360/arenile/internal/desktop"

// Action identifies an entry of the application menu. Entries with a
// Selector are handled natively by AppKit (responder chain); the others
// call a Go handler through Commands.
type Action string

// Menu actions.
const (
	ActionAbout         Action = "about"
	ActionSettings      Action = "settings"
	ActionHide          Action = "hide"
	ActionHideOthers    Action = "hide_others"
	ActionShowAll       Action = "show_all"
	ActionQuit          Action = "quit"
	ActionUndo          Action = "undo"
	ActionRedo          Action = "redo"
	ActionCut           Action = "cut"
	ActionCopy          Action = "copy"
	ActionPaste         Action = "paste"
	ActionSelectAll     Action = "select_all"
	ActionReload        Action = "reload"
	ActionZoomIn        Action = "zoom_in"
	ActionZoomOut       Action = "zoom_out"
	ActionZoomReset     Action = "zoom_reset"
	ActionFullScreen    Action = "full_screen"
	ActionOpenInBrowser Action = "open_in_browser"
	ActionMinimize      Action = "minimize"
	ActionWindowZoom    Action = "window_zoom"
	ActionClose         Action = "close"
	ActionFront         Action = "front"
)

// MenuEntry is one item of a menu. Key is the key equivalent, always used
// with Cmd (plus Shift / Option when set); Selector is the AppKit action
// sent to the first responder (e.g. "copy:" reaches the focused field of
// the WKWebView, so copy and paste work in the panel inputs).
type MenuEntry struct {
	Action    Action `json:"action,omitempty"`
	Label     string `json:"label,omitempty"`
	Key       string `json:"key,omitempty"`
	Shift     bool   `json:"shift,omitempty"`
	Option    bool   `json:"option,omitempty"`
	Control   bool   `json:"control,omitempty"`
	Selector  string `json:"selector,omitempty"`
	Separator bool   `json:"separator,omitempty"`
}

// Menu roles: the native layer makes "app" the application menu (titled
// with the app name by macOS) and "window" the Window menu (window list).
const (
	RoleApp    = "app"
	RoleEdit   = "edit"
	RoleView   = "view"
	RoleWindow = "window"
)

// Menu is a top-level menu of the menu bar.
type Menu struct {
	Role    string      `json:"role"`
	Title   string      `json:"title"`
	Entries []MenuEntry `json:"entries"`
}

var separator = MenuEntry{Separator: true}

// AppMenu is the macOS menu bar of the window in the launcher locale
// (pure apart from the locale): Arenile (Informazioni, Impostazioni,
// Nascondi, Esci), Modifica, Vista, Finestra.
func AppMenu() []Menu {
	t := desktop.T
	return []Menu{
		{Role: RoleApp, Title: "Arenile", Entries: []MenuEntry{
			{Action: ActionAbout, Label: t("app.about"), Selector: "orderFrontStandardAboutPanel:"},
			separator,
			{Action: ActionSettings, Label: t("app.settings"), Key: ","},
			separator,
			{Action: ActionHide, Label: t("app.hide"), Key: "h", Selector: "hide:"},
			{Action: ActionHideOthers, Label: t("app.hide_others"), Key: "h", Option: true, Selector: "hideOtherApplications:"},
			{Action: ActionShowAll, Label: t("app.show_all"), Selector: "unhideAllApplications:"},
			separator,
			{Action: ActionQuit, Label: t("app.quit"), Key: "q"},
		}},
		{Role: RoleEdit, Title: t("app.edit"), Entries: []MenuEntry{
			{Action: ActionUndo, Label: t("app.undo"), Key: "z", Selector: "undo:"},
			{Action: ActionRedo, Label: t("app.redo"), Key: "z", Shift: true, Selector: "redo:"},
			separator,
			{Action: ActionCut, Label: t("app.cut"), Key: "x", Selector: "cut:"},
			{Action: ActionCopy, Label: t("app.copy"), Key: "c", Selector: "copy:"},
			{Action: ActionPaste, Label: t("app.paste"), Key: "v", Selector: "paste:"},
			{Action: ActionSelectAll, Label: t("app.select_all"), Key: "a", Selector: "selectAll:"},
		}},
		{Role: RoleView, Title: t("app.view"), Entries: []MenuEntry{
			{Action: ActionReload, Label: t("app.reload"), Key: "r"},
			separator,
			{Action: ActionZoomReset, Label: t("app.zoom_reset"), Key: "0"},
			{Action: ActionZoomIn, Label: t("app.zoom_in"), Key: "+"},
			{Action: ActionZoomOut, Label: t("app.zoom_out"), Key: "-"},
			separator,
			{Action: ActionFullScreen, Label: t("app.full_screen"), Key: "f", Control: true, Selector: "toggleFullScreen:"},
			separator,
			{Action: ActionOpenInBrowser, Label: t("menu.open_browser")},
		}},
		{Role: RoleWindow, Title: t("app.window"), Entries: []MenuEntry{
			{Action: ActionMinimize, Label: t("app.minimize"), Key: "m", Selector: "performMiniaturize:"},
			{Action: ActionWindowZoom, Label: t("app.window_zoom"), Selector: "performZoom:"},
			{Action: ActionClose, Label: t("app.close"), Key: "w", Selector: "performClose:"},
			separator,
			{Action: ActionFront, Label: t("app.front"), Selector: "arrangeInFront:"},
		}},
	}
}

// Commands are the Go handlers of the menu entries without a Selector.
type Commands struct {
	Quit          func()
	OpenSettings  func()
	OpenInBrowser func()
	Reload        func()
	// Zoom changes the page zoom: >0 in, <0 out, 0 reset.
	Zoom func(dir int)
}

// Handler returns the Go handler of action (pure wiring); ok is false for
// unknown actions and for native ones.
func (c Commands) Handler(a Action) (fn func(), ok bool) {
	var h func()
	switch a {
	case ActionQuit:
		h = c.Quit
	case ActionSettings:
		h = c.OpenSettings
	case ActionOpenInBrowser:
		h = c.OpenInBrowser
	case ActionReload:
		h = c.Reload
	case ActionZoomIn, ActionZoomOut, ActionZoomReset:
		if c.Zoom != nil {
			dir := map[Action]int{ActionZoomIn: 1, ActionZoomOut: -1, ActionZoomReset: 0}[a]
			h = func() { c.Zoom(dir) }
		}
	}
	return h, h != nil
}

// Dispatch runs the handler of action and reports whether one ran.
func (c Commands) Dispatch(a Action) bool {
	if fn, ok := c.Handler(a); ok {
		fn()
		return true
	}
	return false
}
