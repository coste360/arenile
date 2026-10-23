package rollover

import "errors"

// ErrTargetExists: a season of the target year exists and was not created
// by a rollover of this source season (409 season_rollover_conflict).
var ErrTargetExists = errors.New("rollover: target season already exists")
