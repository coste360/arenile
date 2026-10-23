package notify

import (
	"context"
	"testing"
)

func TestMaskEmail_Scenarios(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"mario.rossi@example.it", "m***i@e***.it"},
		{"ab@x.com", "a***@x***.com"},
		{"anna@mail.example.co.uk", "a***a@m***.uk"},
		{"nobody", "***"},
		{"@example.it", "***"},
		{"luca@localhost", "l***a@l***"},
	}
	for _, tt := range tests {
		if got := MaskEmail(tt.in); got != tt.want {
			t.Errorf("MaskEmail(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNewTransport_Selection(t *testing.T) {
	tests := []struct {
		smtp, dir, want string
	}{
		{"smtp://u:p@mail:587", "/tmp/mail", TransportSMTP},
		{"", "/tmp/mail", TransportFile},
		{"", "", TransportLog},
	}
	for _, tt := range tests {
		if got := NewTransport(tt.smtp, tt.dir, nil).Kind(); got != tt.want {
			t.Errorf("NewTransport(%q, %q).Kind() = %q, want %q", tt.smtp, tt.dir, got, tt.want)
		}
	}
}

func TestMemoryTransport_KeepsSent(t *testing.T) {
	var m MemoryTransport
	if _, err := m.Send(context.Background(), Email{To: "a@b.it", DedupeID: "1"}); err != nil {
		t.Fatal(err)
	}
	if got := m.Sent(); len(got) != 1 || got[0].DedupeID != "1" {
		t.Fatalf("sent = %+v", got)
	}
}
