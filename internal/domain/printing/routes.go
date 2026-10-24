package printing

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"strconv"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Document kinds (api PrintDocumentKind).
var documentKinds = map[string]bool{
	store.PrintDocDailyTicket: true, store.PrintDocCashZ: true,
	store.PrintDocPaymentReceipt: true, store.PrintDocTestPage: true,
}

// ValidDocumentKind reports whether k is a document kind.
func ValidDocumentKind(k string) bool { return documentKinds[k] }

// Route is one routing rule: RegisterID nil = default rule of the kind.
type Route struct {
	DocumentKind string
	RegisterID   *string
	PrinterID    string
	Copies       int // 0 = 1
	KickDrawer   bool
}

// RouteSet is the whole routing table with its version (ETag): a hash of
// the content, so any change by anyone gives a new version.
type RouteSet struct {
	Routes  []store.PrinterRoute
	Version int64
}

// GetRoutes returns the routing table (cash.operate).
func (s *Service) GetRoutes(ctx context.Context, p *auth.Principal, establishmentID string) (RouteSet, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermCashOperate)
	if err != nil {
		return RouteSet{}, err
	}
	return s.routes(ctx, s.d.DB, es)
}

func (s *Service) routes(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (RouteSet, error) {
	rs, err := store.ListPrinterRoutes(ctx, db, es)
	if err != nil {
		return RouteSet{}, err
	}
	return RouteSet{Routes: rs, Version: routesVersion(rs)}, nil
}

// routesVersion hashes the content of the set to a positive version.
func routesVersion(rs []store.PrinterRoute) int64 {
	h := fnv.New64a()
	for _, r := range rs {
		reg := ""
		if r.RegisterID != nil {
			reg = *r.RegisterID
		}
		_, _ = fmt.Fprintf(h, "%s|%s|%s|%d|%t;", r.DocumentKind, reg, r.PrinterID, r.Copies, r.KickDrawer)
	}
	v := int64(h.Sum64() & math.MaxInt64)
	if v == 0 {
		v = 1
	}
	return v
}

// UpdateRoutes replaces the routing table (establishment.configure).
// expected is the If-Match version (nil = any). At most one rule per
// (kind, register); printers and registers must belong to the
// establishment (ErrNotFound otherwise).
func (s *Service) UpdateRoutes(ctx context.Context, p *auth.Principal, establishmentID string, expected *int64, routes []Route) (RouteSet, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return RouteSet{}, err
	}
	var ve configerr.ValidationError
	seen := map[string]bool{}
	for i, r := range routes {
		f := "items[" + strconv.Itoa(i) + "]"
		if !documentKinds[r.DocumentKind] {
			ve.Add(f+".document_kind", "Tipo di documento non valido.")
		}
		if r.Copies == 0 {
			routes[i].Copies = 1
		} else if r.Copies < 1 || r.Copies > 3 {
			ve.Add(f+".copies", "Da 1 a 3 copie.")
		}
		if !validID(r.PrinterID) {
			ve.Add(f+".printer_id", "Identificativo non valido.")
		}
		if r.RegisterID != nil && !validID(*r.RegisterID) {
			ve.Add(f+".register_id", "Identificativo non valido.")
		}
		key := r.DocumentKind + "|"
		if r.RegisterID != nil {
			key += *r.RegisterID
		}
		if seen[key] {
			ve.Add(f, "Regola duplicata per questo documento e questa cassa.")
		}
		seen[key] = true
	}
	if err := ve.OrNil(); err != nil {
		return RouteSet{}, err
	}
	now := s.now()
	var out RouteSet
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		cur, err := s.routes(ctx, tx, es)
		if err != nil {
			return err
		}
		if err := configerr.CheckVersion(expected, cur.Version); err != nil {
			return err
		}
		rows := make([]store.PrinterRoute, len(routes))
		checkedPrinters, checkedRegisters := map[string]bool{}, map[string]bool{}
		for i, r := range routes {
			if !checkedPrinters[r.PrinterID] {
				if _, err := store.GetPrinter(ctx, tx, es, r.PrinterID); err != nil {
					if errors.Is(err, store.ErrNotFound) {
						return ErrNotFound
					}
					return err
				}
				checkedPrinters[r.PrinterID] = true
			}
			if r.RegisterID != nil && !checkedRegisters[*r.RegisterID] {
				ok, err := store.CashRegisterExists(ctx, tx, es, *r.RegisterID)
				if err != nil {
					return err
				}
				if !ok {
					return ErrNotFound
				}
				checkedRegisters[*r.RegisterID] = true
			}
			rows[i] = store.PrinterRoute{
				ID: store.NewID(), DocumentKind: r.DocumentKind, RegisterID: r.RegisterID, PrinterID: r.PrinterID,
				Copies: routes[i].Copies, KickDrawer: r.KickDrawer, CreatedAt: now, UpdatedAt: now,
			}
		}
		if err := store.ReplacePrinterRoutes(ctx, tx, es, rows); err != nil {
			return err
		}
		if out, err = s.routes(ctx, tx, es); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "print_routes.updated",
			EntityType: "establishment", EntityID: es.ID(), Diff: map[string]any{"routes": len(rows)},
		})
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			return RouteSet{}, configerr.Invalid("items", "Regola duplicata per questo documento e questa cassa.")
		}
		return RouteSet{}, err
	}
	return out, nil
}

// Unready reasons (api PrintRouteUnreadyReason).
const (
	ReasonServerEdition      = "server_edition"
	ReasonNoRoute            = "no_route"
	ReasonPrinterInactive    = "printer_inactive"
	ReasonPrinterUnreachable = "printer_unreachable"
)

// Resolution tells a print button whether to use ESC/POS or the browser.
type Resolution struct {
	DocumentKind string
	Ready        bool
	Reason       string
	Printer      *store.Printer
	Copies       int
	KickDrawer   bool
}

// Resolve probes the printer again when its status is older than these:
// an unreachable printer may be back on, a reachable one may be off.
const (
	recheckUnreachable = 10 * time.Second
	recheckReachable   = time.Minute
	// resolveProbeTimeout bounds the probe so a print button never waits long.
	resolveProbeTimeout = 1500 * time.Millisecond
)

// ResolveRoute finds the printer of a document (rule of the register, else
// default rule) and whether it is ready (cash.operate).
func (s *Service) ResolveRoute(ctx context.Context, p *auth.Principal, establishmentID, kind string, registerID *string) (Resolution, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermCashOperate)
	if err != nil {
		return Resolution{}, err
	}
	if !documentKinds[kind] {
		return Resolution{}, configerr.Invalid("document_kind", "Tipo di documento non valido.")
	}
	if registerID != nil && !validID(*registerID) {
		return Resolution{}, configerr.Invalid("register_id", "Identificativo non valido.")
	}
	res := Resolution{DocumentKind: kind, Copies: 1}
	route, err := s.findRoute(ctx, es, kind, registerID)
	if err != nil && !errors.Is(err, ErrRouteMissing) {
		return Resolution{}, err
	}
	if route != nil {
		res.Copies, res.KickDrawer = route.Copies, route.KickDrawer
		pr, err := s.printer(ctx, es, route.PrinterID)
		if err != nil {
			return Resolution{}, err
		}
		res.Printer = pr
	}
	switch {
	case !s.d.Available:
		res.Reason = ReasonServerEdition
	case res.Printer == nil:
		res.Reason = ReasonNoRoute
	case !res.Printer.Active:
		res.Reason = ReasonPrinterInactive
	default:
		pr := res.Printer
		after := recheckUnreachable
		if pr.Status == store.PrinterStatusReachable {
			after = recheckReachable
		}
		if pr.LastCheckedAt == nil || s.now().Sub(*pr.LastCheckedAt) >= after {
			pctx, cancel := context.WithTimeout(ctx, resolveProbeTimeout)
			s.check(pctx, pr)
			cancel()
		}
		if pr.Status == store.PrinterStatusUnreachable {
			res.Reason = ReasonPrinterUnreachable
		} else {
			res.Ready = true
		}
	}
	return res, nil
}

// findRoute returns the rule of the register, else the default rule.
func (s *Service) findRoute(ctx context.Context, es store.EstablishmentScope, kind string, registerID *string) (*store.PrinterRoute, error) {
	rs, err := store.ListPrinterRoutes(ctx, s.d.DB, es)
	if err != nil {
		return nil, err
	}
	var def *store.PrinterRoute
	for i := range rs {
		r := &rs[i]
		if r.DocumentKind != kind {
			continue
		}
		if r.RegisterID == nil {
			def = r
		} else if registerID != nil && *r.RegisterID == *registerID {
			return r, nil
		}
	}
	if def == nil {
		return nil, ErrRouteMissing
	}
	return def, nil
}
