package event

import (
	"encoding/json"
	"testing"
)

// TestConfigChange_JSONFieldNames pins the field names of the version 1
// payload of config.pricing_changed and config.map_changed (spec 18.1).
func TestConfigChange_JSONFieldNames(t *testing.T) {
	tests := []struct {
		name string
		in   ConfigChange
		want string
	}{
		{
			"minimal",
			ConfigChange{Entity: "price_rule", EntityID: "r1", Change: ChangeUpdated, SeasonID: "s1"},
			`{"entity":"price_rule","entity_id":"r1","change":"updated","season_id":"s1"}`,
		},
		{
			"map",
			ConfigChange{Entity: "map", EntityID: "m1", Change: ChangeSaved, MapID: "m1", Version: 4},
			`{"entity":"map","entity_id":"m1","change":"saved","map_id":"m1","version":4}`,
		},
		{
			"catalog",
			ConfigChange{Entity: "pack", EntityID: "p1", Code: "base", Change: ChangeArchived},
			`{"entity":"pack","entity_id":"p1","change":"archived","code":"base"}`,
		},
		{
			"setup",
			ConfigChange{Entity: EntitySetup, EntityID: "e1", Change: ChangeApplied, Reason: "setup_template", Template: "classic_rows", MapIDs: []string{"m1"}, ZoneIDs: []string{}, Counts: map[string]int{"spots": 120}},
			`{"entity":"setup","entity_id":"e1","change":"applied","reason":"setup_template","template":"classic_rows","map_ids":["m1"],"counts":{"spots":120}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tt.want {
				t.Fatalf("got  %s\nwant %s", b, tt.want)
			}
			var back ConfigChange
			if err := json.Unmarshal(b, &back); err != nil || back.Entity != tt.in.Entity || back.Change != tt.in.Change {
				t.Fatalf("roundtrip: %+v %v", back, err)
			}
		})
	}
}
