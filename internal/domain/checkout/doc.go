// Package checkout orchestrates the public booking flow (spec 10.1, 12.3)
// and the customer manage area (spec 12.4) on top of the booking service,
// the inventory, the pricing configuration, the payment gateway and notify.
// It owns no business rule of its own beyond the flow: availability stays
// in the database constraints (inventory), prices in the pure engine
// (pricingconfig), transitions in internal/domain/booking.
//
// Ownership (M4): the "checkout" builder owns this package, the handlers in
// internal/http/public/{availability,checkout,manage}.go, the public-channel
// methods of booking (internal/domain/booking/online.go), customer
// (online.go), pass (online.go), inventory (public.go), the repository
// functions in internal/store/checkout.go (model checkout_sessions) and the
// concurrency test of the acceptance criterion "no double sale between the
// cash desk and online under parallel load" (TestConcurrency_OnlineVsDesk, with TestConcurrency_ZoneAutoAssign).
//
// # Files
//
//   - ports.go: interfaces consumed (Settings, Inventory, Pricer, Bookings,
//     Payments, Notifier, SeasonPasses), asserted in internal/app/m4.go;
//   - plan.go: the pure core (functional core) with table tests:
//     validateSelection (what the online channel may sell: slots and
//     formulas offered, booking window, zones visible online, packs and
//     extras of channel online, seat/zone/quantity line shapes),
//     paymentPlan (approval -> at desk -> deposit -> full), deriveState
//     (session state and next action, never writing), manageActions,
//     indicators, calendar statuses, quote items;
//   - token.go: checkout session tokens (256-bit, base64url; SHA-256 hex in
//     checkout_sessions.token_hash; lookups are by hash, so no comparison
//     can leak timing);
//   - browse.go, profile.go: Profile, Calendar, Map, Availability, Catalog,
//     Quote (reads, channel online; "from" prices from the pure pricing
//     engine on the season price list, cached briefly in cache.go; the
//     availability counts are never cached);
//   - session.go: Create, State, UpdateCustomer, Pay, Abandon, Document;
//   - manage.go: Manage, ManageDocument, ManagePay, ManageModify,
//     ManageCancel, ManageRelease, RequestLink;
//   - checkouttest/: the flow on a real database for integration tests.
//
// # Zone-mode zones (spec 7.5)
//
// Zone-mode lines consume the zone pool while held (pending assignment:
// the conditional pool UPDATE decides the capacity and the online quota)
// and get their spot at confirmation when the zone has auto_assign: the
// booking service runs the greedy engine inside the confirmation
// transaction (manual driver, payment webhook, approval), one savepoint
// per line, re-planning on a concurrent placement (booking/online.go
// assignOnline). Never from the browser.
//
// # Limits (spec 17.2, 22)
//
// The HTTP middleware limits every public call per IP (reads and writes
// apart, quotes count as reads). The service adds a budget of checkout
// creations per establishment (Deps.CreatesPerEstablishment) and, for
// "send me a new link", limits per IP and per email + booking code; the
// link request itself runs in the background so the response time never
// tells a match from a miss.
//
// # Flow and transactions
//
//	Create:   Settings.ResolvePublic(slug) -> ONE tx: Bookings.CreateOnline
//	          (held, hold TTL) + insert checkout_sessions(token hash) -> token
//	State:    session by token hash + booking id; NEVER confirms (INV-19);
//	          reads gateway.LatestForBooking for the payment status
//	Update:   tx: Bookings.AttachCustomer (terms version must equal
//	          Settings.CurrentTerms; marketing consent source "online")
//	Pay:      policy plan: at_desk / approval -> tx: ConfirmWithoutPayment;
//	          online -> tx: Bookings.StartPayment (hold +30 min max) then
//	          Payments.Start (provider call, see gateway doc) -> redirect URL
//	          with return URL <base>/b/<slug>?checkout=<booking id> (the page
//	          resumes from sessionStorage and polls State)
//	Abandon:  tx: Bookings.Abandon + session abandoned
//
// Session state follows the booking: open (held), paying
// (pending_payment), completed (confirmed / awaiting approval), expired,
// abandoned. Hold expiry is enforced by the booking hold job and by the
// inventory (expired holds never block a sale).
//
// # Security
//
//   - session token only in the X-Arenile-Checkout header (sessionStorage on
//     the client, spec 12.2); wrong token, other booking or other slug ->
//     ErrSessionNotFound (404 checkout_not_found), no distinction;
//   - magic links resolved by notify (hash only); manage endpoints never
//     reveal whether a token existed;
//   - RequestLink always succeeds (no enumeration) and is rate limited;
//   - responses never carry other customers' data; customer data only for
//     the session or the magic link owner, masked where shown back.
package checkout
