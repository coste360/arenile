package importer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

// errDryRun rolls back the preview transaction.
var errDryRun = errors.New("importer: dry run")

// errRowsRefused rolls back an apply where some row was refused.
var errRowsRefused = errors.New("importer: rows refused")

// outcome of one row.
type outcome int

const (
	outCreated outcome = iota + 1
	outUpdated
	outSkipped
)

// run validates and executes the rows of j in a transaction that is always
// rolled back (preview) and returns the report.
func (s *Service) run(ctx context.Context, p *auth.Principal, ip string, es store.EstablishmentScope, j *store.ImportJob, opts Options) (*Report, error) {
	t, rep, rows, err := s.prepare(j)
	if err != nil {
		return nil, err
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := s.execute(ctx, tx, p, ip, es, j.Kind, t, opts, rows, rep); err != nil {
			return err
		}
		return errDryRun
	})
	if !errors.Is(err, errDryRun) {
		return nil, err
	}
	return rep, nil
}

// runApply executes the rows of j in one transaction and calls commit in
// it when every row succeeded. Invalid or refused rows roll everything back
// and return the report with *InvalidError.
func (s *Service) runApply(ctx context.Context, p *auth.Principal, ip string, es store.EstablishmentScope, j *store.ImportJob, opts Options,
	commit func(ctx context.Context, tx bun.Tx, rep *Report) error,
) (*Report, error) {
	t, rep, rows, err := s.prepare(j)
	if err != nil {
		return nil, err
	}
	if rep.Invalid > 0 {
		return rep, &InvalidError{Code: "invalid_rows", Detail: "Il file contiene righe non valide: correggile e ripeti l'anteprima."}
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := s.execute(ctx, tx, p, ip, es, j.Kind, t, opts, rows, rep); err != nil {
			return err
		}
		if rep.Invalid > 0 {
			return errRowsRefused
		}
		created, updated, skipped := rep.ToCreate, rep.ToUpdate, rep.ToSkip
		rep.Created, rep.Updated, rep.Skipped = &created, &updated, &skipped
		return commit(ctx, tx, rep)
	})
	if errors.Is(err, errRowsRefused) {
		rep.Created, rep.Updated, rep.Skipped = nil, nil, nil
		return rep, &InvalidError{Code: "rows_refused", Detail: "Alcune righe non possono essere importate: nessuna modifica è stata salvata."}
	}
	if err != nil {
		return nil, err
	}
	return rep, nil
}

// prepare decodes and validates the stored content of j.
func (s *Service) prepare(j *store.ImportJob) (*table, *Report, []row, error) {
	if j.Content == nil {
		return nil, nil, nil, ErrContentGone
	}
	t, err := decodeCSV(j.Kind, []byte(*j.Content))
	if err != nil {
		return nil, nil, nil, invalidFrom(err)
	}
	rep := &Report{Rows: len(t.rows), Errors: []Issue{}, Warnings: []Issue{}, Columns: t.recognised()}
	fileWarnings(t, rep)
	rows := validateRows(j.Kind, t, s.now(), rep)
	rep.Valid = len(rows)
	return t, rep, rows, nil
}

// execute writes the rows in tx, each in its own savepoint: a refused row
// is rolled back alone, counted invalid and reported; unexpected errors
// abort the whole run.
func (s *Service) execute(ctx context.Context, tx bun.Tx, p *auth.Principal, ip string, es store.EstablishmentScope, kind string, t *table, opts Options, rows []row, rep *Report) error {
	ex := &executor{
		s: s, tx: tx, p: p, ip: ip, es: es, t: t, opts: opts, now: s.now(),
		customers: s.d.Customers(tx), created: map[string]string{},
	}
	if kind == store.ImportKindSeasonPasses {
		if err := ex.loadCatalog(ctx); err != nil {
			return err
		}
		ex.passes = s.d.Passes(tx)
	}
	for i := range rows {
		r := &rows[i]
		var (
			out      outcome
			warnings []Issue
			issue    *Issue
		)
		err := tx.RunInTx(ctx, nil, func(ctx context.Context, sp bun.Tx) error {
			ex.customers = s.d.Customers(sp)
			if ex.passes != nil {
				ex.passes = s.d.Passes(sp)
			}
			ex.db = sp
			var err error
			if kind == store.ImportKindSeasonPasses {
				out, warnings, issue, err = ex.passRow(ctx, r)
			} else {
				out, warnings, issue, err = ex.customerRow(ctx, r)
			}
			if err == nil && issue != nil {
				return errRowsRefused // roll back this row only
			}
			return err
		})
		if err != nil && !errors.Is(err, errRowsRefused) {
			is, ok := rowIssue(r.line, err)
			if !ok {
				return err
			}
			issue = &is
		}
		if issue != nil {
			ex.pending = nil
			rep.Valid--
			rep.Invalid++
			rep.addError(*issue)
			continue
		}
		ex.commitRow()
		for _, w := range warnings {
			rep.addWarning(w)
		}
		switch out {
		case outCreated:
			rep.ToCreate++
		case outUpdated:
			rep.ToUpdate++
		case outSkipped:
			rep.ToSkip++
		}
	}
	return nil
}

// executor holds the state of one run.
type executor struct {
	s         *Service
	tx        bun.Tx
	db        bun.IDB
	p         *auth.Principal
	ip        string
	es        store.EstablishmentScope
	t         *table
	opts      Options
	now       time.Time
	customers CustomerWriter
	passes    PassWriter
	// created maps the match keys of the customers created by previous
	// rows of a season pass import to their ids (pending: this row).
	created map[string]string
	pending map[string]string
	// Season pass catalog.
	spots map[string][]store.Spot
	packs map[string][]string
	// approvalPacks are the packs with requires_approval (never imported).
	approvalPacks map[string]bool
}

func (ex *executor) commitRow() {
	for k, v := range ex.pending {
		ex.created[k] = v
	}
	ex.pending = nil
}

// customerRow creates, updates or skips the customer of r.
func (ex *executor) customerRow(ctx context.Context, r *row) (outcome, []Issue, *Issue, error) {
	existing, warnings, err := ex.findCustomer(ctx, r)
	if err != nil {
		return 0, nil, nil, err
	}
	if existing == nil {
		in := r.customer
		in.MarketingConsent = ex.newConsent(r)
		if _, err := ex.customers.Create(ctx, ex.p, ex.ip, ex.es.ID(), in, true); err != nil {
			return 0, nil, nil, err
		}
		return outCreated, warnings, nil, nil
	}
	if !ex.opts.UpdateExisting {
		return outSkipped, warnings, nil, nil
	}
	next, err := ex.customers.Update(ctx, ex.p, ex.ip, ex.es.ID(), existing.ID, nil, ex.merge(existing, r))
	if err != nil {
		return 0, nil, nil, err
	}
	if next.Version == existing.Version {
		return outSkipped, warnings, nil, nil
	}
	return outUpdated, warnings, nil, nil
}

// passRow creates the season pass of r (and its customer when missing).
func (ex *executor) passRow(ctx context.Context, r *row) (outcome, []Issue, *Issue, error) {
	refuse := func(col, code, msg string) (outcome, []Issue, *Issue, error) {
		return 0, nil, &Issue{Row: r.line, Column: col, Code: code, Message: msg}, nil
	}
	spots := ex.spots[strings.ToLower(r.spotLabel)]
	switch {
	case len(spots) == 0:
		return refuse(colSpot, codeUnknownSpot, "Posto inesistente o disattivato.")
	case len(spots) > 1:
		return refuse(colSpot, codeAmbiguousSpot, "Più posti attivi hanno questa etichetta.")
	}
	spot := spots[0]
	packID := ex.opts.PackID
	if r.packRef != "" {
		ids := ex.packs[customer.Fold(r.packRef)]
		switch {
		case len(ids) == 0:
			return refuse(colPack, codeUnknownPack, "Pacchetto inesistente o disattivato.")
		case len(ids) > 1:
			return refuse(colPack, codeUnknownPack, "Più pacchetti hanno questo nome: usa il codice.")
		}
		packID = ids[0]
	}
	if packID == "" {
		return refuse(colPack, codeRequired, "Indica il pacchetto nella colonna o nelle opzioni.")
	}
	if ex.approvalPacks[packID] {
		// An imported pass would wait for the approval (rule R2) and expire
		// silently with its hold: refuse the row instead (INV-78).
		return refuse(colPack, codePackNeedsApproval, msgPackNeedsApproval)
	}

	customerID, warnings, err := ex.passCustomer(ctx, r)
	if err != nil {
		return 0, nil, nil, err
	}
	if customerID == "" {
		return refuse(ex.matchColumn(), codeCustomerMissing, "Cliente non trovato e creazione disattivata.")
	}
	exists, err := store.ImportSeasonPassExists(ctx, ex.db, ex.es, customerID, ex.opts.SeasonID, spot.ID)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("importer: %w", err)
	}
	if exists {
		return outSkipped, warnings, nil, nil
	}
	_, err = ex.passes.Create(ctx, ex.p, ex.ip, ex.es.ID(), pass.CreateInput{
		CustomerID: customerID, SeasonID: ex.opts.SeasonID, SpotID: spot.ID, PackID: packID,
		StartsOn: r.startsOn, EndsOn: r.endsOn, Slots: r.slots, NotesInternal: notesOf(r),
	})
	if err != nil {
		return 0, nil, nil, err
	}
	return outCreated, warnings, nil, nil
}

// passCustomer returns the customer of a season pass row: found by the
// match key, created by a previous row, or created now (when allowed).
func (ex *executor) passCustomer(ctx context.Context, r *row) (string, []Issue, error) {
	key := ex.matchKey(r)
	if key != "" {
		if id, ok := ex.created[key]; ok {
			return id, nil, nil
		}
	}
	existing, warnings, err := ex.findCustomer(ctx, r)
	if err != nil {
		return "", nil, err
	}
	if existing != nil {
		return existing.ID, warnings, nil
	}
	if !ex.opts.createMissing() {
		return "", warnings, nil
	}
	in := r.customer
	in.MarketingConsent = ex.newConsent(r)
	c, err := ex.customers.Create(ctx, ex.p, ex.ip, ex.es.ID(), in, true)
	if err != nil {
		return "", nil, err
	}
	if key != "" {
		ex.pending = map[string]string{key: c.ID}
	}
	return c.ID, warnings, nil
}

// findCustomer returns the customer matching r by the match criterion
// (nil when none) and warns about another customer sharing the other key.
func (ex *executor) findCustomer(ctx context.Context, r *row) (*store.Customer, []Issue, error) {
	var warnings []Issue
	if ex.matchKey(r) == "" {
		warnings = append(warnings, Issue{
			Row: r.line, Column: ex.matchColumn(), Code: codeNoMatchKey,
			Message: "Senza questo dato il cliente viene sempre creato.",
		})
	}
	if r.email == "" && r.phone == "" {
		return nil, warnings, nil
	}
	dups, err := store.FindCustomerDuplicates(ctx, ex.db, ex.es, r.email, r.phone, "")
	if err != nil {
		return nil, nil, fmt.Errorf("importer: %w", err)
	}
	var match *store.Customer
	for i := range dups {
		c := &dups[i]
		if ex.opts.matchBy() == MatchByPhone && r.phone != "" && deref2(c.Phone) == r.phone ||
			ex.opts.matchBy() == MatchByEmail && r.email != "" && deref2(c.Email) == r.email {
			match = c
			break
		}
	}
	for i := range dups {
		c := &dups[i]
		if match != nil && c.ID == match.ID {
			continue
		}
		col := colPhone
		if ex.opts.matchBy() == MatchByPhone {
			col = colEmail
		}
		warnings = append(warnings, Issue{
			Row: r.line, Column: col, Code: codePossibleDuplicate,
			Message: "Un altro cliente ha lo stesso dato: verifica dopo l'importazione.",
		})
		break
	}
	return match, warnings, nil
}

func (ex *executor) matchKey(r *row) string {
	if ex.opts.matchBy() == MatchByPhone {
		return r.phone
	}
	return r.email
}

func (ex *executor) matchColumn() string {
	if ex.opts.matchBy() == MatchByPhone {
		return colPhone
	}
	return colEmail
}

// newConsent is the marketing consent of a new customer: the column when
// given, else marketing_consent_default when the column is absent (never
// presumed when the column exists with an empty cell).
func (ex *executor) newConsent(r *row) *store.MarketingConsent {
	given := false
	switch {
	case r.consent != nil:
		given = *r.consent
	case !ex.t.has(colConsent):
		given = ex.opts.MarketingConsentDefault
	}
	if !given {
		return nil
	}
	return ex.consentOf(r)
}

func (ex *executor) consentOf(r *row) *store.MarketingConsent {
	at := r.consentAt
	if at.IsZero() {
		at = ex.now
	}
	tv := r.consentTV
	if tv == "" {
		tv = defaultConsentTV
	}
	return &store.MarketingConsent{GivenAt: at, Source: consentSource, TextVersion: tv}
}

// merge overlays the values of r on the existing customer: empty cells
// keep the current data; the consent column "no" withdraws the consent,
// "sì" sets it only when missing (the original record is kept).
func (ex *executor) merge(c *store.Customer, r *row) customer.Input {
	in := customer.Input{
		FirstName: c.FirstName, LastName: c.LastName, Email: deref2(c.Email), Phone: deref2(c.Phone),
		Locale: c.Locale, Notes: c.Notes, Tags: append([]string{}, c.Tags...), AgreementID: deref2(c.AgreementID),
		MarketingConsent: c.MarketingConsent,
	}
	set := func(col string, dst *string, v string) {
		if r.has[col] {
			*dst = v
		}
	}
	set(colFirstName, &in.FirstName, r.customer.FirstName)
	set(colLastName, &in.LastName, r.customer.LastName)
	set(colEmail, &in.Email, r.customer.Email)
	set(colPhone, &in.Phone, r.customer.Phone)
	set(colLocale, &in.Locale, r.customer.Locale)
	set(colNotes, &in.Notes, r.customer.Notes)
	if r.has[colTags] {
		in.Tags = r.customer.Tags
	}
	if r.consent != nil {
		switch {
		case !*r.consent:
			in.MarketingConsent = nil
		case c.MarketingConsent == nil:
			in.MarketingConsent = ex.consentOf(r)
		}
	}
	return in
}

// loadCatalog indexes the active spots by label and the active packs by
// code and name (any locale), folded.
func (ex *executor) loadCatalog(ctx context.Context) error {
	spots, err := store.ImportActiveSpots(ctx, ex.tx, ex.es)
	if err != nil {
		return fmt.Errorf("importer: %w", err)
	}
	ex.spots = map[string][]store.Spot{}
	for _, sp := range spots {
		k := strings.ToLower(strings.TrimSpace(sp.Label))
		ex.spots[k] = append(ex.spots[k], sp)
	}
	packs, err := store.ImportActivePacks(ctx, ex.tx, ex.es)
	if err != nil {
		return fmt.Errorf("importer: %w", err)
	}
	ex.packs = map[string][]string{}
	ex.approvalPacks = map[string]bool{}
	for _, pk := range packs {
		if pk.RequiresApproval {
			ex.approvalPacks[pk.ID] = true
		}
		keys := map[string]bool{customer.Fold(pk.Code): true}
		for _, n := range pk.Name {
			keys[customer.Fold(n)] = true
		}
		for k := range keys {
			if k != "" {
				ex.packs[k] = append(ex.packs[k], pk.ID)
			}
		}
	}
	return nil
}

func notesOf(r *row) string {
	// Season pass rows keep their notes for the pass (parseRow cleared the
	// customer notes).
	return r.passNotes
}

// rowIssue maps the domain errors of one row to an issue; false means the
// error is not about the row (database down, forbidden...) and aborts.
func rowIssue(line int, err error) (Issue, bool) {
	var (
		ve  *configerr.ValidationError
		ce  *availability.ConflictError
		sue *availability.SpotUnavailableError
		pve *policy.ViolationError
		dup *customer.DuplicateError
	)
	is := func(col, code, msg string) (Issue, bool) {
		return Issue{Row: line, Column: col, Code: code, Message: msg}, true
	}
	switch {
	case errors.As(err, &ve):
		if len(ve.Fields) == 0 {
			return is("", codeInvalid, "Dati non validi.")
		}
		f := ve.Fields[0]
		return is(fieldColumn[strings.TrimPrefix(f.Field, "customer.")], codeInvalid, f.Message)
	case errors.As(err, &ce), errors.As(err, &sue), errors.Is(err, availability.ErrSpotUnavailable),
		errors.Is(err, availability.ErrPoolExhausted), errors.Is(err, availability.ErrQuotaExceeded):
		return is(colSpot, codeSpotUnavailable, "Il posto non è libero in tutte le date richieste.")
	case errors.Is(err, pricing.ErrNoPrice):
		return is(colPack, codeNoPrice, "Nessun prezzo configurato per questo pacchetto e periodo.")
	case errors.As(err, &pve):
		return is("", codePolicy, "La prenotazione non rispetta le regole dello stabilimento.")
	case errors.Is(err, booking.ErrOutsidePeriod):
		return is(colStartsOn, codeOutsideSeason, "Date fuori dalla stagione.")
	case errors.As(err, &dup):
		return is(colEmail, codeConflict, "Esiste già un cliente con gli stessi dati.")
	case errors.Is(err, configerr.ErrValidation):
		return is("", codeInvalid, "Dati non validi.")
	case errors.Is(err, configerr.ErrConflict), errors.Is(err, configerr.ErrNotFound):
		return is("", codeConflict, "La riga non può essere importata.")
	}
	return Issue{}, false
}

func deref2(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
