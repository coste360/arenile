package report

import (
	"context"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
)

// RegisterCounters are the counters of a register entry or category.
type RegisterCounters struct {
	Agreements int
	Bookings   int
	SpotDays   int
	People     int
	Revenue    money.Cents
	Discount   money.Cents
}

// RegisterEntry is one agreement of the register.
type RegisterEntry struct {
	AgreementID          string
	Name                 string
	Code                 string
	PartnerName          string
	CustomerCategoryName string
	Target               string
	RegisterCategory     string
	ConditionsText       string
	ValidFrom, ValidTo   *calendar.Date
	RegisterCounters
}

// RegisterCategoryTotal is the total of one register category.
type RegisterCategoryTotal struct {
	RegisterCategory string
	RegisterCounters
}

// AgreementsRegister is api.AgreementsRegister (neutral layout, ADR 0028).
type AgreementsRegister struct {
	EstablishmentName      string
	EstablishmentLegalName string
	EstablishmentVATNumber string
	SeasonID               string
	SeasonYear             int
	Period                 Period
	GeneratedAt            time.Time
	Entries                []RegisterEntry
	ByCategory             []RegisterCategoryTotal
	Totals                 RegisterCounters
}

// AgreementsRegister returns the register of the agreements active in the
// period (seasonID optional, for the header). Requires
// auth.PermReportsRead.
func (s *Service) AgreementsRegister(ctx context.Context, p *auth.Principal, establishmentID string, period Period, seasonID string) (*AgreementsRegister, error) {
	_, _, _, _, _ = ctx, p, establishmentID, period, seasonID
	return nil, configerr.ErrNotImplemented
}
