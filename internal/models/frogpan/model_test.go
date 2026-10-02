package frogpan

import (
	"testing"
	"time"

	"modelofinanzas/internal/storage/sqlite"
)

// dailyBarsFromPattern genera `totalDays` barras diarias terminando en
// asOf-1 (para que el ultimo dia quede dentro del mes anterior a asOf, y
// common.MonthEndCloses no lo excluya), aplicando sequentially los
// retornos diarios de `dailyReturns` en forma ciclica.
func dailyBarsFromPattern(asOf time.Time, totalDays int, dailyReturns []float64, startPrice float64) []sqlite.PriceBar {
	end := asOf.AddDate(0, 0, -1)
	start := end.AddDate(0, 0, -totalDays+1)

	bars := make([]sqlite.PriceBar, totalDays)
	price := startPrice
	for i := range totalDays {
		if i > 0 {
			price *= 1 + dailyReturns[(i-1)%len(dailyReturns)]
		}
		bars[i] = sqlite.PriceBar{Date: start.AddDate(0, 0, i), Close: price, AdjClose: price}
	}
	return bars
}

func TestCompute_SmoothTrendScoresHigherThanChoppyTrend(t *testing.T) {
	asOf := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	const days = 400 // > 13 meses de historia diaria

	// Tendencia pareja: 19 dias subiendo de a poco por cada dia que baja un poco.
	smoothPattern := make([]float64, 20)
	for i := range 19 {
		smoothPattern[i] = 0.003
	}
	smoothPattern[19] = -0.001
	smoothBars := dailyBarsFromPattern(asOf, days, smoothPattern, 100)

	// Tendencia "a saltos": la mayoria de los dias baja un poco, y de vez en
	// cuando un salto grande hacia arriba compensa todo.
	choppyPattern := make([]float64, 20)
	for i := range 19 {
		choppyPattern[i] = -0.004
	}
	choppyPattern[19] = 0.15
	choppyBars := dailyBarsFromPattern(asOf, days, choppyPattern, 100)

	smoothScore, err := Compute(smoothBars, asOf, 12)
	if err != nil {
		t.Fatalf("error inesperado (smooth): %v", err)
	}
	choppyScore, err := Compute(choppyBars, asOf, 12)
	if err != nil {
		t.Fatalf("error inesperado (choppy): %v", err)
	}

	if smoothScore <= choppyScore {
		t.Errorf("smoothScore=%v, choppyScore=%v; se esperaba que la tendencia pareja puntuara mas alto", smoothScore, choppyScore)
	}
}

func TestCompute_InsufficientHistory(t *testing.T) {
	asOf := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	bars := dailyBarsFromPattern(asOf, 30, []float64{0.01, -0.01}, 100)
	if _, err := Compute(bars, asOf, 12); err == nil {
		t.Error("esperaba error por historia insuficiente")
	}
}
