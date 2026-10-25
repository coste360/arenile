package meta

import "testing"

// TestBuild_PushNotifications: the capability needs a relay key and the
// server edition; the desktop edition never offers push (ADR 0027).
func TestBuild_PushNotifications(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want bool
	}{
		{"server without key", Input{}, false},
		{"server with key", Input{PushNotifications: true}, true},
		{"desktop lan with key", Input{PushNotifications: true, Runtime: Runtime{Edition: EditionDesktop, Desktop: &Desktop{Mode: DesktopLAN}}}, false},
	}
	for _, tt := range tests {
		if got := Build(tt.in).Capabilities.PushNotifications; got != tt.want {
			t.Errorf("%s: push_notifications = %v, want %v", tt.name, got, tt.want)
		}
	}
	if !NewService(Deps{PushNotifications: true}).Get(t.Context()).Capabilities.PushNotifications {
		t.Error("Service.Get ignores Deps.PushNotifications")
	}
}
