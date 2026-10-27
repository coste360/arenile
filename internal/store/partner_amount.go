package store

import "github.com/coste360/arenile/internal/money"

// PartnerAmountOf resolves bookings.partner_amount, the charge of the
// booking on the account of its partner (spec 30.2.4, INV-104), from the
// partner share and the booking total:
//
//   - no partner or no share: 0;
//   - full: the total;
//   - percent: total x basis points / 10000 (half-even to the cent);
//   - amount: the amount, never more than the total;
//   - draft, cancelled, expired and rejected bookings: 0 (ADR 0028: a
//     cancellation never charges the partner; a penalty is settled with
//     the guest).
//
// It is applied by normalizeBooking on every insert and versioned update,
// so the column always follows total_amount in the same statement as the
// total (INV-104), whatever use case changed it.
func PartnerAmountOf(b *Booking) money.Cents {
	if b.PartnerID == nil || b.PartnerShareType == nil || b.TotalAmount <= 0 {
		return 0
	}
	switch b.Status {
	case "draft", "cancelled", "expired", "rejected":
		return 0
	}
	var v int64
	if b.PartnerShareValue != nil {
		v = *b.PartnerShareValue
	}
	var out money.Cents
	switch *b.PartnerShareType {
	case PartnerShareFull:
		out = b.TotalAmount
	case PartnerSharePercent:
		out = b.TotalAmount.PercentOfBasisPoints(min(max(v, 0), 10000))
	case PartnerShareAmount:
		out = money.Cents(max(v, 0))
	}
	return min(max(out, 0), b.TotalAmount)
}
