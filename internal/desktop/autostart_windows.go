//go:build windows

package desktop

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

// platformRunKey returns HKCU\Software\Microsoft\Windows\CurrentVersion\Run.
func platformRunKey() RunKey { return winRunKey{} }

type winRunKey struct{}

func (winRunKey) open(access uint32) (registry.Key, error) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, WindowsRunKey, access)
	return k, err
}

func (w winRunKey) Get(name string) (string, bool, error) {
	k, err := w.open(registry.QUERY_VALUE)
	if err != nil {
		return "", false, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (w winRunKey) Set(name, value string) error {
	k, err := w.open(registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}

func (w winRunKey) Delete(name string) error {
	k, err := w.open(registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
