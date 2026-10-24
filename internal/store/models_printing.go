package store

import (
	"time"

	"github.com/uptrace/bun"
)

// Models of ESC/POS network printing (migration 00011, ADR 0021).
// Ownership: internal/domain/printing (service, job runner, network driver);
// repositories go in internal/store/printing.go.

// Printer drivers (api PrinterDriver). v1 has only the network driver.
const PrinterDriverNetwork = "network"

// Printer statuses (api PrinterStatus).
const (
	PrinterStatusUnknown     = "unknown"
	PrinterStatusReachable   = "reachable"
	PrinterStatusUnreachable = "unreachable"
)

// Printer cut modes (api PrinterCutMode).
const (
	PrinterCutFull    = "full"
	PrinterCutPartial = "partial"
	PrinterCutNone    = "none"
)

// DefaultPrinterPort is the raw TCP (JetDirect) port of network printers.
const DefaultPrinterPort = 9100

// Printer is a network ESC/POS printer of an establishment. Host is a
// private/LAN address (validated by the domain service). LogoRaster caches
// the dithered GS v 0 raster of the establishment logo for this paper width;
// LogoSourceSHA256 is the hash of the source logo it was made from.
type Printer struct {
	bun.BaseModel `bun:"table:printers,alias:prn"`

	ID               string     `bun:"id,pk"`
	EstablishmentID  string     `bun:"establishment_id,notnull"`
	Name             string     `bun:"name,notnull"`
	Driver           string     `bun:"driver,notnull"`
	Host             string     `bun:"host,notnull"`
	Port             int        `bun:"port,notnull"`
	PaperWidthMM     int        `bun:"paper_width_mm,notnull"`
	CharsPerLine     int        `bun:"chars_per_line,notnull"`
	Codepage         string     `bun:"codepage,notnull"`
	CodepageTable    *int       `bun:"codepage_table"`
	CutMode          string     `bun:"cut_mode,notnull"`
	FeedLines        int        `bun:"feed_lines,notnull"`
	PrintLogo        bool       `bun:"print_logo,notnull"`
	LogoRaster       []byte     `bun:"logo_raster,nullzero"`
	LogoSourceSHA256 *string    `bun:"logo_source_sha256"`
	DrawerPin        *int       `bun:"drawer_pin"`
	Active           bool       `bun:"active,notnull"`
	Status           string     `bun:"status,notnull"`
	LastCheckedAt    *time.Time `bun:"last_checked_at"`
	LastSuccessAt    *time.Time `bun:"last_success_at"`
	LastError        *string    `bun:"last_error"`
	Version          int64      `bun:"version,notnull"`
	CreatedAt        time.Time  `bun:"created_at,notnull"`
	UpdatedAt        time.Time  `bun:"updated_at,notnull"`
}

// Print document kinds (api PrintDocumentKind).
const (
	PrintDocDailyTicket    = "daily_ticket"
	PrintDocCashZ          = "cash_z"
	PrintDocPaymentReceipt = "payment_receipt"
	PrintDocTestPage       = "test_page"
)

// PrinterRoute sends a document kind to a printer, for one cash register or,
// with RegisterID nil, as the default rule of the document. Unique per
// (establishment, document_kind, register) through two partial indexes.
type PrinterRoute struct {
	bun.BaseModel `bun:"table:printer_routes,alias:prr"`

	ID              string    `bun:"id,pk"`
	EstablishmentID string    `bun:"establishment_id,notnull"`
	DocumentKind    string    `bun:"document_kind,notnull"`
	RegisterID      *string   `bun:"register_id"`
	PrinterID       string    `bun:"printer_id,notnull"`
	Copies          int       `bun:"copies,notnull"`
	KickDrawer      bool      `bun:"kick_drawer,notnull"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
}

// Print job statuses (api PrintJobStatus).
const (
	PrintJobQueued    = "queued"
	PrintJobPrinting  = "printing"
	PrintJobPrinted   = "printed"
	PrintJobFailed    = "failed"
	PrintJobCancelled = "cancelled"
)

// DefaultPrintJobMaxAttempts is the default number of send attempts.
const DefaultPrintJobMaxAttempts = 5

// PrintJob is a rendered ESC/POS payload queued for a printer. The unique
// key (establishment_id, idempotency_key) makes enqueueing idempotent; the
// claim queued -> printing is a conditional update guarded by LockedUntil.
// LastError never contains personal data; Payload may be purged after
// printing.
type PrintJob struct {
	bun.BaseModel `bun:"table:print_jobs,alias:pj"`

	ID              string     `bun:"id,pk"`
	EstablishmentID string     `bun:"establishment_id,notnull"`
	PrinterID       *string    `bun:"printer_id"`
	DocumentKind    string     `bun:"document_kind,notnull"`
	DocumentID      *string    `bun:"document_id"`
	RegisterID      *string    `bun:"register_id"`
	Copies          int        `bun:"copies,notnull"`
	Status          string     `bun:"status,notnull"`
	IdempotencyKey  string     `bun:"idempotency_key,notnull"`
	Payload         []byte     `bun:"payload,nullzero"`
	Attempts        int        `bun:"attempts,notnull"`
	MaxAttempts     int        `bun:"max_attempts,notnull"`
	NextAttemptAt   *time.Time `bun:"next_attempt_at"`
	LockedUntil     *time.Time `bun:"locked_until"`
	LastError       *string    `bun:"last_error"`
	CreatedByUserID *string    `bun:"created_by_user_id"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	UpdatedAt       time.Time  `bun:"updated_at,notnull"`
	PrintedAt       *time.Time `bun:"printed_at"`
}
