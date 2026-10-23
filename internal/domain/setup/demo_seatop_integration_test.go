//go:build integration

package setup

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

type demoGeometry struct {
	spots map[string][3]float64 // id -> x, y, rotation
	decos map[string][3]float64
}

func loadDemoGeometry(t *testing.T, db bun.IDB, mapID string) demoGeometry {
	t.Helper()
	ctx := context.Background()
	g := demoGeometry{spots: map[string][3]float64{}, decos: map[string][3]float64{}}
	var spots []store.Spot
	if err := db.NewSelect().Model(&spots).Where("sp.map_id = ?", mapID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	for _, sp := range spots {
		g.spots[sp.ID] = [3]float64{sp.X, sp.Y, sp.Rotation}
	}
	var decos []store.MapDecoration
	if err := db.NewSelect().Model(&decos).Where("md.map_id = ?", mapID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	for _, d := range decos {
		g.decos[d.ID] = [3]float64{d.X, d.Y, d.Rotation}
	}
	return g
}

// A demo seeded before M5 (template map with the sea at the bottom) gets
// the sea at the top on the next seed, once: geometry only, audited, with
// config.map_changed "reoriented"; maps drawn by hand are left alone.
func TestSeedDemo_UpgradesSeaToTop(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db, "lido-demo")
		if _, err := f.svc.SeedDemo(ctx, f.est.ID); err != nil {
			t.Fatal(err)
		}
		var m store.Map
		if err := db.NewSelect().Model(&m).Where("m.establishment_id = ?", f.est.ID).Limit(1).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if m.SeaSide != mapping.SeaTop {
			t.Fatalf("new demo sea side %s", m.SeaSide)
		}
		top := loadDemoGeometry(t, db, m.ID)

		// Back to the pre-M5 state: the same layout mirrored, sea at the bottom.
		es, err := store.SystemEstablishmentScope(ctx, db, f.est.ID)
		if err != nil {
			t.Fatal(err)
		}
		err = store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			// Mirror with the same code path, then label the map "bottom".
			if _, err := f.svc.reorientDemoMapTo(ctx, tx, es, &m, mapping.SeaBottom, time.Now()); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		hand := store.Map{ID: store.NewID(), EstablishmentID: f.est.ID, Name: store.I18nText{"it": "A mano"}, Width: 600, Height: 400, GridSize: 10, SeaSide: mapping.SeaBottom, SortOrder: 9, Version: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if _, err := db.NewInsert().Model(&hand).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		sign := store.MapDecoration{ID: store.NewID(), EstablishmentID: f.est.ID, MapID: hand.ID, Kind: "text", X: 300, Y: 200, Shape: store.Shape{Type: "rect", W: 80, H: 20}, Label: store.I18nText{"it": "Mio"}, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if _, err := db.NewInsert().Model(&sign).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		var before store.Map
		if err := db.NewSelect().Model(&before).Where("m.id = ?", m.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if before.SeaSide != mapping.SeaBottom {
			t.Fatalf("fixture sea side %s", before.SeaSide)
		}

		up, err := f.svc.SeedDemo(ctx, f.est.ID)
		if err != nil || up.Applied || up.MapsReoriented != 1 || up.MapsUpgraded != 0 {
			t.Fatalf("upgrade: %+v %v", up, err)
		}
		var after store.Map
		if err := db.NewSelect().Model(&after).Where("m.id = ?", m.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if after.SeaSide != mapping.SeaTop || after.Version != before.Version+1 || after.Width != before.Width || after.Height != before.Height {
			t.Fatalf("map after upgrade: %+v (before %+v)", after, before)
		}
		got := loadDemoGeometry(t, db, m.ID)
		for id, want := range top.spots {
			if got.spots[id] != want {
				t.Fatalf("spot %s: %v, want %v", id, got.spots[id], want)
			}
		}
		for id, want := range top.decos {
			if got.decos[id] != want {
				t.Fatalf("decoration %s: %v, want %v", id, got.decos[id], want)
			}
		}
		var handAfter store.Map
		if err := db.NewSelect().Model(&handAfter).Where("m.id = ?", hand.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if handAfter.SeaSide != mapping.SeaBottom || handAfter.Version != 1 {
			t.Fatalf("hand-made map changed: %+v", handAfter)
		}
		if n, err := db.NewSelect().Model((*store.AuditEntry)(nil)).
			Where("establishment_id = ? AND action = ? AND entity_id = ?", f.est.ID, "setup.demo_map_reoriented", m.ID).Count(ctx); err != nil || n != 2 { // fixture + upgrade
			t.Fatalf("audit entries: %d %v", n, err)
		}
		var events []store.OutboxEvent
		if err := db.NewSelect().Model(&events).Where("type = ?", event.ConfigMapChanged).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, ev := range events {
			var c event.ConfigChange
			if err := json.Unmarshal(ev.Payload, &c); err != nil {
				t.Fatal(err)
			}
			found = found || (c.Change == event.ChangeReoriented && c.MapID == m.ID && c.Version == after.Version)
		}
		if !found {
			t.Fatal("no config.map_changed reoriented event")
		}
		again, err := f.svc.SeedDemo(ctx, f.est.ID)
		if err != nil || again.MapsReoriented != 0 {
			t.Fatalf("again: %+v %v", again, err)
		}
	})
}
