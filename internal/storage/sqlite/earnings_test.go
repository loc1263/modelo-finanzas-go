package sqlite

import (
	"testing"
	"time"

	"modelofinanzas/internal/yahoo"
)

func sampleQuarters(n int) []yahoo.EarningsQuarter {
	start := time.Date(2025, time.January, 31, 0, 0, 0, 0, time.UTC)
	out := make([]yahoo.EarningsQuarter, n)
	for i := range n {
		out[i] = yahoo.EarningsQuarter{
			Quarter:     start.AddDate(0, 3*i, 0),
			EPSActual:   1.0 + float64(i)*0.1,
			EPSEstimate: 1.0,
			SurprisePct: float64(i) * 0.01,
		}
	}
	return out
}

func TestUpsertEarnings_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	quarters := sampleQuarters(4)

	if err := db.UpsertEarnings("AAPL", quarters); err != nil {
		t.Fatalf("UpsertEarnings: %v", err)
	}

	got, err := db.GetEarnings("AAPL", 10)
	if err != nil {
		t.Fatalf("GetEarnings: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4", len(got))
	}
	// Debe venir ascendente (el mas viejo primero).
	if !got[0].Quarter.Before(got[len(got)-1].Quarter) {
		t.Errorf("GetEarnings no vino en orden ascendente: %+v", got)
	}
}

// TestGetEarnings_LimitsToMostRecentQuarters es la prueba de MEJ-002: sin
// limite, el score del modelo 8 iria usando cada vez mas trimestres con el
// paso del tiempo.
func TestGetEarnings_LimitsToMostRecentQuarters(t *testing.T) {
	db := openTestDB(t)
	quarters := sampleQuarters(8) // mas trimestres de los que se van a pedir

	if err := db.UpsertEarnings("AAPL", quarters); err != nil {
		t.Fatalf("UpsertEarnings: %v", err)
	}

	got, err := db.GetEarnings("AAPL", 4)
	if err != nil {
		t.Fatalf("GetEarnings: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4 (limitado)", len(got))
	}

	// Deben ser los 4 MAS RECIENTES (los ultimos 4 de la lista original,
	// que esta en orden ascendente por construccion de sampleQuarters).
	wantFirst := quarters[4].Quarter
	if !got[0].Quarter.Equal(wantFirst) {
		t.Errorf("got[0].Quarter = %v, want %v (deberia ser el 5to trimestre, no el mas viejo)", got[0].Quarter, wantFirst)
	}
}

func TestUpsertEarnings_IdempotentOnRerun(t *testing.T) {
	db := openTestDB(t)
	quarters := sampleQuarters(4)

	if err := db.UpsertEarnings("AAPL", quarters); err != nil {
		t.Fatalf("primer UpsertEarnings: %v", err)
	}
	if err := db.UpsertEarnings("AAPL", quarters); err != nil {
		t.Fatalf("segundo UpsertEarnings: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM earnings WHERE symbol = 'AAPL'`).Scan(&count); err != nil {
		t.Fatalf("contando filas: %v", err)
	}
	if count != 4 {
		t.Errorf("count = %d, want 4 (no deberia duplicar al reejecutar)", count)
	}
}
