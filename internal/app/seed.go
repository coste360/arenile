package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/setup"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Demo data identifiers.
const (
	DemoOrganizationName = "Lido Demo"
	DemoSlug             = "lido-demo"
)

// SeedResult reports what SeedDemo did.
type SeedResult struct {
	OrganizationID  string
	EstablishmentID string
	Created         bool
}

// SeedDemo creates the demo organization and establishment. It is
// idempotent: when the demo establishment already exists nothing changes.
// Later milestones extend it with inventory, catalog and prices.
func SeedDemo(ctx context.Context, db bun.IDB, c clock.Clock) (*SeedResult, error) {
	res := &SeedResult{}
	err := store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		if e, err := store.GetEstablishmentBySlug(ctx, tx, DemoSlug); err == nil {
			res.OrganizationID, res.EstablishmentID = e.OrganizationID, e.ID
			return nil
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		now := c.Now()
		org, err := store.FindOrganizationByName(ctx, tx, DemoOrganizationName)
		if errors.Is(err, store.ErrNotFound) {
			vat := "IT00000000000"
			org = &store.Organization{ID: store.NewID(), Name: DemoOrganizationName, VATNumber: &vat, CreatedAt: now, UpdatedAt: now}
			if err := store.InsertOrganization(ctx, tx, org); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		e := store.NewEstablishmentDefaults(org.ID, DemoSlug, "Lido Demo", now)
		e.EnabledLocales = []string{"it", "en", "de", "fr"}
		e.ContactEmail = "info@lido-demo.example"
		e.ContactPhone = "+39 0771 000000"
		e.Address = store.Address{Street: "Lungomare Caboto 1", City: "Gaeta", Province: "LT", PostalCode: "04024", Country: "IT"}
		e.Geo = &store.Geo{Lat: 41.2152, Lon: 13.5697}
		e.BookingEnabled = true
		showPrices, hideHeader := true, true
		e.Theme = store.Theme{
			PrimaryColor: "#0D79AC", AccentColor: "#B45309", FontFamily: "Inter", Radius: "md",
			ShowPricesOnMap: &showPrices, HideHeaderInEmbed: &hideHeader,
			CustomTexts: map[string]map[string]string{
				"it": {"intro": "Prenota il tuo ombrellone al Lido Demo."},
				"en": {"intro": "Book your beach umbrella at Lido Demo."},
			},
		}
		if err := store.InsertEstablishment(ctx, tx, e); err != nil {
			return err
		}
		res.OrganizationID, res.EstablishmentID, res.Created = org.ID, e.ID, true
		return audit.NewWriter(c).Record(ctx, tx, audit.Entry{
			EstablishmentID: e.ID, Actor: audit.SystemActor, Action: "seed.demo", EntityType: "establishment", EntityID: e.ID,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("app: seed demo: %w", err)
	}
	return res, nil
}

// DemoOperations adapts the M3 services of the App to the ports of
// setup.SeedDemoOperations (adapter: the setup package knows only its own
// narrow ports, the composition root knows the services).
func (a *App) DemoOperations(p *auth.Principal) setup.DemoOperations {
	const ip = "127.0.0.1"
	return setup.DemoOperations{
		Actor: p, Customers: a.Customers, Bookings: a.Bookings,
		ReleaseDays: func(ctx context.Context, p *auth.Principal, est, passID string, dates []calendar.Date) error {
			_, err := a.Passes.Release(ctx, p, ip, est, passID, pass.ReleaseInput{Dates: dates, Slots: calendar.SlotsFull, Note: "Giorni rilasciati (demo)"})
			return err
		},
		OpenCash: func(ctx context.Context, p *auth.Principal, est string, float money.Cents) (string, error) {
			cs, err := a.Cash.Open(ctx, p, ip, est, cash.OpenInput{OpeningFloat: float})
			if err != nil {
				return "", err
			}
			return cs.ID, nil
		},
		RecordPayment: func(ctx context.Context, p *auth.Principal, est string, pay setup.DemoPayment) error {
			sid := pay.CashSessionID
			_, err := a.Payments.RecordPayment(ctx, p, ip, est, pay.BookingID, payment.Input{Amount: pay.Amount, Method: pay.Method, CashSessionID: &sid})
			return err
		},
		CloseCash: func(ctx context.Context, p *auth.Principal, est, sessionID string, counted store.MethodTotals) error {
			_, err := a.Cash.Close(ctx, p, ip, est, sessionID, nil, cash.CloseInput{Counted: counted, Notes: "Chiusura (demo)"})
			return err
		},
		CancelBooking: func(ctx context.Context, p *auth.Principal, est, id, reason string) error {
			_, err := a.Bookings.Cancel(ctx, p, est, id, booking.CancelInput{Reason: reason, RefundMethod: store.PaymentMethodCash})
			return err
		},
		CheckIn: func(ctx context.Context, p *auth.Principal, est, id string) error {
			_, err := a.Bookings.CheckIn(ctx, p, est, id, booking.ActionInput{})
			return err
		},
	}
}

// SeedDemoOperations adds the M3 operational demo data (customers, passes,
// bookings, payments, cash sessions) to the demo establishment, acting as the
// user with the given email (the demo owner). Idempotent like SeedDemo.
func (a *App) SeedDemoOperations(ctx context.Context, actorEmail string) (*setup.DemoOperationsResult, error) {
	est, err := store.GetEstablishmentBySlug(ctx, a.db, DemoSlug)
	if err != nil {
		return nil, fmt.Errorf("app: seed demo operations: %w", err)
	}
	p, err := a.Auth.PrincipalForEmail(ctx, actorEmail)
	if err != nil {
		return nil, fmt.Errorf("app: seed demo operations: %w", err)
	}
	res, err := a.Setup.SeedDemoOperations(ctx, est.ID, a.DemoOperations(p))
	if err != nil {
		return res, err
	}
	if err := a.seedDemoTerms(ctx, p, est.ID); err != nil {
		return res, fmt.Errorf("app: seed demo terms: %w", err)
	}
	return res, nil
}

// demoTermsVersion is the label of the demo booking terms.
const demoTermsVersion = "demo-2027"

// seedDemoTerms publishes example booking terms, privacy notice and
// marketing consent text for the public page of the demo (spec 12.3, 23),
// when the demo has none yet. Idempotent.
func (a *App) seedDemoTerms(ctx context.Context, p *auth.Principal, establishmentID string) error {
	es, err := store.SystemEstablishmentScope(ctx, a.db, establishmentID)
	if err != nil {
		return err
	}
	if cur, err := a.Online.CurrentTerms(ctx, a.db, es); err != nil || cur != nil {
		return err
	}
	_, err = a.Online.PublishTerms(ctx, p, establishmentID, online.TermsInput{
		Version: demoTermsVersion, MakeCurrent: true,
		Texts: map[string]store.TermsTexts{
			"it": {
				Terms: "Condizioni di prenotazione del Lido Demo (esempio): la prenotazione è confermata al pagamento o, " +
					"se previsto, all'arrivo. Cancellazione gratuita fino a 14 giorni prima, credito del 50% fino a 3 giorni prima.",
				Privacy: "Informativa privacy (esempio): Lido Demo tratta nome, email e telefono solo per gestire la " +
					"prenotazione e gli obblighi di legge. Puoi chiedere accesso, rettifica o cancellazione a info@lido-demo.example.",
				Marketing: "Voglio ricevere offerte e novità del Lido Demo via email (facoltativo, revocabile quando vuoi).",
			},
			"en": {
				Terms: "Lido Demo booking terms (example): the booking is confirmed on payment or, where allowed, on arrival. " +
					"Free cancellation up to 14 days before, 50% credit up to 3 days before.",
				Privacy: "Privacy notice (example): Lido Demo processes name, email and phone only to manage the booking and " +
					"legal obligations. Ask for access, correction or erasure at info@lido-demo.example.",
				Marketing: "I want to receive Lido Demo offers and news by email (optional, can be withdrawn at any time).",
			},
		},
	})
	return err
}
