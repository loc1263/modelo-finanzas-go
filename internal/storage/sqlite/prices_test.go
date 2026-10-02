package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"modelofinanzas/internal/yahoo"
)

// openTestDB abre una base SQLite nueva en un directorio temporal de la
// prueba (nunca toca data/ ni data/test/ del proyecto).
func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("abriendo db de prueba: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func sampleBars() []yahoo.Bar {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	return []yahoo.Bar{
		{Date: start, Open: 10, High: 11, Low: 9, Close: 10.5, AdjClose: 10.5, Volume: 1000},
		{Date: start.AddDate(0, 0, 1), Open: 10.5, High: 12, Low: 10, Close: 11.5, AdjClose: 11.5, Volume: 1200},
	}
}

func TestUpsertBars_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	bars := sampleBars()

	if err := db.UpsertBars("AAPL", bars); err != nil {
		t.Fatalf("UpsertBars: %v", err)
	}

	got, err := db.GetPrices("AAPL", bars[0].Date, bars[len(bars)-1].Date)
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	if len(got) != len(bars) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(bars))
	}
	if got[0].Close != bars[0].Close || got[1].AdjClose != bars[1].AdjClose {
		t.Errorf("datos no coinciden: got %+v", got)
	}
}

// TestUpsertBars_IdempotentOnRerun es la garantia central del proyecto:
// correr el pipeline (o solo UpsertBars) dos veces con los mismos datos del
// dia no debe duplicar filas.
func TestUpsertBars_IdempotentOnRerun(t *testing.T) {
	db := openTestDB(t)
	bars := sampleBars()

	if err := db.UpsertBars("AAPL", bars); err != nil {
		t.Fatalf("primer UpsertBars: %v", err)
	}
	if err := db.UpsertBars("AAPL", bars); err != nil {
		t.Fatalf("segundo UpsertBars: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM prices WHERE symbol = 'AAPL'`).Scan(&count); err != nil {
		t.Fatalf("contando filas: %v", err)
	}
	if count != len(bars) {
		t.Errorf("count = %d, want %d (no deberia duplicar al reejecutar)", count, len(bars))
	}
}

func TestUpsertBars_UpdatesValuesOnConflict(t *testing.T) {
	db := openTestDB(t)
	bars := sampleBars()
	if err := db.UpsertBars("AAPL", bars); err != nil {
		t.Fatalf("UpsertBars: %v", err)
	}

	revised := sampleBars()
	revised[0].Close = 99.99
	revised[0].AdjClose = 99.99
	if err := db.UpsertBars("AAPL", revised); err != nil {
		t.Fatalf("UpsertBars (revisado): %v", err)
	}

	got, err := db.GetPrices("AAPL", bars[0].Date, bars[0].Date)
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	if len(got) != 1 || got[0].Close != 99.99 {
		t.Errorf("got = %+v, el valor deberia haberse actualizado a 99.99", got)
	}
}

func TestGetPrices_FiltersBySymbolAndRange(t *testing.T) {
	db := openTestDB(t)
	if err := db.UpsertBars("AAPL", sampleBars()); err != nil {
		t.Fatalf("UpsertBars AAPL: %v", err)
	}
	if err := db.UpsertBars("MSFT", sampleBars()); err != nil {
		t.Fatalf("UpsertBars MSFT: %v", err)
	}

	got, err := db.GetPrices("AAPL", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (solo AAPL, solo el primer dia)", len(got))
	}
}
