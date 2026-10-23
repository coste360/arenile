package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_VersionAndHelp(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"-version"}, &out); code != 0 || !strings.Contains(out.String(), "arenile-desktop") {
		t.Fatalf("version: %d %q", code, out.String())
	}
	if code := run([]string{"-h"}, &out); code != 0 {
		t.Fatalf("help: %d", code)
	}
	if code := run([]string{"-unknown"}, &out); code != 2 {
		t.Fatalf("bad flag: %d", code)
	}
}

func TestStripFragment(t *testing.T) {
	if got := stripFragment("http://127.0.0.1:5/admin/first-run#token=secret"); got != "http://127.0.0.1:5/admin/first-run" {
		t.Fatal(got)
	}
}

func TestPairInput_QRorURL(t *testing.T) {
	in := pairInput("arenile://pair?d=abc", "", "", "cassa")
	if in.QR != "arenile://pair?d=abc" || in.URL != "" || in.DeviceName != "cassa" {
		t.Fatalf("qr: %+v", in)
	}
	in = pairInput("https://lido.example.it", "ABCD-EFGH", "AA:BB", "")
	if in.URL != "https://lido.example.it" || in.Code != "ABCD-EFGH" || in.Fingerprint != "AA:BB" || in.QR != "" {
		t.Fatalf("url: %+v", in)
	}
}

func TestRun_PairFailsCleanly(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"-data-dir", t.TempDir(), "-pair", "http://127.0.0.1:1", "-pair-code", "ABCD-EFGH"}, &out)
	if code != 1 || !strings.Contains(out.String(), "abbinamento non riuscito") {
		t.Fatalf("pair: %d %q", code, out.String())
	}
}
