package event

import (
	"encoding/json"
	"fmt"

	"github.com/coste360/arenile/pkg/ext"
)

func envelopeJSON(ev ext.Event) ([]byte, error) {
	b, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("event: marshal envelope: %w", err)
	}
	return b, nil
}

// StreamMessage is the lightweight real-time message of spec 21.1, derived
// from an event: clients use it to invalidate cached queries.
type StreamMessage struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"`
	EstablishmentID string   `json:"establishment_id"`
	OccurredAt      string   `json:"occurred_at"`
	SpotIDs         []string `json:"spot_ids,omitempty"`
	Dates           []string `json:"dates,omitempty"`
}

// ToStreamMessage extracts spot_ids and dates from the event data when
// present (top level), without exposing the rest of the payload.
func ToStreamMessage(ev ext.Event) StreamMessage {
	m := StreamMessage{ID: ev.ID, Type: ev.Type, EstablishmentID: ev.EstablishmentID, OccurredAt: ev.OccurredAt.UTC().Format("2006-01-02T15:04:05Z07:00")}
	var hint struct {
		SpotIDs []string `json:"spot_ids"`
		Dates   []string `json:"dates"`
	}
	if json.Unmarshal(ev.Data, &hint) == nil {
		m.SpotIDs, m.Dates = hint.SpotIDs, hint.Dates
	}
	return m
}
