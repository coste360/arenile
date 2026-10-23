package event

// M5B event types (spec 18.1 extended; API enum WebhookEventType). Payloads
// carry ids, dates and cents only (no personal data); version 1.
const (
	// SeasonRenewalAccepted: proposal accepted (online: payment started;
	// panel). Data: proposal_id, campaign_id, booking_id, source_pass_id, channel.
	SeasonRenewalAccepted = "season.renewal_accepted"
	// SeasonRenewalDeclined: proposal declined, spot released.
	SeasonRenewalDeclined = "season.renewal_declined"
	// SeasonRenewalExpired: deadline passed or campaign closed, spot released.
	SeasonRenewalExpired = "season.renewal_expired"
	// SeasonRolledOver: season rollover applied. Data: rollover_id,
	// source_season_id, target_season_id, target_year, counts.
	SeasonRolledOver = "season.rolled_over"
	// WaitlistJoined: new waitlist entry. Data: entry_id, channel, dates, pack_id.
	WaitlistJoined = "waitlist.joined"
	// WaitlistOffered: offer created with a hold. Data: entry_id, offer_id,
	// hold_id, cause, expires_at.
	WaitlistOffered = "waitlist.offered"
	// WaitlistOfferExpired: offer expired, declined or revoked (data.reason).
	WaitlistOfferExpired = "waitlist.offer_expired"
	// WaitlistConverted: offer accepted, booking created. Data: entry_id,
	// offer_id, booking_id.
	WaitlistConverted = "waitlist.converted"
	// CreditIssued: positive ledger movement. Data: credit_id, customer_id,
	// amount, source, expires_on, booking_id.
	CreditIssued = "credit.issued"
	// CreditConsumed: redemption (payment method credit). Data: credit_id,
	// customer_id, amount, booking_id, payment_id.
	CreditConsumed = "credit.consumed"
	// CreditExpired: expiry row written. Data: credit_id, customer_id,
	// amount, reference_credit_id.
	CreditExpired = "credit.expired"
)
