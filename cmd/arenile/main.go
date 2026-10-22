// Command arenile is the single binary of Arenile, the open source
// management system for beach establishments.
//
//	arenile serve                 start the server (migrates if ARENILE_AUTO_MIGRATE=true)
//	arenile migrate up|status     apply or inspect database migrations
//	arenile admin create          create an owner user (and its organization)
//	arenile seed demo             load the demo establishment (idempotent)
//	arenile backup --out FILE     consistent SQLite backup (VACUUM INTO)
//	arenile restore --in FILE     restore a SQLite backup (server stopped)
//	arenile version               print the version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/coste360/arenile/internal/config"
)

// version is injected at build time with -ldflags "-X main.version=...".
var version = "dev"

// errUsage signals a command line error (exit code 2).
var errUsage = errors.New("usage error")

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.LookupEnv))
}

type cli struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	lookup config.Lookup
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, lookup config.Lookup) int {
	c := &cli{stdin: stdin, stdout: stdout, stderr: stderr, lookup: lookup}
	if len(args) == 0 {
		c.usage()
		return 2
	}
	var err error
	switch args[0] {
	case "serve":
		err = c.serve(ctx, args[1:])
	case "migrate":
		err = c.migrate(ctx, args[1:])
	case "admin":
		err = c.admin(ctx, args[1:])
	case "seed":
		err = c.seed(ctx, args[1:])
	case "backup":
		err = c.backup(ctx, args[1:])
	case "restore":
		err = c.restore(ctx, args[1:])
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "arenile %s (%s, %s/%s)\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	case "help", "--help", "-h":
		c.usage()
	default:
		fmt.Fprintf(stderr, "arenile: comando sconosciuto %q\n\n", args[0])
		c.usage()
		return 2
	}
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, errUsage):
		fmt.Fprintf(stderr, "arenile: %v\n", err)
		return 2
	default:
		fmt.Fprintf(stderr, "arenile: %v\n", err)
		return 1
	}
}

func (c *cli) usage() {
	fmt.Fprint(c.stderr, `Arenile — gestionale open source per stabilimenti balneari

Uso:
  arenile serve [--dev]                        avvia il server
  arenile migrate up|status                    applica o mostra le migrazioni
  arenile admin create --email E --name N --org O [--password P]
                                               crea un utente owner (password da
                                               ARENILE_ADMIN_PASSWORD o richiesta)
  arenile seed demo                            carica lo stabilimento dimostrativo
  arenile backup --out FILE                    backup consistente (SQLite)
  arenile restore --in FILE                    ripristina un backup (server fermo)
  arenile version                              mostra la versione

La configurazione si legge dalle variabili d'ambiente ARENILE_* e DATABASE_URL.
`)
}

// loadConfig parses the environment, applying overrides, and prints a newly
// generated development key once on stderr (never through the logger).
func (c *cli) loadConfig(overrides map[string]string) (config.Config, error) {
	cfg, err := config.Parse(func(k string) (string, bool) {
		if v, ok := overrides[k]; ok {
			return v, true
		}
		return c.lookup(k)
	})
	if err != nil {
		return config.Config{}, err
	}
	if cfg.GeneratedSecretKey != "" {
		fmt.Fprintf(c.stderr, "\nATTENZIONE (solo sviluppo): generata una nuova ARENILE_SECRET_KEY, salvata in %s/%s.\n"+
			"Per la produzione impostala esplicitamente:\n  ARENILE_SECRET_KEY=%s\n\n", cfg.DataDir, config.DevKeyFile, cfg.GeneratedSecretKey)
	}
	return cfg, nil
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}
