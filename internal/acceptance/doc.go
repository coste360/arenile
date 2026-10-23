// Package acceptance holds the end-to-end acceptance tests of the
// milestones (spec 25, 28). They boot the real application (internal/app:
// every service, the HTTP API, the outbox dispatcher, the jobs and the SSE
// stream) on a temporary database with an injected clock.Fake, drive it
// only through the public admin API with the generated contract types
// (internal/http/api), and then check the cross-cutting invariants of
// docs/invariants.md directly on the database.
//
// The tests carry the build tag integration (go test -tags=integration
// ./internal/acceptance/): SQLite on a file always (several connections, as
// in production), PostgreSQL when ARENILE_TEST_POSTGRES_URL is set.
//
// M3 acceptance (spec 28): TestAcceptance_SimulatedDay runs a beach day of
// more than 300 mixed operations (desk bookings, holds and conversions,
// expiries through the fake clock, modifications and moves, cancellations
// with refunds, season passes with releases and resales, payments in cash
// and by card, counter sales, cash movements, cash opening and closing,
// concurrent bursts on one spot and one pool) and verifies that
// allocations, bookings, payments, cash sessions and the corrispettivi
// export agree with each other, and that the SSE stream carried the changes
// without personal data.
//
// M4 acceptance (spec 28, "nessuna doppia vendita tra cassa e online in un
// test con carico parallelo"): the same day also sells online through the
// public API (online_test.go). Stripe is the fake of package stripetest
// (ARENILE_STRIPE_API_URL, real webhook signatures, scenario clock); the
// signed webhook is the only online confirmation. 120 desk and online
// attempts race on one spot and day (exactly one sale); online customers
// pay (webhook delivered twice), abandon, expire or leave the payment
// pending; the gazebos become a zone-mode zone with an online quota and
// race between channels (never more sales than gazebos, distinct spots
// assigned at confirmation); refunds go through the provider and a late
// payment of an expired booking is recorded and refunded in full. The
// invariants above cover the online bookings too. The real browser part of
// the M4 acceptance (smartphone, iframe on another origin, under 2
// minutes) is e2e/tests/m4-acceptance.spec.ts; the real binary one is
// scripts/acceptance/m4-real-binary.sh.
//
// TestAcceptance_DemoSeed runs the M3 demo seed through App.DemoOperations
// (the adapter `arenile seed demo` uses) and checks the same invariants.
// Any operation answering 501 Not Implemented fails the test.
// ARENILE_ACCEPTANCE_SEED replays a given random sequence (the seed is
// logged by every run).
package acceptance
