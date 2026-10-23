package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"

	"github.com/coste360/arenile/internal/app"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/config"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/setup"
	"github.com/coste360/arenile/internal/store"
)

// EnvAdminPassword provides the password to `admin create` non-interactively.
const EnvAdminPassword = config.EnvAdminPassword

func (c *cli) serve(ctx context.Context, args []string) error {
	fs := newFlagSet("serve", c.stderr)
	dev := fs.Bool("dev", false, "modalità sviluppo (equivale a ARENILE_DEV=true)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	overrides := map[string]string{}
	if *dev {
		overrides[config.EnvDev] = "true"
	}
	cfg, err := c.loadConfig(overrides)
	if err != nil {
		return err
	}
	a, err := app.New(cfg, app.WithVersion(version), app.WithLogger(app.NewLogger(c.stderr, cfg.LogLevel)))
	if err != nil {
		return err
	}
	return a.Run(ctx)
}

// openDB opens the database and, if ARENILE_AUTO_MIGRATE is true, applies
// pending migrations; otherwise it refuses to work on a stale schema.
func (c *cli) openDB(ctx context.Context, cfg config.Config, migrate bool) (*store.DB, error) {
	db, err := app.OpenDB(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if !migrate {
		return db, nil
	}
	m, err := db.Migrator()
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if cfg.AutoMigrate {
		if _, err := m.Up(ctx); err != nil {
			_ = db.Close()
			return nil, err
		}
		return db, nil
	}
	aligned, err := m.Aligned(ctx)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if !aligned {
		_ = db.Close()
		return nil, errors.New("il database ha migrazioni in sospeso: esegui `arenile migrate up`")
	}
	return db, nil
}

func (c *cli) migrate(ctx context.Context, args []string) error {
	if len(args) != 1 || (args[0] != "up" && args[0] != "status") {
		return fmt.Errorf("%w: arenile migrate up|status", errUsage)
	}
	cfg, err := c.loadConfig(nil)
	if err != nil {
		return err
	}
	db, err := c.openDB(ctx, cfg, false)
	if err != nil {
		return err
	}
	defer db.Close()
	m, err := db.Migrator()
	if err != nil {
		return err
	}
	if args[0] == "up" {
		applied, err := m.Up(ctx)
		if err != nil {
			return err
		}
		if len(applied) == 0 {
			fmt.Fprintln(c.stdout, "Database già aggiornato.")
			return nil
		}
		fmt.Fprintf(c.stdout, "Migrazioni applicate: %v\n", applied)
		return nil
	}
	st, err := m.Status(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "VERSIONE\tSTATO\tAPPLICATA\tFILE\n")
	for _, s := range st {
		state := "in sospeso"
		if s.Applied {
			state = "applicata"
		}
		fmt.Fprintf(tw, "%05d\t%s\t%s\t%s\n", s.Version, state, s.AppliedAt, s.Name)
	}
	return tw.Flush()
}

func (c *cli) admin(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "create" {
		return fmt.Errorf("%w: arenile admin create --email E --name N --org O", errUsage)
	}
	fs := newFlagSet("admin create", c.stderr)
	email := fs.String("email", "", "email dell'utente (obbligatoria)")
	name := fs.String("name", "", "nome visualizzato (obbligatorio)")
	org := fs.String("org", "", "nome dell'organizzazione; creata se non esiste (obbligatoria)")
	password := fs.String("password", "", "password (sconsigliato: preferisci "+EnvAdminPassword+" o il prompt)")
	locale := fs.String("locale", "it", "lingua dell'utente")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *email == "" || *name == "" || *org == "" {
		fs.Usage()
		return fmt.Errorf("%w: --email, --name e --org sono obbligatori", errUsage)
	}
	pw, err := c.adminPassword(*password)
	if err != nil {
		return err
	}
	if err := auth.ValidatePassword(pw); err != nil {
		return err
	}
	cfg, err := c.loadConfig(nil)
	if err != nil {
		return err
	}
	db, err := c.openDB(ctx, cfg, true)
	if err != nil {
		return err
	}
	defer db.Close()
	var keys *crypto.KeyRing
	if len(cfg.SecretKeys) > 0 {
		if keys, err = crypto.NewKeyRing(cfg.SecretKeys...); err != nil {
			return err
		}
	}
	log := app.NewLogger(c.stderr, cfg.LogLevel)
	svc, err := auth.NewService(db, clock.System{}, keys, audit.NewWriter(clock.System{}), log, auth.Options{})
	if err != nil {
		return err
	}
	res, err := svc.CreateOwner(ctx, auth.CreateOwnerInput{Email: *email, Name: *name, Password: pw, OrganizationName: *org, Locale: *locale})
	if err != nil {
		if errors.Is(err, auth.ErrUserExists) {
			return errors.New("esiste già un utente con questa email")
		}
		return err
	}
	if res.OrganizationCreated {
		fmt.Fprintf(c.stdout, "Organizzazione creata: %s (%s)\n", res.Organization.Name, res.Organization.ID)
	}
	fmt.Fprintf(c.stdout, "Utente owner creato: %s (%s)\nAccedi al pannello su %s/admin/\n", res.User.Email, res.User.ID, cfg.EffectiveBaseURL())
	return nil
}

func (c *cli) adminPassword(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if v, ok := c.lookup(EnvAdminPassword); ok && v != "" {
		return v, nil
	}
	f, ok := c.stdin.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		// Non-interactive: read one line from stdin (e.g. piped secret).
		line, err := bufio.NewReader(c.stdin).ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if err != nil && !errors.Is(err, io.EOF) {
				return "", err
			}
			return "", fmt.Errorf("%w: password mancante: usa %s, --password o un terminale interattivo", errUsage, EnvAdminPassword)
		}
		return line, nil
	}
	fmt.Fprint(c.stderr, "Password: ")
	p1, err := term.ReadPassword(int(f.Fd()))
	fmt.Fprintln(c.stderr)
	if err != nil {
		return "", fmt.Errorf("lettura password: %w", err)
	}
	fmt.Fprint(c.stderr, "Ripeti la password: ")
	p2, err := term.ReadPassword(int(f.Fd()))
	fmt.Fprintln(c.stderr)
	if err != nil {
		return "", fmt.Errorf("lettura password: %w", err)
	}
	if !bytes.Equal(p1, p2) {
		return "", errors.New("le password non coincidono")
	}
	return string(p1), nil
}

func (c *cli) seed(ctx context.Context, args []string) error {
	if len(args) != 1 || args[0] != "demo" {
		return fmt.Errorf("%w: arenile seed demo", errUsage)
	}
	cfg, err := c.loadConfig(nil)
	if err != nil {
		return err
	}
	db, err := c.openDB(ctx, cfg, true)
	if err != nil {
		return err
	}
	defer db.Close()
	clk := clock.System{}
	res, err := app.SeedDemo(ctx, db, clk)
	if err != nil {
		return err
	}
	if res.Created {
		fmt.Fprintf(c.stdout, "Stabilimento dimostrativo creato: %s (%s)\n", app.DemoSlug, res.EstablishmentID)
	} else {
		fmt.Fprintf(c.stdout, "Stabilimento dimostrativo già presente: %s (%s)\n", app.DemoSlug, res.EstablishmentID)
	}
	log := app.NewLogger(c.stderr, cfg.LogLevel)
	aw := audit.NewWriter(clk)
	setupSvc := setup.NewService(setup.Deps{DB: db, Clock: clk, Audit: aw, Outbox: event.NewOutbox(clk), Log: log})
	demo, err := setupSvc.SeedDemo(ctx, res.EstablishmentID)
	if err != nil {
		return err
	}
	switch {
	case demo.Applied:
		fmt.Fprintf(c.stdout, "Configurazione dimostrativa creata: stagione %d, %d postazioni, %d regole di prezzo\n",
			demo.SeasonYear, demo.SpotsCreated, demo.RulesCreated)
	case demo.MapsUpgraded > 0:
		fmt.Fprintf(c.stdout, "Configurazione dimostrativa già presente: %d mappe aggiornate con le illustrazioni del kit "+
			"(%d postazioni illustrate, %d elementi del modello pre-kit rimossi, %d elementi del kit aggiunti)\n",
			demo.MapsUpgraded, demo.SpotsIllustrated, demo.DecorationsRemoved, demo.DecorationsAdded)
	case demo.MapsReoriented > 0:
		fmt.Fprintf(c.stdout, "Configurazione dimostrativa già presente: %d mappe con il mare spostato in alto\n", demo.MapsReoriented)
	default:
		fmt.Fprintln(c.stdout, "Configurazione dimostrativa già presente: nessuna modifica")
	}
	if err := c.seedDemoOwner(ctx, cfg, db, clk, aw, log); err != nil {
		return err
	}
	if err := c.seedDemoOperations(ctx, cfg, log); err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "Pagina pubblica: %s/b/%s\nPannello: %s/admin/\n", cfg.EffectiveBaseURL(), app.DemoSlug, cfg.EffectiveBaseURL())
	return nil
}

// seedDemoOperations adds the M3 operational demo data (customers, season
// passes, bookings, payments, cash sessions) through the fully wired
// services, acting as the demo owner. Idempotent.
func (c *cli) seedDemoOperations(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	a, err := app.New(cfg, app.WithLogger(log), app.WithoutSignalHandling())
	if err != nil {
		return err
	}
	defer a.Close()
	ops, err := a.SeedDemoOperations(ctx, DemoOwnerEmail)
	if err != nil {
		return err
	}
	if ops.Applied {
		fmt.Fprintf(c.stdout, "Dati operativi dimostrativi creati: %d clienti, %d abbonamenti, %d prenotazioni, %d pagamenti\n",
			ops.Customers, ops.SeasonPasses, ops.Bookings, ops.Payments)
	} else {
		fmt.Fprintln(c.stdout, "Dati operativi dimostrativi già presenti: nessuna modifica")
	}
	return nil
}

// DemoOwnerEmail is the owner account created by `seed demo`.
const DemoOwnerEmail = "demo@lido-demo.example"

// seedDemoOwner creates the demo owner of the demo organization when missing.
// The password comes from ARENILE_ADMIN_PASSWORD, otherwise it is random and
// printed once on stdout.
func (c *cli) seedDemoOwner(ctx context.Context, cfg config.Config, db *store.DB, clk clock.Clock, aw *audit.Writer, log *slog.Logger) error {
	if _, err := store.GetUserByEmail(ctx, db, DemoOwnerEmail); err == nil {
		fmt.Fprintf(c.stdout, "Utente demo già presente: %s\n", DemoOwnerEmail)
		return nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	pw, generated := "", false
	if v, ok := c.lookup(EnvAdminPassword); ok && v != "" {
		pw = v
	} else {
		b := make([]byte, 18)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		pw, generated = base64.RawURLEncoding.EncodeToString(b), true
	}
	var keys *crypto.KeyRing
	if len(cfg.SecretKeys) > 0 {
		var err error
		if keys, err = crypto.NewKeyRing(cfg.SecretKeys...); err != nil {
			return err
		}
	}
	svc, err := auth.NewService(db, clk, keys, aw, log, auth.Options{})
	if err != nil {
		return err
	}
	if _, err := svc.CreateOwner(ctx, auth.CreateOwnerInput{
		Email: DemoOwnerEmail, Name: "Gestore Demo", Password: pw, OrganizationName: app.DemoOrganizationName, Source: "seed",
	}); err != nil {
		if errors.Is(err, auth.ErrUserExists) {
			return nil
		}
		return err
	}
	if generated {
		fmt.Fprintf(c.stdout, "Utente demo creato: %s, password: %s (mostrata una sola volta)\n", DemoOwnerEmail, pw)
	} else {
		fmt.Fprintf(c.stdout, "Utente demo creato: %s (password da %s)\n", DemoOwnerEmail, EnvAdminPassword)
	}
	return nil
}

func (c *cli) backup(ctx context.Context, args []string) error {
	fs := newFlagSet("backup", c.stderr)
	out := fs.String("out", "", "file di destinazione (non deve esistere)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("%w: arenile backup --out FILE", errUsage)
	}
	cfg, err := c.loadConfig(nil)
	if err != nil {
		return err
	}
	if cfg.UsesPostgres() {
		fmt.Fprintln(c.stderr, "Con PostgreSQL usa gli strumenti del database, ad esempio:\n  pg_dump --format=custom --file=arenile.dump \"$DATABASE_URL\"")
		return errors.New("backup integrato disponibile solo con SQLite")
	}
	abs, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err == nil {
		return fmt.Errorf("il file %s esiste già", abs)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return err
	}
	db, err := c.openDB(ctx, cfg, false)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", abs); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if err := os.Chmod(abs, 0o600); err != nil {
		return err
	}
	st, _ := os.Stat(abs)
	fmt.Fprintf(c.stdout, "Backup completato: %s (%d byte)\n", abs, st.Size())
	return nil
}

var sqliteHeader = []byte("SQLite format 3\x00")

func (c *cli) restore(ctx context.Context, args []string) error {
	fs := newFlagSet("restore", c.stderr)
	in := fs.String("in", "", "file di backup da ripristinare")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" {
		return fmt.Errorf("%w: arenile restore --in FILE", errUsage)
	}
	cfg, err := c.loadConfig(nil)
	if err != nil {
		return err
	}
	if cfg.UsesPostgres() {
		fmt.Fprintln(c.stderr, "Con PostgreSQL usa gli strumenti del database, ad esempio:\n  pg_restore --clean --dbname=\"$DATABASE_URL\" arenile.dump")
		return errors.New("ripristino integrato disponibile solo con SQLite")
	}
	if err := validateSQLiteBackup(ctx, *in); err != nil {
		return err
	}
	fmt.Fprintln(c.stderr, "ATTENZIONE: il server Arenile deve essere fermo durante il ripristino.")
	dst := cfg.SQLitePath()
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		prev := fmt.Sprintf("%s.pre-restore-%s", dst, time.Now().UTC().Format("20060102T150405Z"))
		if err := os.Rename(dst, prev); err != nil {
			return fmt.Errorf("salvataggio del database corrente: %w", err)
		}
		fmt.Fprintf(c.stdout, "Database precedente salvato in %s\n", prev)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(dst + suffix)
	}
	if err := copyFile(*in, dst); err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "Ripristino completato in %s\n", dst)
	return nil
}

func validateSQLiteBackup(ctx context.Context, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	head := make([]byte, len(sqliteHeader))
	_, err = io.ReadFull(f, head)
	_ = f.Close()
	if err != nil || !bytes.Equal(head, sqliteHeader) {
		return fmt.Errorf("%s non è un database SQLite", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	sqldb, err := sql.Open("sqlite", "file:"+abs+"?mode=ro")
	if err != nil {
		return err
	}
	defer sqldb.Close()
	var res string
	if err := sqldb.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&res); err != nil {
		return fmt.Errorf("verifica di integrità fallita: %w", err)
	}
	if res != "ok" {
		return fmt.Errorf("verifica di integrità fallita: %s", res)
	}
	var n int
	if err := sqldb.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name IN ('goose_db_version', 'establishments')").Scan(&n); err != nil || n != 2 {
		return errors.New("il file non sembra un backup di Arenile")
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
