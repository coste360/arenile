-- ESC/POS network printers and print jobs (SQLite dialect of postgres/00011; ADR 0021).
--   * printers: network ESC/POS printers (raw TCP, default port 9100) reached
--     by the server of the desktop edition. driver is 'network' only in v1;
--     a future bridge driver (desktop client for the VPS, mobile Bluetooth)
--     adds a value to the CHECK in a new migration. host is a private/LAN
--     address (checked by internal/domain/printing). logo_raster caches the
--     dithered GS v 0 raster of the establishment logo for this printer
--     width; logo_source_sha256 invalidates it when the logo changes.
--     status/last_* are the outcome of the last check or job.
--   * printer_routes: which printer prints which document, per cash register
--     (register_id NULL = default rule of the document). At most one rule
--     per (establishment, document_kind, register) via two partial unique
--     indexes (NULL is not equal to NULL in a plain unique key).
--   * print_jobs: rendered ESC/POS payload queued for a printer, sent by the
--     in-process job runner with retries and backoff. idempotency_key is
--     unique per establishment: the same print request never creates two
--     jobs; the claim (queued -> printing) is a conditional update with a
--     lease (locked_until), so a job is sent by one worker at a time (an
--     expired lease is reclaimed: at-least-once delivery). payload is nulled
--     30 days after the job ends (job printing.retention, the row stays as
--     history); last_error never contains personal data.
-- 00010 is reserved by the daily sale branch.

-- +goose Up
CREATE TABLE printers (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    name               TEXT NOT NULL,
    driver             TEXT NOT NULL DEFAULT 'network' CHECK (driver IN ('network')),
    host               TEXT NOT NULL,
    port               INTEGER NOT NULL DEFAULT 9100 CHECK (port BETWEEN 1 AND 65535),
    paper_width_mm     INTEGER NOT NULL DEFAULT 80 CHECK (paper_width_mm IN (58, 80)),
    chars_per_line     INTEGER NOT NULL DEFAULT 48 CHECK (chars_per_line BETWEEN 24 AND 64),
    codepage           TEXT NOT NULL DEFAULT 'cp858'
                           CHECK (codepage IN ('cp858', 'cp437', 'cp850', 'cp852', 'cp1252')),
    codepage_table     INTEGER CHECK (codepage_table IS NULL OR codepage_table BETWEEN 0 AND 255),
    cut_mode           TEXT NOT NULL DEFAULT 'partial' CHECK (cut_mode IN ('full', 'partial', 'none')),
    feed_lines         INTEGER NOT NULL DEFAULT 3 CHECK (feed_lines BETWEEN 0 AND 10),
    print_logo         INTEGER NOT NULL DEFAULT 0 CHECK (print_logo IN (0, 1)),
    logo_raster        BLOB,
    logo_source_sha256 TEXT,
    drawer_pin         INTEGER CHECK (drawer_pin IS NULL OR drawer_pin IN (2, 5)),
    active             INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    status             TEXT NOT NULL DEFAULT 'unknown' CHECK (status IN ('unknown', 'reachable', 'unreachable')),
    last_checked_at    TIMESTAMP,
    last_success_at    TIMESTAMP,
    last_error         TEXT,
    version            INTEGER NOT NULL DEFAULT 1,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX printers_establishment_name_uidx ON printers (establishment_id, name);

CREATE TABLE printer_routes (
    id               TEXT PRIMARY KEY,
    establishment_id TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    document_kind    TEXT NOT NULL CHECK (document_kind IN ('daily_ticket', 'cash_z', 'payment_receipt', 'test_page')),
    register_id      TEXT REFERENCES cash_registers (id) ON DELETE CASCADE,
    printer_id       TEXT NOT NULL REFERENCES printers (id) ON DELETE CASCADE,
    copies           INTEGER NOT NULL DEFAULT 1 CHECK (copies BETWEEN 1 AND 3),
    kick_drawer      INTEGER NOT NULL DEFAULT 0 CHECK (kick_drawer IN (0, 1)),
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX printer_routes_default_uidx ON printer_routes (establishment_id, document_kind)
    WHERE register_id IS NULL;
CREATE UNIQUE INDEX printer_routes_register_uidx ON printer_routes (establishment_id, document_kind, register_id)
    WHERE register_id IS NOT NULL;
CREATE INDEX printer_routes_printer_idx ON printer_routes (printer_id);

CREATE TABLE print_jobs (
    id                 TEXT PRIMARY KEY,
    establishment_id   TEXT NOT NULL REFERENCES establishments (id) ON DELETE CASCADE,
    printer_id         TEXT REFERENCES printers (id) ON DELETE SET NULL,
    document_kind      TEXT NOT NULL CHECK (document_kind IN ('daily_ticket', 'cash_z', 'payment_receipt', 'test_page')),
    document_id        TEXT,
    register_id        TEXT REFERENCES cash_registers (id) ON DELETE SET NULL,
    copies             INTEGER NOT NULL DEFAULT 1 CHECK (copies BETWEEN 1 AND 3),
    status             TEXT NOT NULL DEFAULT 'queued'
                           CHECK (status IN ('queued', 'printing', 'printed', 'failed', 'cancelled')),
    idempotency_key    TEXT NOT NULL,
    payload            BLOB,
    attempts           INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts       INTEGER NOT NULL DEFAULT 5 CHECK (max_attempts >= 1),
    next_attempt_at    TIMESTAMP,
    locked_until       TIMESTAMP,
    last_error         TEXT,
    created_by_user_id TEXT REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL,
    printed_at         TIMESTAMP,
    CHECK ((status = 'printed') = (printed_at IS NOT NULL)),
    CHECK (document_kind = 'test_page' OR document_id IS NOT NULL)
);
CREATE UNIQUE INDEX print_jobs_idempotency_uidx ON print_jobs (establishment_id, idempotency_key);
CREATE INDEX print_jobs_establishment_created_idx ON print_jobs (establishment_id, created_at);
CREATE INDEX print_jobs_due_idx ON print_jobs (next_attempt_at) WHERE status IN ('queued', 'printing');
CREATE INDEX print_jobs_printer_idx ON print_jobs (printer_id);

-- +goose Down
DROP TABLE print_jobs;
DROP TABLE printer_routes;
DROP TABLE printers;
