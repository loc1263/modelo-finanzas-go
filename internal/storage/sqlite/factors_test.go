package sqlite

import (
	"testing"
	"time"

	"modelofinanzas/internal/famafrench"
)

func TestUpsertFactors_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	monthly := []famafrench.MonthlyFactors{
		{Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Values: map[string]float64{"Mkt-RF": 0.02, "SMB": -0.01, "HML": 0.005, "RF": 0.003}},
		{Date: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), Values: map[string]float64{"Mkt-RF": -0.01, "SMB": 0.01, "HML": -0.002, "RF": 0.003}},
	}

	if err := db.UpsertFactors("F-F_Research_Data_Factors", monthly); err != nil {
		t.Fatalf("UpsertFactors: %v", err)
	}

	got, err := db.GetFactors("F-F_Research_Data_Factors", monthly[0].Date, monthly[1].Date)
	if err != nil {
		t.Fatalf("GetFactors: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Values["Mkt-RF"] != 0.02 || got[1].Values["SMB"] != 0.01 {
		t.Errorf("valores no coinciden: got %+v", got)
	}
}

func TestUpsertFactors_IdempotentOnRerun(t *testing.T) {
	db := openTestDB(t)
	monthly := []famafrench.MonthlyFactors{
		{Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Values: map[string]float64{"Mkt-RF": 0.02, "RF": 0.003}},
	}

	if err := db.UpsertFactors("F-F_Research_Data_Factors", monthly); err != nil {
		t.Fatalf("primer UpsertFactors: %v", err)
	}
	if err := db.UpsertFactors("F-F_Research_Data_Factors", monthly); err != nil {
		t.Fatalf("segundo UpsertFactors: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM factors`).Scan(&count); err != nil {
		t.Fatalf("contando filas: %v", err)
	}
	if count != 2 { // 1 mes x 2 factores (Mkt-RF, RF)
		t.Errorf("count = %d, want 2 (no deberia duplicar al reejecutar)", count)
	}
}

func TestGetFactors_FiltersByDatasetAndRange(t *testing.T) {
	db := openTestDB(t)
	monthly := []famafrench.MonthlyFactors{
		{Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Values: map[string]float64{"RF": 0.001}},
	}
	if err := db.UpsertFactors("F-F_Research_Data_Factors", monthly); err != nil {
		t.Fatalf("UpsertFactors (3-factor): %v", err)
	}
	if err := db.UpsertFactors("F-F_Momentum_Factor", monthly); err != nil {
		t.Fatalf("UpsertFactors (momentum): %v", err)
	}

	got, err := db.GetFactors("F-F_Momentum_Factor", monthly[0].Date, monthly[0].Date)
	if err != nil {
		t.Fatalf("GetFactors: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (solo el dataset pedido)", len(got))
	}
}
