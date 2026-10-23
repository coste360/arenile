package report

import (
	"github.com/coste360/arenile/internal/store"
)

// Functional core of the top customers and open balances reports. Only the
// display name of a customer is ever exposed (spec 23).

// buildTopCustomers ranks the rows returned by store.ReportTopCustomers
// (already sorted and limited) and attaches the spot-days.
func buildTopCustomers(p Period, sort TopCustomersSort, rows []store.ReportCustomer, spotDays map[string]int) *TopCustomers {
	out := &TopCustomers{Period: p, Sort: sort, Rows: make([]TopCustomerRow, 0, len(rows))}
	for i, r := range rows {
		row := TopCustomerRow{
			Rank: i + 1, CustomerID: r.CustomerID, DisplayName: displayName(r.FirstName, r.LastName, r.AnonymizedAt),
			Bookings: r.Bookings, SpotDays: spotDays[r.CustomerID], Revenue: r.Revenue,
		}
		if r.LastBookingOn != nil && !r.LastBookingOn.IsZero() {
			d := *r.LastBookingOn
			row.LastBookingOn = &d
		}
		out.Rows = append(out.Rows, row)
	}
	return out
}

// buildOpenBalances maps the open balance rows and sums the balances.
func buildOpenBalances(p Period, rows []store.ReportOpenBalance) *OpenBalances {
	out := &OpenBalances{Period: p, Rows: make([]OpenBalanceRow, 0, len(rows))}
	for _, r := range rows {
		bal := max(r.TotalAmount-r.PaidAmount, 0)
		out.Rows = append(out.Rows, OpenBalanceRow{
			BookingID: r.BookingID, Code: r.Code, CustomerID: r.CustomerID,
			DisplayName: displayName(r.FirstName, r.LastName, r.AnonymizedAt), Status: r.Status,
			StartsOn: r.StartsOn, EndsOn: r.EndsOn, Total: r.TotalAmount, Paid: r.PaidAmount, Balance: bal,
		})
		out.TotalBalance += bal
	}
	return out
}
