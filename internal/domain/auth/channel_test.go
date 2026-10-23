package auth

import (
	"testing"

	"github.com/coste360/arenile/internal/store"
)

func TestChannelFor_APIKeyAlwaysAPI(t *testing.T) {
	key := &Principal{Kind: PrincipalAPIKey}
	user := &Principal{Kind: PrincipalUser}
	tests := []struct {
		name      string
		p         *Principal
		requested string
		want      string
	}{
		{"key asking desk", key, store.ChannelDesk, store.ChannelAPI},
		{"key asking online", key, store.ChannelOnline, store.ChannelAPI},
		{"key asking nothing", key, "", store.ChannelAPI},
		{"user desk", user, store.ChannelDesk, store.ChannelDesk},
		{"user default", user, "", ""},
		{"user api", user, store.ChannelAPI, store.ChannelAPI},
		{"no principal", nil, store.ChannelDesk, store.ChannelDesk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ChannelFor(tt.p, tt.requested); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}
