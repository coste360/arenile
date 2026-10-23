package notify

// M5B customer templates (spec 19: renewal proposal, waitlist offer). The
// renewal and waitlist builders add the template files (templates/<lang>/
// <code>.*, it/en/de/fr) and append the code to Templates when the files
// exist; the compose step issues the magic link of BookingID like the
// booking templates (renewal) or uses Payload["waitlist_url"] (waitlist,
// the raw token never enters message_log: pass it only to the renderer).
const (
	TemplateRenewalProposal = "renewal_proposal"
	TemplateRenewalReminder = "renewal_reminder"
	TemplateWaitlistJoined  = "waitlist_joined"
	TemplateWaitlistOffer   = "waitlist_offer"
)
