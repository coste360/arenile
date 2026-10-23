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
