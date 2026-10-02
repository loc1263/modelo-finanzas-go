package max52w

import (
	"math"
	"testing"
	"time"

	"modelofinanzas/internal/storage/sqlite"
)

func dailyBars(prices []float64) []sqlite.PriceBar {
	bars := make([]sqlite.PriceBar, len(prices))
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	for i, p := range prices {
		bars[i] = sqlite.PriceBar{Date: start.AddDate(0, 0, i), Close: p, AdjClose: p}
	}
	return bars
}

func TestCompute_RatioAgainstWindowMax(t *testing.T) {
	// Maximo de la ventana es 150 (indice 2); precio actual (ultimo) es 120.
	bars := dailyBars([]float64{100, 110, 150, 130, 120})

	got, err := Compute(bars, 5)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	want := 120.0 / 150.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Compute() = %v, want %v", got, want)
	}
}

func TestCompute_AtNewHigh(t *testing.T) {
	// El precio mas reciente ES el maximo: ratio debe ser exactamente 1.
	bars := dailyBars([]float64{90, 95, 100})
	got, err := Compute(bars, 3)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if got != 1 {
		t.Errorf("Compute() = %v, want 1 (precio actual en maximo)", got)
	}
}

func TestCompute_LookbackWindowSmallerThanHistory(t *testing.T) {
	// Precio mas alto (200) queda fuera de la ventana de 2 dias; solo deben
	// mirarse los ultimos 2 dias (100, 90).
	bars := dailyBars([]float64{200, 50, 100, 90})
	got, err := Compute(bars, 2)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	want := 90.0 / 100.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Compute() = %v, want %v (no deberia considerar el 200 fuera de ventana)", got, want)
	}
}

func TestCompute_EmptyBars(t *testing.T) {
	if _, err := Compute(nil, 252); err == nil {
		t.Error("esperaba error con barras vacias")
	}
}
