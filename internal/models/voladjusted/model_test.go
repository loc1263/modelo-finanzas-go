package voladjusted

import (
	"math"
	"testing"
	"time"

	"modelofinanzas/internal/models/common"
	"modelofinanzas/internal/storage/sqlite"
)

// alternatingDailyBars genera `days` barras diarias donde el retorno diario
// alterna entre +g y -g/2 (nunca es constante, para que el desvio estandar
// de los retornos diarios sea siempre distinto de cero).
func alternatingDailyBars(days int, start time.Time, startPrice, g float64) []sqlite.PriceBar {
	bars := make([]sqlite.PriceBar, days)
	price := startPrice
	for i := range days {
		bars[i] = sqlite.PriceBar{Date: start.AddDate(0, 0, i), Close: price, AdjClose: price}
		if i%2 == 0 {
			price *= 1 + g
		} else {
			price *= 1 - g/2
		}
	}
	return bars
}

func TestCompute_MatchesIndependentCalculation(t *testing.T) {
	asOf := time.Date(2026, time.June, 10, 0, 0, 0, 0, time.UTC)
	start := asOf.AddDate(-1, -2, 0) // ~14 meses de historia
	bars := alternatingDailyBars(430, start, 100, 0.01)

	longMonths, shortMonths, volLookbackDays := 12, 6, 252

	got, err := Compute(bars, asOf, longMonths, shortMonths, volLookbackDays)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	// Recalculo independiente usando directamente las funciones de common,
	// tal como lo documenta el paquete.
	closes := common.MonthEndCloses(bars, asOf)
	rLong, err := common.TrailingReturn(closes, longMonths)
	if err != nil {
		t.Fatalf("TrailingReturn(long) error: %v", err)
	}
	rShort, err := common.TrailingReturn(closes, shortMonths)
	if err != nil {
		t.Fatalf("TrailingReturn(short) error: %v", err)
	}
	window := common.LastNBars(common.ExcludeCurrentMonth(bars, asOf), volLookbackDays)
	vol := common.AnnualizedVol(common.DailyReturns(window), 252)
	want := ((rLong / vol) + (rShort / vol)) / 2

	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Compute() = %v, want %v", got, want)
	}
}

func TestCompute_ZeroVolatilityErrors(t *testing.T) {
	asOf := time.Date(2026, time.June, 10, 0, 0, 0, 0, time.UTC)
	start := asOf.AddDate(-1, -2, 0)

	// Precio constante: retornos diarios son todos 0, desvio estandar = 0.
	bars := make([]sqlite.PriceBar, 430)
	for i := range bars {
		bars[i] = sqlite.PriceBar{Date: start.AddDate(0, 0, i), Close: 100, AdjClose: 100}
	}

	if _, err := Compute(bars, asOf, 12, 6, 252); err == nil {
		t.Error("esperaba error por volatilidad cero")
	}
}

func TestCompute_InsufficientHistory(t *testing.T) {
	asOf := time.Date(2026, time.June, 10, 0, 0, 0, 0, time.UTC)
	bars := alternatingDailyBars(60, asOf.AddDate(0, -2, 0), 100, 0.01) // solo 2 meses
	if _, err := Compute(bars, asOf, 12, 6, 252); err == nil {
		t.Error("esperaba error por historia insuficiente para el retorno de 12 meses")
	}
}
