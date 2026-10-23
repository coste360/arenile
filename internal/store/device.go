package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

// Repository functions of the paired devices (spec 29.2, migration 00006),
// owned by internal/domain/device. Every operational function takes an
// EstablishmentScope; the two lookups used before a scope exists
// (ConsumePairingCode, GetActiveDeviceByTokenHash) are system operations
// where the secret itself proves the scope.

// InsertPairingCode inserts a pairing code of the establishment of es. A
// code_hash collision is ErrConflict (the caller draws a new code).
func InsertPairingCode(ctx context.Context, db bun.IDB, es EstablishmentScope, c *DevicePairingCode) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	c.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(c).Exec(ctx)
	return wrapWrite("insert pairing code", err)
}

// ConsumePairingCode marks the code with codeHash as used, if it is unused
// and not expired at now, and returns it. The conditional UPDATE is the
// single-use guarantee: of concurrent callers exactly one sees a row, the
// others get ErrNotFound (as for unknown or expired codes).
func ConsumePairingCode(ctx context.Context, db bun.IDB, codeHash string, now time.Time) (*DevicePairingCode, error) {
	res, err := db.NewUpdate().Model((*DevicePairingCode)(nil)).
		Set("used_at = ?", now).
		Where("code_hash = ?", codeHash).Where("used_at IS NULL").Where("expires_at > ?", now).
		Exec(ctx)
	if err != nil {
		return nil, wrapWrite("consume pairing code", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, ErrNotFound
	}
	c := new(DevicePairingCode)
	err = db.NewSelect().Model(c).Where("code_hash = ?", codeHash).Scan(ctx)
	return c, wrapRead("get pairing code", err)
}

// SetPairingCodeDevice records the device created by a consumed code.
func SetPairingCodeDevice(ctx context.Context, db bun.IDB, codeID, deviceID string) error {
	_, err := db.NewUpdate().Model((*DevicePairingCode)(nil)).Set("device_id = ?", deviceID).Where("id = ?", codeID).Exec(ctx)
	return wrapWrite("set pairing code device", err)
}

// DeleteExpiredPairingCodes deletes the codes that expired before cutoff.
func DeleteExpiredPairingCodes(ctx context.Context, db bun.IDB, cutoff time.Time) (int64, error) {
	res, err := db.NewDelete().Model((*DevicePairingCode)(nil)).Where("expires_at < ?", cutoff).Exec(ctx)
	if err != nil {
		return 0, wrapWrite("delete expired pairing codes", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// InsertDevice inserts a paired device of the establishment of es.
func InsertDevice(ctx context.Context, db bun.IDB, es EstablishmentScope, d *Device) error {
	if err := es.mustBeValid(); err != nil {
		return err
	}
	d.EstablishmentID = es.ID()
	_, err := db.NewInsert().Model(d).Exec(ctx)
	return wrapWrite("insert device", err)
}

// GetActiveDeviceByTokenHash loads a non-revoked device by the hash of its
// token (authentication: the token proves the scope).
func GetActiveDeviceByTokenHash(ctx context.Context, db bun.IDB, tokenHash string) (*Device, error) {
	d := new(Device)
	err := db.NewSelect().Model(d).Where("token_hash = ?", tokenHash).Where("revoked_at IS NULL").Scan(ctx)
	return d, wrapRead("get device by token", err)
}

// GetDevice loads a device (revoked or not) of the establishment of es.
func GetDevice(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Device, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	d := new(Device)
	err := db.NewSelect().Model(d).Where("id = ?", id).Where("establishment_id = ?", es.ID()).Scan(ctx)
	return d, wrapRead("get device", err)
}

// TouchDevice sets last_used_at = at unless it was set after notBefore
// (conditional, so concurrent requests of one device write at most once
// per interval). It reports whether a row was written.
func TouchDevice(ctx context.Context, db bun.IDB, id string, at, notBefore time.Time) (bool, error) {
	res, err := db.NewUpdate().Model((*Device)(nil)).Set("last_used_at = ?", at).
		Where("id = ?", id).
		WhereGroup(" AND ", func(q *bun.UpdateQuery) *bun.UpdateQuery {
			return q.Where("last_used_at IS NULL").WhereOr("last_used_at < ?", notBefore)
		}).Exec(ctx)
	if err != nil {
		return false, wrapWrite("touch device", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// RevokeDevice sets revoked_at on a device of the establishment of es that
// is not revoked yet. It reports whether this call revoked it (false: it
// was already revoked); ErrNotFound for an unknown device.
func RevokeDevice(ctx context.Context, db bun.IDB, es EstablishmentScope, id string, at time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*Device)(nil)).Set("revoked_at = ?", at).Set("updated_at = ?", at).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).Where("revoked_at IS NULL").Exec(ctx)
	if err != nil {
		return false, wrapWrite("revoke device", err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return true, nil
	}
	if _, err := GetDevice(ctx, db, es, id); err != nil {
		return false, err
	}
	return false, nil
}

// DeviceRow is a device with the name of its user and whether it is the
// active fiscal bridge of the establishment (listing).
type DeviceRow struct {
	Device       `bun:",extend"`
	UserName     string `bun:"user_name"`
	FiscalBridge bool   `bun:"fiscal_bridge"`
}

// DeviceFilter filters ListDevices. UserID empty = every user.
type DeviceFilter struct {
	UserID         string
	DeviceID       string
	IncludeRevoked bool
}

// MaxDevicesPerListing bounds ListDevices: an establishment has tens of
// devices, the domain sorts and paginates them in memory.
const MaxDevicesPerListing = 2000

// ListDevices lists the devices of the establishment of es matching f, by
// id, at most MaxDevicesPerListing.
func ListDevices(ctx context.Context, db bun.IDB, es EstablishmentScope, f DeviceFilter) ([]DeviceRow, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	bridge := db.NewSelect().TableExpr("fiscal_bridges AS fb").ColumnExpr("1").
		Where("fb.device_id = dev.id").Where("fb.active = ?", true)
	var rows []DeviceRow
	q := db.NewSelect().Model(&rows).
		ColumnExpr("dev.*").
		ColumnExpr("u.name AS user_name").
		ColumnExpr("EXISTS (?) AS fiscal_bridge", bridge).
		Join("JOIN users AS u ON u.id = dev.user_id").
		Where("dev.establishment_id = ?", es.ID()).
		OrderExpr("dev.id ASC").Limit(MaxDevicesPerListing)
	if f.UserID != "" {
		q = q.Where("dev.user_id = ?", f.UserID)
	}
	if f.DeviceID != "" {
		q = q.Where("dev.id = ?", f.DeviceID)
	}
	if !f.IncludeRevoked {
		q = q.Where("dev.revoked_at IS NULL")
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: list devices: %w", err)
	}
	return rows, nil
}
