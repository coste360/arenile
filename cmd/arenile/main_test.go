package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type cliEnv struct {
	vars map[string]string
}

func (e cliEnv) lookup(k string) (string, bool) {
	v, ok := e.vars[k]
	return v, ok
}

func runCLI(t *testing.T, e cliEnv, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(context.Background(), args, strings.NewReader(stdin), &out, &errb, e.lookup)
	return code, out.String(), errb.String()
}

func TestCLI_Lifecycle(t *testing.T) {
	dir := t.TempDir()
	e := cliEnv{vars: map[string]string{"ARENILE_DEV": "true", "ARENILE_DATA_DIR": dir, "ARENILE_LOG_LEVEL": "error"}}

	code, out, _ := runCLI(t, e, "", "version")
	if code != 0 || !strings.HasPrefix(out, "arenile ") {
		t.Fatalf("version: %d %q", code, out)
	}
	if code, _, _ := runCLI(t, e, "", "bogus"); code != 2 {
		t.Fatalf("unknown command: %d", code)
	}
	if code, _, _ := runCLI(t, e, ""); code != 2 {
		t.Fatalf("no command: %d", code)
	}

	code, out, errOut := runCLI(t, e, "", "migrate", "status")
	if code != 0 || !strings.Contains(out, "in sospeso") {
		t.Fatalf("status before up: %d %q %q", code, out, errOut)
	}
	if !strings.Contains(errOut, "ARENILE_SECRET_KEY=") {
		t.Fatal("generated dev key must be printed once on stderr")
	}
	code, out, errOut = runCLI(t, e, "", "migrate", "up")
	if code != 0 || !strings.Contains(out, "Migrazioni applicate") || strings.Contains(errOut, "ARENILE_SECRET_KEY=") {
		t.Fatalf("migrate up: %d %q %q", code, out, errOut)
	}
	code, out, _ = runCLI(t, e, "", "migrate", "up")
	if code != 0 || !strings.Contains(out, "già aggiornato") {
		t.Fatalf("second up: %q", out)
	}
	if code, _, _ := runCLI(t, e, "", "migrate", "down"); code != 2 {
		t.Fatal("migrate down is not supported")
	}

	code, out, _ = runCLI(t, e, "", "seed", "demo")
	if code != 0 || !strings.Contains(out, "creato") {
		t.Fatalf("seed: %d %q", code, out)
	}
	// The demo cashier for the "Prova la demo" button, with the env to set.
	if !strings.Contains(out, "Utente cassa demo creato: cassa@lido-demo.example, password: ") ||
		!strings.Contains(out, "ARENILE_DEMO_PAIRING_USER=cassa@lido-demo.example e ARENILE_DEMO_PAIRING_ESTABLISHMENT=lido-demo") {
		t.Fatalf("seed demo cashier: %q", out)
	}
	code, out, _ = runCLI(t, e, "", "seed", "demo")
	if code != 0 || !strings.Contains(out, "già presente") || !strings.Contains(out, "Utente cassa demo già presente: cassa@lido-demo.example") {
		t.Fatalf("seed again: %d %q", code, out)
	}

	if code, _, _ := runCLI(t, e, "", "admin", "create", "--email", "a@b.it"); code != 2 {
		t.Fatal("missing flags must be a usage error")
	}
	e.vars["ARENILE_ADMIN_PASSWORD"] = "password-from-env"
	code, out, errOut = runCLI(t, e, "", "admin", "create", "--email", "Owner@Lido-Demo.it", "--name", "Owner", "--org", "Lido Demo")
	if code != 0 || !strings.Contains(out, "owner@lido-demo.it") || strings.Contains(out, "Organizzazione creata") {
		t.Fatalf("admin create: %d %q %q", code, out, errOut)
	}
	code, _, errOut = runCLI(t, e, "", "admin", "create", "--email", "owner@lido-demo.it", "--name", "Owner", "--org", "Lido Demo")
	if code != 1 || !strings.Contains(errOut, "esiste già") {
		t.Fatalf("duplicate admin: %d %q", code, errOut)
	}
	delete(e.vars, "ARENILE_ADMIN_PASSWORD")
	code, out, _ = runCLI(t, e, "piped-password-1\n", "admin", "create", "--email", "second@x.it", "--name", "Second", "--org", "Nuova Srl")
	if code != 0 || !strings.Contains(out, "Organizzazione creata") {
		t.Fatalf("piped password: %d %q", code, out)
	}
	code, _, errOut = runCLI(t, e, "", "admin", "create", "--email", "third@x.it", "--name", "T", "--org", "X", "--password", "short")
	if code != 1 || !strings.Contains(errOut, "password") {
		t.Fatalf("weak password: %d %q", code, errOut)
	}

	backup := filepath.Join(t.TempDir(), "backups", "arenile-backup.db")
	code, out, errOut = runCLI(t, e, "", "backup", "--out", backup)
	if code != 0 || !strings.Contains(out, "Backup completato") {
		t.Fatalf("backup: %d %q %q", code, out, errOut)
	}
	if code, _, _ := runCLI(t, e, "", "backup", "--out", backup); code != 1 {
		t.Fatal("backup must refuse to overwrite")
	}
	notDB := filepath.Join(t.TempDir(), "x.db")
	_ = os.WriteFile(notDB, []byte("hello"), 0o600)
	if code, _, _ := runCLI(t, e, "", "restore", "--in", notDB); code != 1 {
		t.Fatal("restore must validate the file")
	}
	code, out, errOut = runCLI(t, e, "", "restore", "--in", backup)
	if code != 0 || !strings.Contains(out, "Ripristino completato") || !strings.Contains(errOut, "deve essere fermo") {
		t.Fatalf("restore: %d %q %q", code, out, errOut)
	}
	// The restored database still contains the demo data.
	code, out, _ = runCLI(t, e, "", "seed", "demo")
	if code != 0 || !strings.Contains(out, "già presente") {
		t.Fatalf("after restore: %q", out)
	}
}

func TestCLI_PostgresBackupPrintsInstructions(t *testing.T) {
	e := cliEnv{vars: map[string]string{"DATABASE_URL": "postgres://u:p@localhost:1/x", "ARENILE_DATA_DIR": t.TempDir()}}
	code, _, errOut := runCLI(t, e, "", "backup", "--out", filepath.Join(t.TempDir(), "x"))
	if code != 1 || !strings.Contains(errOut, "pg_dump") {
		t.Fatalf("postgres backup: %d %q", code, errOut)
	}
}

func TestCLI_ServeRejectsProductionWithoutConfig(t *testing.T) {
	e := cliEnv{vars: map[string]string{"ARENILE_DATA_DIR": t.TempDir()}}
	code, _, errOut := runCLI(t, e, "", "serve")
	if code != 1 || !strings.Contains(errOut, "ARENILE_BASE_URL") {
		t.Fatalf("serve: %d %q", code, errOut)
	}
}
