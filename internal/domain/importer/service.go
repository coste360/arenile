package importer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
)

// Errors of the importer. ErrNotFound and the conflicts wrap the configerr
// kinds (404/409); *InvalidError and ErrAlreadyApplied have their own
// problem types (422 import_invalid, 409 import_already_applied), mapped by
// the handlers.
var (
	ErrNotFound              = fmt.Errorf("%w: import", configerr.ErrNotFound)
	ErrEstablishmentNotFound = fmt.Errorf("%w: establishment", configerr.ErrNotFound)
	// ErrAlreadyApplied: the same file (SHA-256) of the same kind was
	// already applied in this establishment.
	ErrAlreadyApplied = errors.New("importer: file already applied")
	// ErrNotValidated: apply before a preview (or after the options changed).
	ErrNotValidated = fmt.Errorf("%w: esegui prima l'anteprima dell'importazione", configerr.ErrConflict)
	// ErrClosed: the import was applied or discarded.
	ErrClosed = fmt.Errorf("%w: importazione già applicata o scartata", configerr.ErrConflict)
	// ErrContentGone: the uploaded file was cleared (retention).
	ErrContentGone = fmt.Errorf("%w: il file caricato non è più disponibile, caricalo di nuovo", configerr.ErrConflict)
)

// InvalidError is a file that cannot be imported: unreadable, missing
// columns, invalid rows or rows refused while applying. Detail is Italian
// and carries no personal data; the row issues are in the job report.
type InvalidError struct {
	Code   string
	Detail string
}

func (e *InvalidError) Error() string { return "importer: " + e.Code + ": " + e.Detail }

// Retention of the uploaded content (personal data), spec ADR 0019.
const (
	ContentRetention = 7 * 24 * time.Hour
	// JobRetention is the name of the background job clearing old content.
	JobRetention = "imports.retention"
)

// CustomerWriter creates and updates customers (customer.Service bound to
// a transaction with WithDB).
type CustomerWriter interface {
	Create(ctx context.Context, p *auth.Principal, ip, establishmentID string, in customer.Input, force bool) (*store.Customer, error)
	Update(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64, in customer.Input) (*store.Customer, error)
}

// PassWriter creates season passes through the booking service
// (pass.Service bound to a transaction with WithDB).
type PassWriter interface {
	Create(ctx context.Context, p *auth.Principal, ip, establishmentID string, in pass.CreateInput) (*pass.Detail, error)
}

// Deps are the dependencies of the importer. Every field is required
// except Log and Audit.
type Deps struct {
	DB    bun.IDB
	Clock clock.Clock
	Audit *audit.Writer
	Log   *slog.Logger
	// Customers and Passes bind the writers to the import transaction.
	Customers func(db bun.IDB) CustomerWriter
	Passes    func(db bun.IDB) PassWriter
}

// Service implements the CSV import use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Audit == nil && d.Clock != nil {
		d.Audit = audit.NewWriter(d.Clock)
	}
	return &Service{d: d}
}

// List returns a page of the imports of an establishment (newest first).
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID, cursor string, limit int) (store.Page[store.ImportJob], error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return store.Page[store.ImportJob]{}, err
	}
	page, err := store.ListImportJobs(ctx, s.d.DB, es, cursor, limit)
	if err != nil && !errors.Is(err, store.ErrInvalidCursor) {
		err = fmt.Errorf("importer: list: %w", err)
	}
	return page, err
}

// Get returns an import with its report.
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID, id string) (*store.ImportJob, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	return s.load(ctx, s.d.DB, es, id)
}

// Template returns the CSV template of kind (header only).
func (s *Service) Template(ctx context.Context, p *auth.Principal, establishmentID, kind string) ([]byte, error) {
	if _, err := s.authorize(ctx, p, establishmentID); err != nil {
		return nil, err
	}
	if !ValidKind(kind) {
		return nil, configerr.Invalid("kind", "Tipo di importazione non valido.")
	}
	return Template(kind), nil
}

// Create stores an uploaded CSV file (status uploaded): the header is
// checked right away, nothing else is written. The report lists the
// recognised columns and the row count.
func (s *Service) Create(ctx context.Context, p *auth.Principal, establishmentID, kind, filename string, content []byte) (*store.ImportJob, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if !ValidKind(kind) {
		return nil, configerr.Invalid("kind", "Tipo di importazione non valido.")
	}
	sum := sha256.Sum256(content)
	t, err := decodeCSV(kind, content)
	if err != nil {
		return nil, invalidFrom(err)
	}
	rep := &Report{Rows: len(t.rows), Errors: []Issue{}, Warnings: []Issue{}, Columns: t.recognised()}
	fileWarnings(t, rep)
	body := string(t.text)
	now := s.now()
	j := &store.ImportJob{
		ID: store.NewID(), Kind: kind, Status: store.ImportStatusUploaded,
		Filename: cleanFilename(filename), SizeBytes: int64(len(content)), SHA256: hex.EncodeToString(sum[:]),
		Content: &body, Options: map[string]any{}, Report: toMap(rep), Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if p != nil && p.UserID != "" {
		uid := p.UserID
		j.CreatedByUserID = &uid
	}
	if err := store.InsertImportJob(ctx, s.d.DB, es, j); err != nil {
		return nil, fmt.Errorf("importer: create: %w", err)
	}
	return j, nil
}

// Discard clears the uploaded file of an import not applied yet.
func (s *Service) Discard(ctx context.Context, p *auth.Principal, ip, establishmentID, id string) error {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return err
	}
	return store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		j, err := s.load(ctx, tx, es, id)
		if err != nil {
			return err
		}
		switch j.Status {
		case store.ImportStatusDiscarded:
			return nil
		case store.ImportStatusApplied:
			return ErrClosed
		}
		next := *j
		next.Status, next.Content = store.ImportStatusDiscarded, nil
		if err := s.save(ctx, tx, es, &next, j.Version); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: "import.discarded",
			EntityType: "import_job", EntityID: j.ID, Diff: map[string]any{"kind": j.Kind},
		})
	})
}

// Preview validates every row with opts and computes what apply would
// create, update or skip by running the import in a transaction that is
// always rolled back (same code path as apply: availability, pricing and
// duplicates are checked for real). The job becomes validated.
func (s *Service) Preview(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64, opts Options) (*store.ImportJob, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	j, err := s.load(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, err
	}
	if err := configerr.CheckVersion(ifMatch, j.Version); err != nil {
		return nil, err
	}
	switch j.Status {
	case store.ImportStatusApplied, store.ImportStatusDiscarded:
		return nil, ErrClosed
	}
	if j.Content == nil {
		return nil, ErrContentGone
	}
	if err := s.checkOptions(ctx, s.d.DB, es, j.Kind, &opts); err != nil {
		return nil, err
	}
	rep, err := s.run(ctx, p, ip, es, j, opts)
	if err != nil {
		return nil, err
	}
	next := *j
	next.Status, next.Options, next.Report = store.ImportStatusValidated, toMap(opts), toMap(rep)
	if err := s.save(ctx, s.d.DB, es, &next, j.Version); err != nil {
		return nil, err
	}
	return &next, nil
}

// Apply imports a validated job in one transaction, all or nothing. A
// second apply of an applied job returns it unchanged; the same file
// already applied by another job is ErrAlreadyApplied. When some row is
// refused nothing is written, the job becomes failed with the report and
// the error is *InvalidError.
func (s *Service) Apply(ctx context.Context, p *auth.Principal, ip, establishmentID, id string, ifMatch *int64) (*store.ImportJob, error) {
	es, err := s.authorize(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	j, err := s.load(ctx, s.d.DB, es, id)
	if err != nil {
		return nil, err
	}
	if j.Status == store.ImportStatusApplied {
		return j, nil
	}
	if err := configerr.CheckVersion(ifMatch, j.Version); err != nil {
		return nil, err
	}
	switch {
	case j.Status == store.ImportStatusDiscarded:
		return nil, ErrClosed
	case j.Status != store.ImportStatusValidated:
		return nil, ErrNotValidated
	case j.Content == nil:
		return nil, ErrContentGone
	}
	other, err := store.AppliedImportJobID(ctx, s.d.DB, es, j.Kind, j.SHA256)
	if err != nil {
		return nil, fmt.Errorf("importer: apply: %w", err)
	}
	if other != "" {
		return nil, ErrAlreadyApplied
	}
	var opts Options
	fromMap(j.Options, &opts)
	if err := s.checkOptions(ctx, s.d.DB, es, j.Kind, &opts); err != nil {
		return nil, err
	}
	var out *store.ImportJob
	rep, err := s.runApply(ctx, p, ip, es, j, opts, func(ctx context.Context, tx bun.Tx, rep *Report) error {
		now := s.now()
		next := *j
		next.Status, next.Content, next.Report, next.AppliedAt = store.ImportStatusApplied, nil, toMap(rep), &now
		if err := s.save(ctx, tx, es, &next, j.Version); err != nil {
			return err
		}
		out = &next
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: "import.applied",
			EntityType: "import_job", EntityID: j.ID, Diff: map[string]any{
				"kind": j.Kind, "rows": rep.Rows, "created": deref(rep.Created),
				"updated": deref(rep.Updated), "skipped": deref(rep.Skipped),
			},
		})
	})
	var inv *InvalidError
	if errors.As(err, &inv) && rep != nil {
		// Nothing was written: record the failure and its report.
		next := *j
		next.Status, next.Report = store.ImportStatusFailed, toMap(rep)
		if serr := s.save(ctx, s.d.DB, es, &next, j.Version); serr != nil {
			s.d.Log.WarnContext(ctx, "import: cannot record the failed apply", slog.String("import_id", j.ID), slog.String("error", serr.Error()))
		}
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Jobs returns the retention job clearing the uploaded files older than
// ContentRetention.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{Name: JobRetention, Interval: time.Hour, RunOnStart: true, Timeout: time.Minute, Run: func(ctx context.Context) error {
		_, err := s.ClearExpired(ctx)
		return err
	}}}
}

// ClearExpired clears the content of the imports older than ContentRetention.
func (s *Service) ClearExpired(ctx context.Context) (int64, error) {
	now := s.now()
	n, err := store.ClearExpiredImportContent(ctx, s.d.DB, now.Add(-ContentRetention), now)
	if err != nil {
		return 0, fmt.Errorf("importer: retention: %w", err)
	}
	return n, nil
}

// ---- helpers -------------------------------------------------------------------

// authorize resolves the establishment and checks establishment.configure.
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string) (store.EstablishmentScope, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermEstablishmentConfigure)
	if errors.Is(err, store.ErrNotFound) {
		return es, ErrEstablishmentNotFound
	}
	return es, err
}

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

func (s *Service) load(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id string) (*store.ImportJob, error) {
	j, err := store.GetImportJob(ctx, db, es, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("importer: %w", err)
	}
	return j, nil
}

// save writes next over version old (optimistic): a lost race is a version
// conflict, the unique index of applied files ErrAlreadyApplied.
func (s *Service) save(ctx context.Context, db bun.IDB, es store.EstablishmentScope, next *store.ImportJob, old int64) error {
	next.Version = old + 1
	next.UpdatedAt = s.now()
	ok, err := store.UpdateImportJob(ctx, db, es, next, old)
	if errors.Is(err, store.ErrConflict) {
		return ErrAlreadyApplied
	}
	if err != nil {
		return fmt.Errorf("importer: %w", err)
	}
	if !ok {
		cur, err := s.load(ctx, db, es, next.ID)
		if err != nil {
			return err
		}
		return &configerr.VersionConflictError{Current: cur.Version}
	}
	return nil
}

// checkOptions validates opts for kind (references in es).
func (s *Service) checkOptions(ctx context.Context, db bun.IDB, es store.EstablishmentScope, kind string, opts *Options) error {
	ve := &configerr.ValidationError{}
	switch opts.matchBy() {
	case MatchByEmail, MatchByPhone:
	case MatchByExternalRef:
		ve.Add("match_by", "Il riconoscimento per codice esterno non è disponibile: usa email o telefono.")
	default:
		ve.Add("match_by", "Criterio non valido: email o phone.")
	}
	if kind == store.ImportKindSeasonPasses {
		if opts.SeasonID == "" {
			ve.Add("season_id", "Scegli la stagione degli abbonamenti.")
		} else if _, err := store.GetSeason(ctx, db, es, opts.SeasonID); errors.Is(err, store.ErrNotFound) {
			ve.Add("season_id", "Stagione inesistente in questo stabilimento.")
		} else if err != nil {
			return fmt.Errorf("importer: %w", err)
		}
		if opts.PackID != "" {
			if _, err := store.GetPack(ctx, db, es, opts.PackID); errors.Is(err, store.ErrNotFound) {
				ve.Add("pack_id", "Pacchetto inesistente in questo stabilimento.")
			} else if err != nil {
				return fmt.Errorf("importer: %w", err)
			}
		}
	} else {
		opts.SeasonID, opts.PackID, opts.CreateMissingCustomers = "", "", nil
	}
	return ve.OrNil()
}

// invalidFrom maps a decode error to *InvalidError.
func invalidFrom(err error) error {
	var de *decodeError
	if errors.As(err, &de) {
		return &InvalidError{Code: de.code, Detail: de.message}
	}
	return err
}

// fileWarnings adds the file-level warnings (encoding, unknown columns).
func fileWarnings(t *table, rep *Report) {
	if t.converted {
		rep.addWarning(Issue{Row: 1, Code: codeConvertedEncoding, Message: "Il file non era in UTF-8: letto come Windows-1252."})
	}
	for _, u := range t.unknown {
		rep.addWarning(Issue{Row: 1, Column: u, Code: codeUnknownColumn, Message: "Colonna ignorata."})
	}
}

// cleanFilename keeps a short printable label of the file name.
func cleanFilename(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "import.csv"
	}
	return truncate(s, 200)
}

// toMap converts a JSON document (options, report) to the stored map.
func toMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}

// fromMap decodes a stored map into v (zero value on malformed data).
func fromMap(m map[string]any, v any) {
	if m == nil {
		return
	}
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, v)
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
