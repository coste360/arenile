package printing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/escpos"
	"github.com/coste360/arenile/internal/printing/templates"
	"github.com/coste360/arenile/internal/store"
)

// Deps are the dependencies of Service.
type Deps struct {
	DB    *store.DB
	Clock clock.Clock
	Audit *audit.Writer
	Log   *slog.Logger
	// Available is true on the desktop edition only: the server is inside
	// the establishment and can reach LAN printers. False = every
	// operation that touches a printer returns ErrUnavailable.
	Available bool
	// Drivers by printers.driver value (default: the network driver).
	Drivers map[string]Driver
	// Resolver resolves host names (default net.DefaultResolver).
	Resolver Resolver
	// Logos fetches the establishment logo (nil = no logo printed).
	Logos LogoLoader
	// Tickets renders daily-sale tickets (internal/app wires the ticketing
	// service; nil = daily_ticket jobs are refused).
	Tickets TicketSource
	// Lease bounds one delivery attempt (default 1 minute).
	Lease time.Duration
	// AllowLoopback admits loopback printer addresses (development and
	// tests: ARENILE_PRINTERS_ALLOW_LOOPBACK). Applied to the default
	// network driver; explicit Drivers carry their own policy.
	AllowLoopback bool
}

// Service configures printers and routes and delivers print jobs.
type Service struct {
	d    Deps
	wake chan struct{}
	// logoFailed remembers, per printer id, when the last logo fetch
	// failed, so an unreachable logo URL does not delay every print.
	logoFailed sync.Map
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Audit == nil {
		d.Audit = audit.NewWriter(d.Clock)
	}
	if d.Drivers == nil {
		d.Drivers = map[string]Driver{DriverNetwork: NetworkDriver{AllowLoopback: d.AllowLoopback}}
	}
	if d.Lease <= 0 {
		d.Lease = time.Minute
	}
	return &Service{d: d, wake: make(chan struct{}, 1)}
}

// Available reports whether ESC/POS printing works on this edition.
func (s *Service) Available() bool { return s.d.Available }

// TicketsAvailable reports whether daily-sale tickets can be printed:
// printing is available (desktop edition) and a TicketSource is wired.
func (s *Service) TicketsAvailable() bool { return s.d.Available && s.d.Tickets != nil }

// policy is the address policy of this instance.
func (s *Service) policy() AddressPolicy { return AddressPolicy{AllowLoopback: s.d.AllowLoopback} }

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

// PrinterInput is the configuration of a printer (PUT replaces it all:
// absent optional fields take their defaults).
type PrinterInput struct {
	Name          string
	Host          string
	Port          int // 0 = 9100
	PaperWidthMM  int // 58 or 80
	CharsPerLine  int // 0 = 32 on 58 mm, 48 on 80 mm
	Codepage      string
	CodepageTable *int
	CutMode       string
	FeedLines     *int // nil = 3
	PrintLogo     bool
	DrawerPin     *int
	Active        *bool // nil = true
}

// normalize validates in and fills the defaults.
func (in *PrinterInput) normalize() error {
	var ve configerr.ValidationError
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		ve.Add("name", "Obbligatorio.")
	} else if utf8.RuneCountInString(in.Name) > 60 {
		ve.Add("name", "Al massimo 60 caratteri.")
	}
	host, err := NormalizeHost(in.Host)
	var hve *configerr.ValidationError
	switch {
	case errors.As(err, &hve):
		ve.Fields = append(ve.Fields, hve.Fields...)
	case err != nil:
		return err
	}
	in.Host = host
	if in.Port == 0 {
		in.Port = store.DefaultPrinterPort
	}
	if in.Port < 1 || in.Port > 65535 {
		ve.Add("port", "Deve essere tra 1 e 65535.")
	}
	if in.PaperWidthMM != 58 && in.PaperWidthMM != 80 {
		ve.Add("paper_width_mm", "Scegli 58 o 80 mm.")
	}
	if in.CharsPerLine == 0 {
		in.CharsPerLine = templates.DefaultCharsPerLine(in.PaperWidthMM)
	}
	if in.CharsPerLine < 24 || in.CharsPerLine > 64 {
		ve.Add("chars_per_line", "Deve essere tra 24 e 64.")
	}
	if in.Codepage == "" {
		in.Codepage = string(escpos.CP858)
	}
	if !escpos.Codepage(in.Codepage).Valid() {
		ve.Add("codepage", "Codepage non supportata.")
	}
	if in.CodepageTable != nil && (*in.CodepageTable < 0 || *in.CodepageTable > 255) {
		ve.Add("codepage_table", "Deve essere tra 0 e 255.")
	}
	switch in.CutMode {
	case "":
		in.CutMode = store.PrinterCutPartial
	case store.PrinterCutFull, store.PrinterCutPartial, store.PrinterCutNone:
	default:
		ve.Add("cut_mode", "Valore non valido.")
	}
	if in.FeedLines == nil {
		three := 3
		in.FeedLines = &three
	}
	if *in.FeedLines < 0 || *in.FeedLines > 10 {
		ve.Add("feed_lines", "Deve essere tra 0 e 10.")
	}
	if in.DrawerPin != nil && *in.DrawerPin != 2 && *in.DrawerPin != 5 {
		ve.Add("drawer_pin", "Scegli il pin 2 o 5.")
	}
	if in.Active == nil {
		t := true
		in.Active = &t
	}
	return ve.OrNil()
}

// ListPrinters returns the printers of the establishment (cash.operate).
func (s *Service) ListPrinters(ctx context.Context, p *auth.Principal, establishmentID string, includeInactive bool) ([]store.Printer, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermCashOperate)
	if err != nil {
		return nil, err
	}
	return store.ListPrinters(ctx, s.d.DB, es, includeInactive)
}

// GetPrinter returns one printer (cash.operate).
func (s *Service) GetPrinter(ctx context.Context, p *auth.Principal, establishmentID, id string) (*store.Printer, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermCashOperate)
	if err != nil {
		return nil, err
	}
	return s.printer(ctx, es, id)
}

func (s *Service) printer(ctx context.Context, es store.EstablishmentScope, id string) (*store.Printer, error) {
	if !validID(id) {
		return nil, ErrNotFound
	}
	pr, err := store.GetPrinter(ctx, s.d.DB, es, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	return pr, err
}

var errNameTaken = fmt.Errorf("%w: a printer with this name already exists", configerr.ErrConflict)

// CreatePrinter adds a printer (establishment.configure, desktop edition:
// ErrUnavailable on the server edition, where nothing could be printed).
// The host must be a private address (ErrAddressNotAllowed).
func (s *Service) CreatePrinter(ctx context.Context, p *auth.Principal, establishmentID string, in PrinterInput) (*store.Printer, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	if !s.d.Available {
		return nil, ErrUnavailable
	}
	if err := in.normalize(); err != nil {
		return nil, err
	}
	if err := CheckHost(ctx, s.d.Resolver, in.Host, s.policy()); err != nil {
		return nil, err
	}
	now := s.now()
	pr := &store.Printer{
		ID: store.NewID(), Driver: DriverNetwork, Status: store.PrinterStatusUnknown, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	apply(pr, in)
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := store.InsertPrinter(ctx, tx, es, pr); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "printer.created",
			EntityType: "printer", EntityID: pr.ID, Diff: printerDiff(pr),
		})
	})
	if errors.Is(err, store.ErrConflict) {
		return nil, errNameTaken
	}
	if err != nil {
		return nil, fmt.Errorf("printing: create printer: %w", err)
	}
	return pr, nil
}

// UpdatePrinter replaces the configuration of a printer
// (establishment.configure, desktop edition). expected is the If-Match
// version (nil = any). Changing paper width, logo flag or host resets the
// cached logo and the reachability status.
func (s *Service) UpdatePrinter(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64, in PrinterInput) (*store.Printer, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, err
	}
	if !s.d.Available {
		return nil, ErrUnavailable
	}
	if err := in.normalize(); err != nil {
		return nil, err
	}
	if err := CheckHost(ctx, s.d.Resolver, in.Host, s.policy()); err != nil {
		return nil, err
	}
	pr, err := s.printer(ctx, es, id)
	if err != nil {
		return nil, err
	}
	if err := configerr.CheckVersion(expected, pr.Version); err != nil {
		return nil, err
	}
	before := printerDiff(pr)
	if in.PaperWidthMM != pr.PaperWidthMM || in.PrintLogo != pr.PrintLogo {
		pr.LogoRaster, pr.LogoSourceSHA256 = nil, nil
	}
	if in.Host != pr.Host || in.Port != pr.Port {
		pr.Status, pr.LastError = store.PrinterStatusUnknown, nil
	}
	apply(pr, in)
	pr.UpdatedAt = s.now()
	current := pr.Version
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := store.UpdatePrinter(ctx, tx, es, pr, current); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "printer.updated",
			EntityType: "printer", EntityID: pr.ID, Diff: map[string]any{"before": before, "after": printerDiff(pr)},
		})
	})
	switch {
	case errors.Is(err, store.ErrConflict):
		return nil, errNameTaken
	case errors.Is(err, store.ErrNotFound):
		return nil, s.lostUpdate(ctx, es, id)
	case err != nil:
		return nil, fmt.Errorf("printing: update printer: %w", err)
	}
	return pr, nil
}

// lostUpdate tells a deleted printer from a concurrent update.
func (s *Service) lostUpdate(ctx context.Context, es store.EstablishmentScope, id string) error {
	cur, err := s.printer(ctx, es, id)
	if err != nil {
		return err
	}
	return &configerr.VersionConflictError{Current: cur.Version}
}

// DeletePrinter removes a printer, its routes, and cancels its queued jobs
// (establishment.configure).
func (s *Service) DeletePrinter(ctx context.Context, p *auth.Principal, establishmentID, id string, expected *int64) error {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return err
	}
	pr, err := s.printer(ctx, es, id)
	if err != nil {
		return err
	}
	if err := configerr.CheckVersion(expected, pr.Version); err != nil {
		return err
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := store.DeletePrinter(ctx, tx, es, id, pr.Version, s.now()); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "printer.deleted",
			EntityType: "printer", EntityID: id, Diff: printerDiff(pr),
		})
	})
	if errors.Is(err, store.ErrNotFound) {
		return s.lostUpdate(ctx, es, id)
	}
	if err != nil {
		return fmt.Errorf("printing: delete printer: %w", err)
	}
	return nil
}

// CheckResult is the outcome of CheckPrinter.
type CheckResult struct {
	Status    string
	Latency   time.Duration
	Error     string
	PaperLow  *bool
	CoverOpen *bool
	CheckedAt time.Time
}

// CheckPrinter probes a printer without printing and records its status
// (cash.operate; desktop edition only).
func (s *Service) CheckPrinter(ctx context.Context, p *auth.Principal, establishmentID, id string) (CheckResult, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermCashOperate)
	if err != nil {
		return CheckResult{}, err
	}
	if !s.d.Available {
		return CheckResult{}, ErrUnavailable
	}
	pr, err := s.printer(ctx, es, id)
	if err != nil {
		return CheckResult{}, err
	}
	return s.check(ctx, pr), nil
}

// check probes pr and stores its health.
func (s *Service) check(ctx context.Context, pr *store.Printer) CheckResult {
	res := CheckResult{CheckedAt: s.now()}
	drv, ok := s.d.Drivers[pr.Driver]
	var st Status
	var err error
	if !ok {
		err = fmt.Errorf("printing: no driver %q", pr.Driver)
	} else {
		st, err = drv.Check(ctx, Target{Host: pr.Host, Port: pr.Port})
	}
	h := store.PrinterHealth{CheckedAt: res.CheckedAt}
	if err != nil {
		res.Status, res.Error = store.PrinterStatusUnreachable, technical(err)
		h.Status, h.LastError = res.Status, &res.Error
	} else {
		res.Status, res.Latency = store.PrinterStatusReachable, st.Latency
		res.PaperLow, res.CoverOpen = &st.PaperLow, &st.CoverOpen
		h.Status, h.Success = res.Status, true
	}
	if err := store.SetPrinterHealth(ctx, s.d.DB, pr.ID, h); err != nil {
		s.d.Log.WarnContext(ctx, "printer health not saved", slog.String("printer_id", pr.ID), slog.String("error", err.Error()))
	}
	pr.Status, pr.LastCheckedAt, pr.LastError = h.Status, &h.CheckedAt, h.LastError
	if h.Success {
		pr.LastSuccessAt = &h.CheckedAt
	}
	return res
}

func apply(pr *store.Printer, in PrinterInput) {
	pr.Name, pr.Host, pr.Port = in.Name, in.Host, in.Port
	pr.PaperWidthMM, pr.CharsPerLine, pr.Codepage, pr.CodepageTable = in.PaperWidthMM, in.CharsPerLine, in.Codepage, in.CodepageTable
	pr.CutMode, pr.FeedLines, pr.PrintLogo, pr.DrawerPin, pr.Active = in.CutMode, *in.FeedLines, in.PrintLogo, in.DrawerPin, *in.Active
}

func printerDiff(pr *store.Printer) map[string]any {
	return map[string]any{
		"name": pr.Name, "host": pr.Host, "port": pr.Port, "paper_width_mm": pr.PaperWidthMM,
		"chars_per_line": pr.CharsPerLine, "codepage": pr.Codepage, "cut_mode": pr.CutMode,
		"feed_lines": pr.FeedLines, "print_logo": pr.PrintLogo, "drawer_pin": pr.DrawerPin, "active": pr.Active,
	}
}

// validID reports whether id is a UUID (every id column is one on
// PostgreSQL: a malformed id is simply not found).
func validID(id string) bool {
	_, err := uuid.Parse(id)
	return err == nil && len(id) == 36
}

// maxErrorLen bounds last_error.
const maxErrorLen = 300

// technical returns the error text for last_error: network class only,
// bounded; payloads are never part of driver errors.
func technical(err error) string {
	msg := err.Error()
	if errors.Is(err, ErrAddressNotAllowed) {
		msg = "address not allowed (not a private network address)"
	}
	if utf8.RuneCountInString(msg) > maxErrorLen {
		msg = string([]rune(msg)[:maxErrorLen])
	}
	return msg
}
