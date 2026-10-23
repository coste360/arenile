//go:build integration

package checkout_test

import (
	"testing"

	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/checkout/checkouttest"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestCheckoutFlow_NeverLinksExistingCustomer: step 4 never links the held
// booking to an existing customer whose email or phone matches what the
// visitor typed (GDPR: the session would echo the stored name, email and
// phone back to anyone who knows one of them; the booking would land in
// the victim's history and be priced with their tags). The details typed
// go to a customer created for this checkout, which later submits of the
// same session update in place; a customer with other bookings is never
// rewritten from the public page.
func TestCheckoutFlow_NeverLinksExistingCustomer(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		now := e.F.Clock.Now()
		victimEmail, victimPhone := "giulia.barbieri@example.org", "+393330000021"
		victim := &store.Customer{
			ID: store.NewID(), EstablishmentID: e.F.Est.ID, FirstName: "Giulia", LastName: "Barbieri",
			Email: &victimEmail, Phone: &victimPhone, Locale: "it", Tags: []string{"vip"}, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := store.InsertCustomer(ctx, db, e.ES, victim); err != nil {
			t.Fatal(err)
		}
		s := e.Create(e.Seat(e.F.Spots[0], "2027-07-10"))
		in := checkout.CustomerInput{FirstName: "Attacker", LastName: "Evil", Email: victimEmail, Phone: "+39 333 999 8877", Locale: "it"}
		st, err := e.Svc.UpdateCustomer(ctx, slug, s.BookingID, s.Token, in)
		if err != nil {
			t.Fatalf("details with a known email: %v", err)
		}
		c := st.Booking.Customer
		if c.ID == victim.ID || c.FirstName != "Attacker" || c.LastName != "Evil" || c.Phone == nil || *c.Phone != "+393339998877" || len(c.Tags) != 0 {
			t.Fatalf("the session must only see what it typed, got %+v", c)
		}
		first := c.ID

		// Phone match, different email: still no link; the customer of
		// this checkout is updated in place (typo corrections).
		in.Email, in.Phone = "attacker@evil.test", victimPhone
		st, err = e.Svc.UpdateCustomer(ctx, slug, s.BookingID, s.Token, in)
		if err != nil {
			t.Fatalf("details with a known phone: %v", err)
		}
		c = st.Booking.Customer
		if c.ID != first || c.FirstName != "Attacker" || c.Email == nil || *c.Email != "attacker@evil.test" || c.Version != 2 {
			t.Fatalf("second submit must update the checkout's own customer, got %+v", c)
		}
		if n := e.Count((*store.Customer)(nil), "establishment_id = ? AND anonymized_at IS NULL", e.F.Est.ID); n != 2 {
			t.Fatalf("customers = %d, want victim + the checkout's own", n)
		}
		got, err := store.GetCustomer(ctx, db, e.ES, victim.ID)
		if err != nil || got.Version != 1 || got.FirstName != "Giulia" || *got.Email != victimEmail || *got.Phone != victimPhone {
			t.Fatalf("victim must be untouched: %v %+v", err, got)
		}
		if b := e.Booking(s.BookingID); b.CustomerID != first {
			t.Fatalf("booking customer %s, want %s", b.CustomerID, first)
		}

		// A customer with other bookings (linked by the staff) is never
		// rewritten by the public page: a new one is created instead.
		desk := e.F.MustCreate(e.F.Input(e.F.Spots[1], "2027-07-12", "2027-07-12"))
		if _, err := db.NewUpdate().Model((*store.Booking)(nil)).Set("customer_id = ?", desk.Booking.CustomerID).
			Where("id = ?", s.BookingID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		in.FirstName = "Altro"
		st, err = e.Svc.UpdateCustomer(ctx, slug, s.BookingID, s.Token, in)
		if err != nil {
			t.Fatalf("details over a staff customer: %v", err)
		}
		if st.Booking.Customer.ID == desk.Booking.CustomerID || st.Booking.Customer.FirstName != "Altro" {
			t.Fatalf("staff customer must not be rewritten, got %+v", st.Booking.Customer)
		}
		if mario, err := store.GetCustomer(ctx, db, e.ES, desk.Booking.CustomerID); err != nil || mario.FirstName != "Mario" || mario.Version != 1 {
			t.Fatalf("staff customer changed: %v %+v", err, mario)
		}
		if customer.IsPlaceholder(&st.Booking.Customer) {
			t.Fatal("placeholder after details")
		}
	})
}
