//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestMigration00011_PrintingSchema checks the constraints of migration
// 00011 on both dialects: printer name unique per establishment, private
// fields round-trip (logo raster bytes), one routing rule per document and
// register (default rule with NULL register included), idempotency key
// unique per establishment and printed_at bound to the printed status.
func TestMigration00011_PrintingSchema(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-prn", "Lido PRN", t0)
		if err != nil {
			t.Fatal(err)
		}
		pin := 2
		p := &store.Printer{
			ID: store.NewID(), EstablishmentID: est.ID, Name: "Banco", Driver: store.PrinterDriverNetwork,
			Host: "192.168.1.50", Port: store.DefaultPrinterPort, PaperWidthMM: 80, CharsPerLine: 48,
			Codepage: "cp858", CutMode: store.PrinterCutPartial, FeedLines: 3, PrintLogo: true,
			LogoRaster: []byte{0x1d, 0x76, 0x30, 0x00, 0xff}, DrawerPin: &pin, Active: true,
			Status: store.PrinterStatusUnknown, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		if _, err := db.NewInsert().Model(p).Exec(ctx); err != nil {
			t.Fatalf("insert printer: %v", err)
		}
		var got store.Printer
		if err := db.NewSelect().Model(&got).Where("id = ?", p.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if string(got.LogoRaster) != string(p.LogoRaster) || got.DrawerPin == nil || *got.DrawerPin != 2 || !got.PrintLogo {
			t.Fatalf("printer round trip = %+v", got)
		}
		dup := *p
		dup.ID = store.NewID()
		if _, err := db.NewInsert().Model(&dup).Exec(ctx); err == nil {
			t.Fatal("duplicate printer name accepted")
		}
		bad := *p
		bad.ID, bad.Name, bad.Driver = store.NewID(), "Bridge", "bluetooth"
		if _, err := db.NewInsert().Model(&bad).Exec(ctx); err == nil {
			t.Fatal("unknown driver accepted")
		}

		reg := &store.CashRegister{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "bar", Name: "Bar", Active: true,
			SortOrder: 1, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		if _, err := db.NewInsert().Model(reg).Exec(ctx); err != nil {
			t.Fatalf("insert register: %v", err)
		}
		route := func(register *string) *store.PrinterRoute {
			return &store.PrinterRoute{
				ID: store.NewID(), EstablishmentID: est.ID, DocumentKind: store.PrintDocCashZ, RegisterID: register,
				PrinterID: p.ID, Copies: 1, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		for i, r := range []*store.PrinterRoute{route(nil), route(&reg.ID)} {
			if _, err := db.NewInsert().Model(r).Exec(ctx); err != nil {
				t.Fatalf("insert route %d: %v", i, err)
			}
		}
		for i, r := range []*store.PrinterRoute{route(nil), route(&reg.ID)} {
			if _, err := db.NewInsert().Model(r).Exec(ctx); err == nil {
				t.Fatalf("duplicate route %d accepted", i)
			}
		}

		job := func(key string) *store.PrintJob {
			return &store.PrintJob{
				ID: store.NewID(), EstablishmentID: est.ID, PrinterID: &p.ID, DocumentKind: store.PrintDocTestPage,
				Copies: 1, Status: store.PrintJobQueued, IdempotencyKey: key, Payload: []byte{0x1b, 0x40},
				MaxAttempts: store.DefaultPrintJobMaxAttempts, NextAttemptAt: &t0, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		if _, err := db.NewInsert().Model(job("test:1")).Exec(ctx); err != nil {
			t.Fatalf("insert job: %v", err)
		}
		if _, err := db.NewInsert().Model(job("test:1")).Exec(ctx); err == nil {
			t.Fatal("duplicate idempotency key accepted")
		}
		printed := job("test:2")
		printed.Status = store.PrintJobPrinted
		if _, err := db.NewInsert().Model(printed).Exec(ctx); err == nil {
			t.Fatal("printed job without printed_at accepted")
		}
		receipt := job("receipt:1")
		receipt.DocumentKind = store.PrintDocPaymentReceipt
		if _, err := db.NewInsert().Model(receipt).Exec(ctx); err == nil {
			t.Fatal("receipt job without document_id accepted")
		}

		// Deleting the printer drops its routes and keeps the job history.
		if _, err := db.NewDelete().Model((*store.Printer)(nil)).Where("id = ?", p.ID).Exec(ctx); err != nil {
			t.Fatalf("delete printer: %v", err)
		}
		n, err := db.NewSelect().Model((*store.PrinterRoute)(nil)).Where("establishment_id = ?", est.ID).Count(ctx)
		if err != nil || n != 0 {
			t.Fatalf("routes after delete = %d, %v", n, err)
		}
		var jobs []store.PrintJob
		if err := db.NewSelect().Model(&jobs).Where("establishment_id = ?", est.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if len(jobs) != 1 || jobs[0].PrinterID != nil {
			t.Fatalf("jobs after delete = %+v", jobs)
		}
	})
}
