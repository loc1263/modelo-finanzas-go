package sectormomentum

import (
	"math"
	"testing"
	"time"

	"modelofinanzas/internal/models/momentum12_1"
	"modelofinanzas/internal/storage/sqlite"
)

func TestCompute_DelegatesToMomentum12_1(t *testing.T) {
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)

	var bars []sqlite.PriceBar
	price := 100.0
	d := time.Date(asOf.Year(), asOf.Month(), 28, 0, 0, 0, 0, time.UTC).AddDate(0, -13, 0)
	bars = append(bars, sqlite.PriceBar{Date: d, Close: price, AdjClose: price})
	for range 12 {
		price *= 1.02
		d = d.AddDate(0, 1, 0)
		bars = append(bars, sqlite.PriceBar{Date: d, Close: price, AdjClose: price})
	}

	got, err := Compute(bars, asOf, 12)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	want, err := momentum12_1.Compute(bars, asOf, 12)
	if err != nil {
		t.Fatalf("error inesperado en momentum12_1.Compute: %v", err)
	}
	if math.Abs(got-want) > 1e-15 {
		t.Errorf("Compute() = %v, want %v (igual a momentum12_1.Compute)", got, want)
	}
}
