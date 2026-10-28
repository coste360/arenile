package public

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// Mapping between the checkout domain types and the api.* schemas of the
// public API. Never exposes other customers' data: customer fields come
// only from the session or magic link owner, masked where shown back.

// decode reads a JSON body into v (400 bad_request on malformed input).
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		problem.Write(w, r, problem.BadRequest, problem.Detail("Corpo JSON non valido."))
		return false
	}
	return true
}

// decodeOptional reads an optional JSON body (empty body = zero value).
func decodeOptional(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil && err.Error() != "EOF" {
		problem.Write(w, r, problem.BadRequest, problem.Detail("Corpo JSON non valido."))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ptr[T any](v T) *T { return &v }

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func text(t store.I18nText) api.I18nText {
	if t == nil {
		return api.I18nText{}
	}
	return api.I18nText(t)
}

func formulas(fs []string) []api.Formula {
	out := make([]api.Formula, len(fs))
	for i, f := range fs {
		out[i] = api.Formula(f)
	}
	return out
}

func extrasFromAPI(xs *[]api.PublicExtraInput) []booking.ExtraInput {
	if xs == nil {
		return nil
	}
	out := make([]booking.ExtraInput, len(*xs))
	for i, x := range *xs {
		out[i] = booking.ExtraInput{ExtraID: x.ExtraId, Quantity: x.Quantity}
	}
	return out
}

func linesFromAPI(ls []api.PublicLineInput) []booking.LineInput {
	out := make([]booking.LineInput, len(ls))
	for i, l := range ls {
		in := booking.LineInput{PackID: l.PackId, Quantity: 1, Extras: extrasFromAPI(l.Extras)}
		if l.Quantity != nil {
			in.Quantity = *l.Quantity
		}
		if l.SpotId != nil {
			in.SpotID = *l.SpotId
		}
		if l.ZoneId != nil {
			in.ZoneID = *l.ZoneId
		}
		out[i] = in
	}
	return out
}

func selectionFromAPI(s api.PublicSelection) checkout.Selection {
	out := checkout.Selection{
		StartsOn: s.StartsOn, EndsOn: s.EndsOn, Slots: slotsOf(s.Slots), Formula: string(s.Formula),
		Lines: linesFromAPI(s.Lines), Extras: extrasFromAPI(s.Extras),
	}
	if s.PromoCode != nil {
		out.PromoCode = strings.TrimSpace(*s.PromoCode)
	}
	if s.AgreementCode != nil {
		out.AgreementCode = strings.TrimSpace(*s.AgreementCode)
	}
	return out
}

func conflictsToAPI(cs []availability.Conflict) []api.AllocationConflict {
	out := make([]api.AllocationConflict, len(cs))
	for i, c := range cs {
		out[i] = api.AllocationConflict{
			Date: c.Date, Slot: api.Slot(c.Slot), Reason: api.AllocationConflictReason(c.Reason),
			SpotId: optStr(c.SpotID), PoolId: optStr(c.PoolID), ZoneId: optStr(c.ZoneID),
		}
	}
	return out
}

func violationsToAPI(vs []policy.Violation) []api.PolicyViolation {
	out := make([]api.PolicyViolation, len(vs))
	for i, v := range vs {
		out[i] = api.PolicyViolation{Code: string(v.Code)}
		if len(v.Params) > 0 {
			p := v.Params
			out[i].Params = &p
		}
	}
	return out
}

func quoteToAPI(q *checkout.Quote) api.PublicQuote {
	out := api.PublicQuote{
		Available: q.Available, Currency: q.Currency, Total: int64(q.Total), DepositDue: int64(q.DepositDue),
		DueNow: int64(q.DueNow), BalanceLater: int64(q.BalanceLater), PaymentPlan: api.PaymentPlan(q.Plan),
		Conflicts: conflictsToAPI(q.Conflicts), Violations: violationsToAPI(q.Violations), Items: make([]api.PublicQuoteItem, len(q.Items)),
	}
	for i, it := range q.Items {
		item := api.PublicQuoteItem{Kind: api.PublicQuoteItemKind(it.Kind), Label: text(it.Label), Amount: int64(it.Amount), RefId: optStr(it.RefID)}
		if it.Quantity > 0 {
			item.Quantity = ptr(it.Quantity)
		}
		out.Items[i] = item
	}
	if q.Promo != nil {
		out.Promo = &api.PromoStatus{Code: q.Promo.Code, Status: api.PromoStatusStatus(q.Promo.Status)}
	}
	out.Agreement = agreementStatusToAPI(q.Agreement)
	return out
}

// summaryToAPI is the public summary of a booking (no customer data; the
// code appears only from the confirmation on).
func summaryToAPI(d *booking.Detail, q *checkout.Quote, names checkout.Labels) api.PublicBookingSummary {
	b := &d.Booking
	out := api.PublicBookingSummary{
		StartsOn: b.StartsOn, EndsOn: b.EndsOn, Slots: api.Slots(b.Slots), Formula: api.Formula(b.Formula),
		Total: int64(b.TotalAmount), DepositDue: int64(b.DepositDue), Paid: int64(b.PaidAmount),
		BalanceDue: int64(b.BalanceDue()), Lines: []api.PublicBookingLine{}, Extras: []api.PublicBookingExtra{},
	}
	if q != nil {
		out.Currency = q.Currency
	}
	switch booking.Status(b.Status) {
	case booking.StatusConfirmed, booking.StatusCheckedIn, booking.StatusCompleted, booking.StatusAwaitingApproval, booking.StatusCancelled:
		out.Code = ptr(b.Code)
	}
	lineExtras := map[string][]api.PublicBookingExtra{}
	for _, x := range d.Extras {
		e := api.PublicBookingExtra{ExtraId: x.ExtraID, Quantity: x.Quantity, Name: label(names.Extras, x.ExtraID)}
		if x.BookingLineID == nil {
			out.Extras = append(out.Extras, e)
		} else {
			lineExtras[*x.BookingLineID] = append(lineExtras[*x.BookingLineID], e)
		}
	}
	for _, l := range d.Lines {
		pl := api.PublicBookingLine{
			Assignment: api.AssignmentStatus(l.AssignmentStatus), PackId: l.PackID, PackName: label(names.Packs, l.PackID),
			Quantity: l.Quantity, Extras: lineExtras[l.ID], ZoneId: optStr(l.ZoneID),
		}
		if pl.Extras == nil {
			pl.Extras = []api.PublicBookingExtra{}
		}
		if l.ZoneID != "" {
			zn := label(names.Zones, l.ZoneID)
			pl.ZoneName = &zn
		}
		if l.SpotID != nil {
			if label, ok := d.SpotLabels[*l.SpotID]; ok {
				pl.SpotLabel = ptr(label)
			}
		}
		out.Lines = append(out.Lines, pl)
	}
	return out
}

// label resolves a name of the summary labels.
func label(m map[string]store.I18nText, id string) api.I18nText { return text(m[id]) }

func stateToAPI(st *checkout.State) api.CheckoutState {
	names := st.Labels
	d := st.Booking
	out := api.CheckoutState{
		BookingId: d.Booking.ID, CheckoutState: api.CheckoutSessionState(st.Session.State), NextAction: api.CheckoutNextAction(st.NextAction),
		Status: api.BookingStatus(d.Booking.Status), Summary: summaryToAPI(d, st.Quote, names), HoldExpiresAt: d.Booking.HoldExpiresAt,
	}
	if st.Quote != nil {
		q := quoteToAPI(st.Quote)
		out.Quote = &q
	}
	if st.Payment != nil {
		out.Payment = &api.PublicPaymentState{Status: api.PublicPaymentStateStatus(st.Payment.Status), Amount: int64(st.Payment.Amount), FailureReason: optStr(st.Payment.FailureReason)}
	}
	if c := &d.Customer; !customer.IsPlaceholder(c) {
		cc := api.CheckoutCustomer{FirstName: c.FirstName, LastName: c.LastName, Phone: c.Phone, AcceptedTermsVersion: d.Booking.TermsVersion}
		if c.Email != nil {
			cc.Email = *c.Email
		}
		cc.MarketingConsent = c.MarketingConsent != nil
		out.Customer = &cc
	}
	if cf := st.Confirmation; cf != nil {
		out.Confirmation = &api.PublicConfirmation{
			Code: cf.Code, QrPayload: cf.QRPayload, QrSvg: optStr(cf.QRSVG), ManageLinkSent: cf.ManageLinkSent, EmailMasked: optStr(cf.EmailMasked),
		}
	}
	return out
}

func payToAPI(p *checkout.PayOutcome) api.PayResult {
	out := api.PayResult{
		Mode: api.PayResultMode(p.Mode), Status: api.BookingStatus(p.Status), RedirectUrl: optStr(p.RedirectURL),
		ClientSecret: optStr(p.ClientSecret), PublishableKey: optStr(p.PublishableKey), HoldExpiresAt: p.HoldExpiresAt,
	}
	if p.Amount > 0 {
		out.Amount = ptr(int64(p.Amount))
	}
	return out
}

func refundToAPI(rf booking.RefundPreview) api.RefundPreview {
	out := api.RefundPreview{Amount: int64(rf.Amount), As: api.RefundPreviewAs(rf.As), PaidAmount: int64(rf.PaidAmount), RefundPercent: rf.RefundPercent}
	if out.As == "" {
		out.As = api.RefundPreviewAs(booking.SettleRefund)
	}
	if rf.Tier != nil {
		t := api.PolicyCancellationTier{RefundPercent: rf.Tier.RefundPercent, UntilDaysBefore: rf.Tier.UntilDaysBefore}
		if rf.Tier.As != "" {
			t.As = ptr(api.PolicyCancellationTierAs(rf.Tier.As))
		}
		out.Tier = &t
	}
	return out
}

func managedToAPI(m *checkout.Managed) api.ManageBooking {
	names, q := m.Labels, m.Quote
	d := m.Booking
	b := &d.Booking
	out := api.ManageBooking{
		BookingId: b.ID, Code: b.Code, Status: api.BookingStatus(b.Status), IsSeasonPass: b.IsSeasonPass, Version: b.Version,
		LinkExpiresAt: m.Link.ExpiresAt.UTC(), QrPayload: m.QRPayload, QrSvg: optStr(m.QRSVG), Summary: summaryToAPI(d, q, names),
		Allowed: api.ManageActions{
			PayBalance: m.Allowed.PayBalance, Modify: m.Allowed.Modify, Cancel: m.Allowed.Cancel, ReleaseDays: m.Allowed.ReleaseDays,
			ModifyUntil: m.Allowed.ModifyUntil, CancelUntilFree: m.Allowed.CancelUntilFree,
		},
		Customer: api.ManageCustomer{FirstName: d.Customer.FirstName, LastName: d.Customer.LastName},
	}
	out.Summary.Code = ptr(b.Code)
	if d.Customer.Email != nil {
		out.Customer.EmailMasked = ptr(notify.MaskEmail(*d.Customer.Email))
	}
	if d.Customer.Phone != nil {
		out.Customer.PhoneMasked = ptr(maskPhone(*d.Customer.Phone))
	}
	if m.CancelPreview != nil {
		rp := refundToAPI(*m.CancelPreview)
		out.CancellationPreview = &rp
	}
	if m.Refund != nil {
		out.Refund = &api.ManageRefundStatus{
			Status: api.RefundRequestStatus(m.Refund.Status), Amount: int64(m.Refund.Amount),
			RequestedAt: m.Refund.RequestedAt, CompletedAt: m.Refund.CompletedAt,
		}
	}
	if b.IsSeasonPass {
		rels := make([]api.ManageRelease, len(m.Releases))
		for i, r := range m.Releases {
			rels[i] = releaseToAPI(r)
		}
		out.Releases = &rels
	}
	return out
}

func releaseToAPI(r store.SeasonRelease) api.ManageRelease {
	return api.ManageRelease{Date: r.Date, Slot: api.Slot(r.Slot), CreditAmount: int64(r.CreditAmount), Resold: ptr(r.ResoldBookingLineID != nil)}
}

// maskPhone keeps the international prefix and the last two digits.
func maskPhone(p string) string {
	if len(p) <= 5 {
		return "***"
	}
	return p[:3] + strings.Repeat("*", len(p)-5) + p[len(p)-2:]
}

func mapToAPI(v *checkout.MapView) api.PublicMapDocument {
	m := v.Map
	out := api.PublicMapDocument{
		Map: api.PublicMap{
			Id: m.ID, Name: text(m.Name), Width: m.Width, Height: m.Height, GridSize: m.GridSize, SeaSide: api.SeaSide(m.SeaSide), Version: m.Version,
		},
		Spots:       make([]api.PublicSpot, len(v.Spots)),
		Decorations: make([]api.Decoration, len(v.Decorations)),
	}
	if b := mapping.ParseBackground(m.Background); b != nil {
		var ab api.MapBackground
		if err := admin.ConvertJSON(b, &ab); err == nil {
			out.Map.Background = &ab
		}
	}
	for i, s := range v.Spots {
		out.Spots[i] = api.PublicSpot{
			Id: s.ID, ZoneId: s.ZoneID, Label: s.Label, RowLabel: s.RowLabel, Kind: api.SpotKind(s.Kind),
			X: s.X, Y: s.Y, Rotation: s.Rotation, Shape: api.Shape{Type: api.ShapeType(s.Shape.Type), W: s.Shape.W, H: s.Shape.H},
			PriceCategory: v.PriceCategory[s.ID], Asset: s.Asset,
		}
	}
	for i, d := range v.Decorations {
		dec := api.Decoration{
			Id: d.ID, MapId: d.MapID, Kind: api.DecorationKind(d.Kind), X: d.X, Y: d.Y, Rotation: d.Rotation,
			Shape: api.Shape{Type: api.ShapeType(d.Shape.Type), W: d.Shape.W, H: d.Shape.H}, Asset: d.Asset,
		}
		if d.Variant != nil && *d.Variant != "" {
			dec.Variant = ptr(api.DecorationVariant(*d.Variant))
		}
		if len(d.Label) > 0 {
			l := api.I18nText(d.Label)
			dec.Label = &l
		}
		out.Decorations[i] = dec
	}
	return out
}

func slotsOf(s api.Slots) calendar.Slots { return calendar.Slots(strings.ToUpper(string(s))) }
