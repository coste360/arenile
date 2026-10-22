package policy

import (
	"encoding/json"
	"fmt"
	"time"
)

// SnapshotVersion is the current schema version of Snapshot. Bump it when the
// meaning of a stored snapshot would change.
const SnapshotVersion = 1

// Snapshot is the content of bookings.policy_snapshot: the effective rules at
// booking time, so that cancellations and refunds stay verifiable even after
// the manager changes the policies (spec 9).
type Snapshot struct {
	Version    int       `json:"version"`
	PolicyID   string    `json:"policy_id,omitempty"` // empty = establishment default
	Priority   int       `json:"priority"`
	Channel    Channel   `json:"channel,omitempty"`
	Rules      Rules     `json:"rules"`
	ResolvedAt time.Time `json:"resolved_at"`
}

// NewSnapshot builds a snapshot from a resolution.
func NewSnapshot(res Resolution, ch Channel, resolvedAt time.Time) Snapshot {
	return Snapshot{
		Version:    SnapshotVersion,
		PolicyID:   res.PolicyID,
		Priority:   res.Priority,
		Channel:    ch,
		Rules:      res.Rules.Clone(),
		ResolvedAt: resolvedAt.UTC(),
	}
}

// Marshal encodes the snapshot as JSON.
func (s Snapshot) Marshal() ([]byte, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("policy: marshal snapshot: %w", err)
	}
	return b, nil
}

// ParseSnapshot decodes a stored snapshot. Unknown versions are rejected.
func ParseSnapshot(b []byte) (Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %w", ErrInvalidSnapshot, err)
	}
	if s.Version < 1 || s.Version > SnapshotVersion {
		return Snapshot{}, fmt.Errorf("%w: unsupported version %d", ErrInvalidSnapshot, s.Version)
	}
	return s, nil
}
