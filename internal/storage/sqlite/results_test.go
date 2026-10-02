package sqlite

import (
	"testing"
	"time"
)

func sampleResults(asOf time.Time) []ModelResult {
	return []ModelResult{
		{Universe: "megacap", Model: "momentum_12_1", Symbol: "AAPL", AsOf: asOf, Score: 0.25, Rank: 1},
		{Universe: "megacap", Model: "momentum_12_1", Symbol: "MSFT", AsOf: asOf, Score: 0.10, Rank: 2},
	}
}

func TestInsertResults_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	runDate := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	asOf := runDate

	if err := db.InsertResults(runDate, sampleResults(asOf)); err != nil {
		t.Fatalf("InsertResults: %v", err)
	}

	got, err := db.GetResults(runDate, "megacap", "momentum_12_1")
	if err != nil {
		t.Fatalf("GetResults: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Symbol != "AAPL" || got[0].Rank != 1 {
		t.Errorf("got[0] = %+v, want AAPL rank 1 primero", got[0])
	}
}

// TestInsertResults_SameDayOverwritesNotDuplicates es la garantia central
// de idempotencia documentada en README/CLAUDE.md: reejecutar el pipeline
// el mismo dia debe sobreescribir, no acumular filas.
func TestInsertResults_SameDayOverwritesNotDuplicates(t *testing.T) {
	db := openTestDB(t)
	runDate := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)

	if err := db.InsertResults(runDate, sampleResults(runDate)); err != nil {
		t.Fatalf("primer InsertResults: %v", err)
	}

	// "Reejecucion" el mismo dia, con scores distintos (como pasaria si los
	// precios cambiaron durante el dia).
	revised := sampleResults(runDate)
	revised[0].Score = 0.99
	if err := db.InsertResults(runDate, revised); err != nil {
		t.Fatalf("segundo InsertResults: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM model_results`).Scan(&count); err != nil {
		t.Fatalf("contando filas: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 (no deberia duplicar al reejecutar el mismo dia)", count)
	}

	got, err := db.GetResults(runDate, "megacap", "momentum_12_1")
	if err != nil {
		t.Fatalf("GetResults: %v", err)
	}
	for _, r := range got {
		if r.Symbol == "AAPL" && r.Score != 0.99 {
			t.Errorf("AAPL.Score = %v, want 0.99 (deberia haberse sobreescrito)", r.Score)
		}
	}
}

func TestInsertResults_DifferentDaysCoexist(t *testing.T) {
	db := openTestDB(t)
	day1 := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC)

	if err := db.InsertResults(day1, sampleResults(day1)); err != nil {
		t.Fatalf("InsertResults day1: %v", err)
	}
	if err := db.InsertResults(day2, sampleResults(day2)); err != nil {
		t.Fatalf("InsertResults day2: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM model_results`).Scan(&count); err != nil {
		t.Fatalf("contando filas: %v", err)
	}
	if count != 4 {
		t.Errorf("count = %d, want 4 (2 dias x 2 resultados, ambos dias deben coexistir)", count)
	}
}
