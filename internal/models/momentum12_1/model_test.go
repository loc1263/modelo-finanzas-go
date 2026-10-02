package momentum12_1

import (
	"math"
	"testing"
	"time"

	"modelofinanzas/internal/storage/sqlite"
)

// monthEndBars genera una barra por mes (dia 28) desde startMonth meses
// antes de asOf, con un precio que crece `monthlyGrowth` cada mes.
func monthEndBars(asOf time.Time, months int, startPrice, monthlyGrowth float64) []sqlite.PriceBar {
	var bars []sqlite.PriceBar
	price := startPrice
	start := asOf.AddDate(0, -months, 0)
	for i := 0; i <= months; i++ {
		d := time.Date(start.Year(), start.Month(), 28, 0, 0, 0, 0, time.UTC).AddDate(0, i, 0)
		bars = append(bars, sqlite.PriceBar{Date: d, Close: price, AdjClose: price})
		price *= 1 + monthlyGrowth
	}
	return bars
}

func TestCompute_KnownGrowth(t *testing.T) {
	asOf := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)
	// 13 meses de historia (para tener 12 meses completos antes de marzo),
	// precio sube 1% cada mes -> retorno 12 meses ~ (1.01^12 - 1).
	bars := monthEndBars(asOf, 13, 100, 0.01)

	got, err := Compute(bars, asOf, 12)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	want := math.Pow(1.01, 12) - 1
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Compute() = %v, want %v", got, want)
	}
}

func TestCompute_InsufficientHistory(t *testing.T) {
	asOf := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)
	bars := monthEndBars(asOf, 3, 100, 0.01) // solo 3 meses, pide 12
	if _, err := Compute(bars, asOf, 12); err == nil {
		t.Error("esperaba error por historia insuficiente")
	}
}
