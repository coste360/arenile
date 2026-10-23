//go:build !windows

package desktop

// platformRunKey: the registry exists only on Windows.
func platformRunKey() RunKey { return nil }
