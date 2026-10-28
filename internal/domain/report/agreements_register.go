package report

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Agreements register for the concession tenders (spec 30.2.5, ADR 0028
// decision 3): neutral layout, Excel and a printable page (PDF through the
// browser). For each agreement active in the period: conditions in
// readable Italian, bookings, spot-days, people, revenue and the discount
// granted; totals per register category and overall.

// RegisterCounters are the counters of a register entry or category.
type RegisterCounters struct {
	Agreements int
	Bookings   int
	SpotDays   int
	People     int
	Revenue    money.Cents
	Discount   money.Cents
}

func (c *RegisterCounters) add(o RegisterCounters) {
	c.Agreements += o.Agreements
	c.Bookings += o.Bookings
	c.SpotDays += o.SpotDays
	c.People += o.People
	c.Revenue += o.Revenue
	c.Discount += o.Discount
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

// RegisterCategoryOrder is the order of the categories in the register.
var RegisterCategoryOrder = []string{
	store.RegisterResidents, store.RegisterAccommodation, store.RegisterProtected,
	store.RegisterCompanies, store.RegisterAssociations, store.RegisterOther,
}

// RegisterCategoryLabels are the Italian labels of the register categories.
var RegisterCategoryLabels = map[string]string{
	store.RegisterResidents: "Residenti", store.RegisterAccommodation: "Strutture ricettive",
	store.RegisterProtected: "Categorie tutelate", store.RegisterCompanies: "Aziende",
	store.RegisterAssociations: "Associazioni", store.RegisterOther: "Altro",
}

// RegisterInput is everything BuildAgreementsRegister needs (pure).
type RegisterInput struct {
	Period      Period
	SeasonIDs   map[string]bool // agreements restricted to other seasons are left out ("" key ignored)
	SeasonID    string
	Agreements  []store.Agreement
	Partners    map[string]store.Partner
	Categories  map[string]store.CustomerCategory
	PackNames   map[string]string
	Bookings    []store.M8ReportBooking
	GeneratedAt time.Time
}

// DeriveRegisterCategory is the register category of an agreement: its own
// one, else the category of its customer category, else from the kind of
// its partner, else other.
func DeriveRegisterCategory(a *store.Agreement, partners map[string]store.Partner, cats map[string]store.CustomerCategory) string {
	if a.RegisterCategory != nil && *a.RegisterCategory != "" {
		return *a.RegisterCategory
	}
	if a.CustomerCategoryID != nil {
		if c, ok := cats[*a.CustomerCategoryID]; ok && c.RegisterCategory != "" {
			return c.RegisterCategory
		}
	}
	if a.PartnerID != nil {
		switch partners[*a.PartnerID].Kind {
		case store.PartnerKindAccommodation:
			return store.RegisterAccommodation
		case store.PartnerKindCompany:
			return store.RegisterCompanies
		case store.PartnerKindAssociation:
			return store.RegisterAssociations
		}
	}
	return store.RegisterOther
}

// BuildAgreementsRegister is the pure builder of the register. An
// agreement is listed when its validity overlaps the period, it is in the
// season (when one is given) and it is active, or when it has bookings in
// the period anyway.
func BuildAgreementsRegister(in RegisterInput) *AgreementsRegister {
	reg := &AgreementsRegister{Period: in.Period, SeasonID: in.SeasonID, GeneratedAt: in.GeneratedAt, Entries: []RegisterEntry{}}
	counters := map[string]*RegisterCounters{}
	for i := range in.Bookings {
		b := &in.Bookings[i]
		if b.AgreementID == nil {
			continue
		}
		c := counters[*b.AgreementID]
		if c == nil {
			c = &RegisterCounters{}
			counters[*b.AgreementID] = c
		}
		c.Bookings++
		c.People += people(b)
		c.SpotDays += spotDays(b, in.Period)
		c.Revenue += b.TotalAmount
		c.Discount += AgreementDiscount(b.Price())
	}
	byCat := map[string]*RegisterCounters{}
	for i := range in.Agreements {
		a := &in.Agreements[i]
		c := counters[a.ID]
		if c == nil {
			if !a.Active || !validityOverlaps(a, in.Period) || !inSeason(a, in.SeasonID) {
				continue
			}
			c = &RegisterCounters{}
		}
		c.Agreements = 1
		e := RegisterEntry{
			AgreementID: a.ID, Name: a.Name, Target: store.AgreementTargetCustomers,
			RegisterCategory: DeriveRegisterCategory(a, in.Partners, in.Categories),
			ValidFrom:        a.ValidFrom, ValidTo: a.ValidTo, RegisterCounters: *c,
		}
		if a.Code != nil {
			e.Code = *a.Code
		}
		if a.Target != nil {
			e.Target = *a.Target
		}
		e.PartnerName = a.PartnerName
		if a.PartnerID != nil {
			if p, ok := in.Partners[*a.PartnerID]; ok {
				e.PartnerName = p.Name
			}
		}
		if a.CustomerCategoryID != nil {
			e.CustomerCategoryName = in.Categories[*a.CustomerCategoryID].Name
		}
		e.ConditionsText = ConditionsText(a, in.PackNames)
		reg.Entries = append(reg.Entries, e)
		t := byCat[e.RegisterCategory]
		if t == nil {
			t = &RegisterCounters{}
			byCat[e.RegisterCategory] = t
		}
		t.add(e.RegisterCounters)
		reg.Totals.add(e.RegisterCounters)
	}
	rank := map[string]int{}
	for i, c := range RegisterCategoryOrder {
		rank[c] = i
	}
	sort.SliceStable(reg.Entries, func(i, j int) bool {
		a, b := reg.Entries[i], reg.Entries[j]
		if rank[a.RegisterCategory] != rank[b.RegisterCategory] {
			return rank[a.RegisterCategory] < rank[b.RegisterCategory]
		}
		return a.Name < b.Name
	})
	for _, c := range RegisterCategoryOrder {
		if t, ok := byCat[c]; ok {
			reg.ByCategory = append(reg.ByCategory, RegisterCategoryTotal{RegisterCategory: c, RegisterCounters: *t})
		}
	}
	if reg.ByCategory == nil {
		reg.ByCategory = []RegisterCategoryTotal{}
	}
	return reg
}

func validityOverlaps(a *store.Agreement, p Period) bool {
	if a.ValidFrom != nil && a.ValidFrom.After(p.To) {
		return false
	}
	return a.ValidTo == nil || !a.ValidTo.Before(p.From)
}

func inSeason(a *store.Agreement, seasonID string) bool {
	if seasonID == "" || len(a.SeasonIDs) == 0 {
		return true
	}
	var ids []string
	if err := json.Unmarshal(a.SeasonIDs, &ids); err != nil || len(ids) == 0 {
		return true
	}
	for _, id := range ids {
		if id == seasonID {
			return true
		}
	}
	return false
}

// conditionsDoc is the part of the agreement columns the text needs.
type conditionsDoc struct {
	Action     pricing.Action               `json:"-"`
	PackPrices []pricing.AgreementPackPrice `json:"-"`
	Scope      *pricing.AgreementScope      `json:"-"`
	Quota      *struct {
		MaxPerDay         *int `json:"max_per_day"`
		ReservedPerDay    *int `json:"reserved_per_day"`
		ReleaseDaysBefore *int `json:"release_days_before"`
	} `json:"-"`
	Channels []string `json:"-"`
}

var perLabels = map[pricing.AmountPer]string{
	pricing.PerDay: "al giorno", pricing.PerLine: "per postazione", pricing.PerBooking: "per prenotazione",
}

var agreementChannelLabels = map[string]string{
	"desk": "cassa", "online_code": "online con codice", "online_link": "link della convenzione",
}

// ConditionsText describes the conditions of an agreement in readable
// Italian (economic condition, scope, validity, quota, requirement,
// channels) for the register.
func ConditionsText(a *store.Agreement, packNames map[string]string) string {
	var d conditionsDoc
	_ = json.Unmarshal(a.Action, &d.Action)
	if len(a.PackPrices) > 0 {
		_ = json.Unmarshal(a.PackPrices, &d.PackPrices)
	}
	if len(a.Scope) > 0 {
		_ = json.Unmarshal(a.Scope, &d.Scope)
	}
	if len(a.Quota) > 0 {
		_ = json.Unmarshal(a.Quota, &d.Quota)
	}
	if len(a.Channels) > 0 {
		_ = json.Unmarshal(a.Channels, &d.Channels)
	}
	var parts []string
	for _, pp := range d.PackPrices {
		name := packNames[pp.PackID]
		if name == "" {
			name = "pack"
		}
		per := pp.Per
		if per == "" {
			per = pricing.PerDay
		}
		parts = append(parts, fmt.Sprintf("Prezzo fisso %s: %s € %s", name, eur(pp.Amount), perLabels[per]))
	}
	if t := actionText(d.Action); t != "" {
		if len(d.PackPrices) > 0 {
			t += " sugli altri pack"
		}
		parts = append(parts, t)
	}
	if s := d.Scope; s != nil {
		if len(s.PackIDs) > 0 {
			names := make([]string, 0, len(s.PackIDs))
			for _, id := range s.PackIDs {
				if n := packNames[id]; n != "" {
					names = append(names, n)
				}
			}
			sort.Strings(names)
			parts = append(parts, "Solo per: "+strings.Join(names, ", "))
		}
		if s.ApplyToExtras {
			parts = append(parts, "Anche sugli extra")
		} else {
			parts = append(parts, "Extra esclusi")
		}
	}
	switch {
	case a.ValidFrom != nil && a.ValidTo != nil:
		parts = append(parts, fmt.Sprintf("Valida dal %s al %s", dayLabel(*a.ValidFrom), dayLabel(*a.ValidTo)))
	case a.ValidFrom != nil:
		parts = append(parts, "Valida dal "+dayLabel(*a.ValidFrom))
	case a.ValidTo != nil:
		parts = append(parts, "Valida fino al "+dayLabel(*a.ValidTo))
	}
	if q := d.Quota; q != nil {
		if q.MaxPerDay != nil {
			parts = append(parts, fmt.Sprintf("Massimo %d postazioni per mezza giornata", *q.MaxPerDay))
		}
		if q.ReservedPerDay != nil && *q.ReservedPerDay > 0 {
			days := 3
			if q.ReleaseDaysBefore != nil {
				days = *q.ReleaseDaysBefore
			}
			parts = append(parts, fmt.Sprintf("%d postazioni riservate fino a %d giorni prima", *q.ReservedPerDay, days))
		}
	}
	if a.Requirement != "" {
		parts = append(parts, "Requisito: "+a.Requirement)
	}
	if len(d.Channels) > 0 {
		cs := make([]string, 0, len(d.Channels))
		for _, c := range d.Channels {
			cs = append(cs, labelOr(agreementChannelLabels, c))
		}
		parts = append(parts, "Canali: "+strings.Join(cs, ", "))
	}
	return strings.Join(parts, ". ")
}

func actionText(a pricing.Action) string {
	switch a.Type {
	case pricing.ActionPercentOff:
		if a.Percent == nil {
			return ""
		}
		if bp, _ := a.Percent.BasisPoints(); bp == 0 {
			return ""
		}
		return "Sconto del " + strings.ReplaceAll(a.Percent.String(), ".", ",") + "%"
	case pricing.ActionAmountOff:
		if a.Amount == 0 {
			return ""
		}
		per := a.Per
		if per == "" {
			per = pricing.PerBooking
		}
		return fmt.Sprintf("Sconto di %s € %s", eur(a.Amount), perLabels[per])
	case pricing.ActionMultiply:
		if a.Factor == nil {
			return ""
		}
		return "Prezzo moltiplicato per " + strings.ReplaceAll(a.Factor.String(), ".", ",")
	case pricing.ActionFreeDays:
		return fmt.Sprintf("Ogni %d giorni, %d gratis", a.Every, a.Free)
	}
	return ""
}

// AgreementsRegister returns the register of the agreements active in the
// period (seasonID optional, for the header). Requires
// auth.PermReportsRead.
func (s *Service) AgreementsRegister(ctx context.Context, p *auth.Principal, establishmentID string, period Period, seasonID string) (*AgreementsRegister, error) {
	e, err := s.open(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	if err := validPeriod(period); err != nil {
		return nil, err
	}
	db := s.d.DB
	est, err := store.GetEstablishment(ctx, db, e.es)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	year := 0
	if seasonID != "" {
		sn, err := store.PricingGetSeason(ctx, db, e.es, seasonID)
		if err != nil {
			return nil, notFoundOr(err)
		}
		year = sn.Year
	}
	ags, err := store.ListAllAgreements(ctx, db, e.es)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	partners, err := store.ListAllPartners(ctx, db, e.es)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	cats, err := store.ListCustomerCategories(ctx, db, e.es, nil)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	packs, err := store.ListAllPacks(ctx, db, e.es)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	rows, err := store.ReportM8Bookings(ctx, db, e.es, store.M8BookingFilter{From: period.From, To: period.To, WithAgreement: true})
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	in := RegisterInput{
		Period: period, SeasonID: seasonID, Agreements: ags, Bookings: rows, GeneratedAt: s.d.Clock.Now().UTC(),
		Partners: map[string]store.Partner{}, Categories: map[string]store.CustomerCategory{}, PackNames: map[string]string{},
	}
	for _, pa := range partners {
		in.Partners[pa.ID] = pa
	}
	for _, c := range cats {
		in.Categories[c.ID] = c
	}
	for _, pk := range packs {
		in.PackNames[pk.ID] = packName(pk)
	}
	reg := BuildAgreementsRegister(in)
	reg.EstablishmentName, reg.SeasonYear = est.Name, year
	if org, err := store.GetOrganization(ctx, db, est.OrganizationID); err == nil {
		if org.Name != est.Name {
			reg.EstablishmentLegalName = org.Name
		}
		if org.VATNumber != nil {
			reg.EstablishmentVATNumber = *org.VATNumber
		}
	}
	return reg, nil
}

// packName is the Italian name of a pack, else its code.
func packName(p store.Pack) string {
	if n := strings.TrimSpace(p.Name["it"]); n != "" {
		return n
	}
	for _, n := range p.Name {
		if n = strings.TrimSpace(n); n != "" {
			return n
		}
	}
	return p.Code
}

// WriteAgreementsRegisterCSV writes the register (the xlsx is the same
// table converted).
func WriteAgreementsRegisterCSV(w io.Writer, r *AgreementsRegister) error {
	c := NewCSVWriter(w)
	c.Row("Categoria", "Convenzione", "Codice", "Destinatari", "Condizioni", "Dal", "Al",
		"Prenotazioni", "Giorni-postazione", "Persone", "Incasso", "Sconto concesso") //nolint:misspell // Italian
	for _, e := range r.Entries {
		var from, to string
		if e.ValidFrom != nil {
			from = dateCell(*e.ValidFrom)
		}
		if e.ValidTo != nil {
			to = dateCell(*e.ValidTo)
		}
		c.Row(labelOr(RegisterCategoryLabels, e.RegisterCategory), text(e.Name), text(e.Code), text(targetLabel(e)),
			text(e.ConditionsText), from, to, itoa(e.Bookings), itoa(e.SpotDays), itoa(e.People), eur(e.Revenue), eur(e.Discount))
	}
	for _, t := range r.ByCategory {
		c.Row("Totale "+labelOr(RegisterCategoryLabels, t.RegisterCategory), itoa(t.Agreements)+" convenzioni", "", "", "", "", "",
			itoa(t.Bookings), itoa(t.SpotDays), itoa(t.People), eur(t.Revenue), eur(t.Discount))
	}
	t := r.Totals
	c.Row("Totale generale", itoa(t.Agreements)+" convenzioni", "", "", "", "", "",
		itoa(t.Bookings), itoa(t.SpotDays), itoa(t.People), eur(t.Revenue), eur(t.Discount))
	return c.Flush()
}

// targetLabel describes the recipients of an entry.
func targetLabel(e RegisterEntry) string {
	switch e.Target {
	case store.AgreementTargetPartner:
		return "Ospiti di " + e.PartnerName
	case store.AgreementTargetCategory:
		return e.CustomerCategoryName
	}
	if e.PartnerName != "" {
		return e.PartnerName
	}
	return "Singoli clienti"
}

// TargetLabel is targetLabel for the printout.
func TargetLabel(e RegisterEntry) string { return targetLabel(e) }

// PrintContext is what a printed M8 document needs from the
// establishment: header data, currency, timezone and default locale.
type PrintContext struct {
	Establishment store.Establishment
	Organization  *store.Organization
	Location      *time.Location
}

// PrintContext returns the establishment data of the printed documents.
// Requires auth.PermReportsRead.
func (s *Service) PrintContext(ctx context.Context, p *auth.Principal, establishmentID string) (*PrintContext, error) {
	e, err := s.open(ctx, p, establishmentID)
	if err != nil {
		return nil, err
	}
	est, err := store.GetEstablishment(ctx, s.d.DB, e.es)
	if err != nil {
		return nil, fmt.Errorf("report: %w", err)
	}
	out := &PrintContext{Establishment: *est, Location: e.loc}
	if org, err := store.GetOrganization(ctx, s.d.DB, est.OrganizationID); err == nil {
		out.Organization = org
	}
	return out, nil
}
