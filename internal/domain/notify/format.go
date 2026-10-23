package notify

import (
	"strconv"
	"strings"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
)

// Locale-aware formatting of amounts and dates for the emails (pure). The
// four built-in locales are covered explicitly (spec 20); month and weekday
// names come from the message catalogs.

// formatMoney formats cents in the establishment currency for locale:
// it/de/fr "1.234,50 €" (fr with narrow no-break spaces), en "€1,234.50".
func formatMoney(c money.Cents, currency, locale string) string {
	neg := c < 0
	if neg {
		c = -c
	}
	units := strconv.FormatInt(int64(c)/100, 10)
	cents := int64(c) % 100
	group, decimal := ".", ","
	switch locale {
	case "en":
		group, decimal = ",", "."
	case "fr":
		group = " "
	}
	var b strings.Builder
	for i, r := range units {
		if i > 0 && (len(units)-i)%3 == 0 {
			b.WriteString(group)
		}
		b.WriteRune(r)
	}
	num := b.String() + decimal + twoDigits(cents)
	sym := currencySymbol(currency)
	sign := ""
	if neg {
		sign = "-"
	}
	if locale == "en" {
		return sign + sym + num
	}
	return sign + num + " " + sym
}

func twoDigits(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

func currencySymbol(code string) string {
	switch strings.ToUpper(code) {
	case "", "EUR":
		return "€"
	case "CHF":
		return "CHF"
	case "GBP":
		return "£"
	case "USD":
		return "$"
	}
	return strings.ToUpper(code)
}

// formatDate formats a date as "sabato 12 luglio 2026" (it), "Saturday,
// 12 July 2026" (en), "Samstag, 12. Juli 2026" (de), "samedi 12 juillet
// 2026" (fr), with names from the catalog.
func formatDate(cat catalog, d calendar.Date) string {
	wd := cat.weekday(d.ISOWeekday())
	m := cat.month(int(d.Month()))
	day := strconv.Itoa(d.Day())
	y := strconv.Itoa(d.Year())
	switch cat.locale {
	case "en":
		return wd + ", " + day + " " + m + " " + y
	case "de":
		return wd + ", " + day + ". " + m + " " + y
	default:
		return wd + " " + day + " " + m + " " + y
	}
}

// formatPeriod is one date, or "from – to" for a stay.
func formatPeriod(cat catalog, from, to calendar.Date) string {
	if from.Equal(to) {
		return formatDate(cat, from)
	}
	return cat.t("period_range", map[string]string{"from": formatDate(cat, from), "to": formatDate(cat, to)})
}
