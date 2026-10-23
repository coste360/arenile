package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/store"
)

// Labels of the report rows and of the CSV headers are Italian (the source
// language, spec 20): the API returns them next to the stable keys, so a
// client can show the label as is or translate the key.

var monthNames = [...]string{
	"Gennaio", "Febbraio", "Marzo", "Aprile", "Maggio", "Giugno",
	"Luglio", "Agosto", "Settembre", "Ottobre", "Novembre", "Dicembre",
}

// dayLabel is "01/07/2027".
func dayLabel(d calendar.Date) string {
	return fmt.Sprintf("%02d/%02d/%04d", d.Day(), int(d.Month()), d.Year())
}

// weekLabel is "Settimana 27/2027" (ISO week).
func weekLabel(d calendar.Date) string {
	y, w := d.Time().ISOWeek()
	return fmt.Sprintf("Settimana %d/%d", w, y)
}

// monthLabel is "Luglio 2027".
func monthLabel(d calendar.Date) string {
	return monthNames[d.Month()-time.January] + " " + fmt.Sprint(d.Year())
}

// channelOrder and channelLabels: booking channels plus counter sales.
var (
	channelOrder  = []string{store.ChannelDesk, store.ChannelOnline, store.ChannelAPI, ChannelCounter}
	channelLabels = map[string]string{
		store.ChannelDesk:   "Cassa",
		store.ChannelOnline: "Online",
		store.ChannelAPI:    "API",
		ChannelCounter:      "Vendita al banco",
	}
)

var (
	methodOrder = []string{
		store.PaymentMethodCash, store.PaymentMethodCard, store.PaymentMethodBankTransfer,
		store.PaymentMethodWallet, store.PaymentMethodCredit, store.PaymentMethodOther,
	}
	methodLabels = map[string]string{
		store.PaymentMethodCash:         "Contanti", //nolint:misspell // Italian
		store.PaymentMethodCard:         "Carta",
		store.PaymentMethodBankTransfer: "Bonifico",
		store.PaymentMethodWallet:       "Wallet",
		store.PaymentMethodCredit:       "Credito cliente",
		store.PaymentMethodOther:        "Altro",
	}
)

var (
	formulaOrder  = []string{"half_day", "day", "multi_day", "week", "fortnight", "month", "season"}
	formulaLabels = map[string]string{
		"half_day": "Mezza giornata", "day": "Giornaliero", "multi_day": "Più giorni", "week": "Settimana",
		"fortnight": "Quindicina", "month": "Mese", "season": "Stagione",
	}
)

var leadLabels = map[LeadTimeBucket]string{
	LeadSameDay: "Stesso giorno", Lead1to3: "1-3 giorni", Lead4to7: "4-7 giorni", Lead8to14: "8-14 giorni",
	Lead15to30: "15-30 giorni", Lead31to60: "31-60 giorni", Lead61Plus: "Oltre 60 giorni",
}

var statusLabels = map[string]string{
	"confirmed": "Confermata", "checked_in": "Arrivato", "completed": "Completata", "no_show": "Non presentato", //nolint:misspell // Italian
}

var slotLabels = map[calendar.Slot]string{calendar.AM: "Mattina", calendar.PM: "Pomeriggio"}

// labelOr returns m[k] or k itself.
func labelOr[K comparable](m map[K]string, k K) string {
	if l, ok := m[k]; ok {
		return l
	}
	return fmt.Sprint(k)
}

// rateLabel is "22%" or "5,5%".
func rateLabel(bp int64) string { return fiscal.FormatRate(bp) + "%" }

// i18nName is the Italian name, else any name, else the fallback.
func i18nName(t store.I18nText, fallback string) string {
	if v := strings.TrimSpace(t["it"]); v != "" {
		return v
	}
	for _, v := range t {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return fallback
}

// displayName is "First Last", or AnonymizedName for anonymised customers
// (spec 23). It never uses contact data.
func displayName(first, last string, anonymizedAt *time.Time) string {
	if anonymizedAt != nil {
		return AnonymizedName
	}
	return strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last))
}
