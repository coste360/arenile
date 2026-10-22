package policy

// Channel is the origin of a booking (spec 5.1).
type Channel string

// Booking channels.
const (
	ChannelDesk   Channel = "desk"
	ChannelOnline Channel = "online"
	ChannelAPI    Channel = "api"
)

// Valid reports whether c is a known channel.
func (c Channel) Valid() bool {
	return c == ChannelDesk || c == ChannelOnline || c == ChannelAPI
}

// Formula is a duration type (spec 5.1, 6.3).
type Formula string

// Duration formulas.
const (
	FormulaHalfDay   Formula = "half_day"
	FormulaDay       Formula = "day"
	FormulaMultiDay  Formula = "multi_day"
	FormulaWeek      Formula = "week"
	FormulaFortnight Formula = "fortnight"
	FormulaMonth     Formula = "month"
	FormulaSeason    Formula = "season"
)

// Valid reports whether f is a known formula.
func (f Formula) Valid() bool {
	switch f {
	case FormulaHalfDay, FormulaDay, FormulaMultiDay, FormulaWeek, FormulaFortnight, FormulaMonth, FormulaSeason:
		return true
	}
	return false
}

// HasFixedLength reports whether the length of the stay is implied by the
// formula itself (week, fortnight, month, season). Duration min/max days do not
// apply to these formulas: their length is validated by the catalog and the
// pricing engine.
func (f Formula) HasFixedLength() bool {
	switch f {
	case FormulaWeek, FormulaFortnight, FormulaMonth, FormulaSeason:
		return true
	}
	return false
}

// PaymentMode tells how much must be paid when the booking is made.
type PaymentMode string

// Payment modes.
const (
	PaymentFull    PaymentMode = "full"
	PaymentDeposit PaymentMode = "deposit"
	PaymentNone    PaymentMode = "none"
)

// DepositType is the kind of deposit value.
type DepositType string

// Deposit types.
const (
	// DepositPercent: Value is an integer percentage (0..100) of the total.
	DepositPercent DepositType = "percent"
	// DepositAmount: Value is a fixed amount in cents, capped at the total.
	DepositAmount DepositType = "amount"
)

// Compensation tells how money returned to the customer is given back.
type Compensation string

// Compensation kinds.
const (
	AsRefund Compensation = "refund"
	AsCredit Compensation = "credit"
)

// Same-day only_slot values.
const (
	// OnlySlotAM restricts same-day bookings to the morning slot.
	OnlySlotAM = "AM"
	// OnlySlotPM restricts same-day bookings to the afternoon slot.
	OnlySlotPM = "PM"
	// OnlySlotPMAfter keeps same-day bookings open after "until", but only for
	// the PM slot. Before "until" every slot is allowed.
	OnlySlotPMAfter = "PM_after"
)

// Rules is the set of constraints of a booking policy (spec 9). Every field is
// optional: a nil field is "not specified" and is inherited from the
// establishment default policy during Resolve. Nested objects are merged field
// by field as well.
type Rules struct {
	MinLeadTime                  *MinLeadTime         `json:"min_lead_time,omitempty"`
	MaxLeadTimeDays              *int                 `json:"max_lead_time_days,omitempty"`
	SameDayBooking               *SameDayBooking      `json:"same_day_booking,omitempty"`
	Duration                     *Duration            `json:"duration,omitempty"`
	Payment                      *Payment             `json:"payment,omitempty"`
	Cancellation                 []CancellationTier   `json:"cancellation,omitempty"`
	Modification                 *Modification        `json:"modification,omitempty"`
	WeatherPolicy                *WeatherPolicy       `json:"weather_policy,omitempty"`
	ApprovalRequired             *bool                `json:"approval_required,omitempty"`
	MaxSpotsPerBooking           *int                 `json:"max_spots_per_booking,omitempty"`
	AllowSpotChangeDuringStay    *bool                `json:"allow_spot_change_during_stay,omitempty"`
	CustomerCanReleaseSeasonDays *bool                `json:"customer_can_release_season_days,omitempty"`
	SeasonReleaseCredit          *SeasonReleaseCredit `json:"season_release_credit,omitempty"`
}

// MinLeadTime is the minimum advance with which a booking must be placed.
//
// Days is the minimum number of calendar days between the booking day and the
// start date. CutoffTime ("HH:MM", establishment time) is the time of day after
// which the last eligible day is closed: a booking starting on S must be placed
// before CutoffTime on day S - max(Days, 1). Same-day bookings (Days == 0) are
// governed by SameDayBooking instead.
type MinLeadTime struct {
	Days       *int    `json:"days,omitempty"`
	CutoffTime *string `json:"cutoff_time,omitempty"`
}

// SameDayBooking controls bookings starting today.
type SameDayBooking struct {
	Enabled  *bool   `json:"enabled,omitempty"`
	Until    *string `json:"until,omitempty"`     // "HH:MM", establishment time
	OnlySlot *string `json:"only_slot,omitempty"` // "", "AM", "PM", "PM_after"
}

// Duration constrains the formula and length of the stay.
type Duration struct {
	AllowedFormulas []Formula `json:"allowed_formulas,omitempty"`
	MinDays         *int      `json:"min_days,omitempty"`
	MaxDays         *int      `json:"max_days,omitempty"`
	// FixedStartWeekday is the ISO weekday (1 = Monday .. 7 = Sunday) on which
	// stays must start. 0 explicitly means "any weekday" and can be used to
	// override a default that sets one.
	FixedStartWeekday *int `json:"fixed_start_weekday,omitempty"`
}

// Payment tells what must be paid upfront and when the balance is due.
type Payment struct {
	Mode       *PaymentMode `json:"mode,omitempty"`
	Deposit    *DepositRule `json:"deposit,omitempty"`
	BalanceDue *string      `json:"balance_due,omitempty"` // "on_arrival" | "days_before:N"
}

// DepositRule describes the deposit amount when Payment.Mode is "deposit".
type DepositRule struct {
	Type  *DepositType `json:"type,omitempty"`
	Value *int64       `json:"value,omitempty"`
}

// CancellationTier is one step of the cancellation schedule. A tier applies when
// the cancellation happens at least UntilDaysBefore days before the start date.
type CancellationTier struct {
	UntilDaysBefore int          `json:"until_days_before"`
	RefundPercent   int          `json:"refund_percent"`
	As              Compensation `json:"as,omitempty"` // default "refund"
}

// Modification controls customer self-service changes.
type Modification struct {
	AllowedUntilDaysBefore *int   `json:"allowed_until_days_before,omitempty"`
	Fee                    *int64 `json:"fee,omitempty"` // cents
}

// WeatherPolicy controls compensation for days declared unusable by weather.
type WeatherPolicy struct {
	Enabled *bool         `json:"enabled,omitempty"`
	As      *Compensation `json:"as,omitempty"`
}

// SeasonReleaseCredit controls the credit given to a season pass holder when a
// released day is resold.
type SeasonReleaseCredit struct {
	Enabled *bool `json:"enabled,omitempty"`
	Percent *int  `json:"percent,omitempty"`
}

// Scope filters the bookings a policy applies to. An empty dimension matches
// everything. Values are compared as strings: zones and packs may be listed by
// ID or by code (see doc.go).
type Scope struct {
	Channels    []Channel `json:"channels,omitempty"`
	Zones       []string  `json:"zones,omitempty"`
	Packs       []string  `json:"packs,omitempty"`
	RatePeriods []string  `json:"rate_periods,omitempty"`
}

// Policy is a booking policy of an establishment (table booking_policies).
type Policy struct {
	ID       string `json:"id"`
	Scope    Scope  `json:"scope"`
	Rules    Rules  `json:"rules"`
	Priority int    `json:"priority"`
}

// Ptr returns a pointer to v. Handy to build Rules literals.
func Ptr[T any](v T) *T { return &v }
