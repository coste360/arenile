//go:build windows

package desktop

import "golang.org/x/sys/windows"

// platformLocaleTags returns the Windows UI languages in preference order
// ("it-IT", "en-GB", ...).
func platformLocaleTags() []string {
	tags, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil {
		return nil
	}
	return tags
}
