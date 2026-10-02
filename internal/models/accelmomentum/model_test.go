package accelmomentum

import (
	"math"
	"testing"
	"time"

	"modelofinanzas/internal/storage/sqlite"
)

// monthlyBarsFromSteps genera un cierre de fin de mes por cada paso de
// `growthSteps` (len(growthSteps)+1 barras en total: el precio base mas
// uno por cada paso), en los meses asOf-(n+1) .. asOf-1 (ninguno cae en el
// mes de asOf, para no depender de la exclusion del mes en curso).
func monthlyBarsFromSteps(asOf time.Time, growthSteps []float64, startPrice float64) []sqlite.PriceBar {
	n := len(growthSteps)
	bars := make([]sqlite.PriceBar, 0, n+1)
	price := startPrice
	d := time.Date(asOf.Year(), asOf.Month(), 28, 0, 0, 0, 0, time.UTC).AddDate(0, -(n + 1), 0)
	bars = append(bars, sqlite.PriceBar{Date: d, Close: price, AdjClose: price})
	for _, g := range growthSteps {
		price *= 1 + g
		d = d.AddDate(0, 1, 0)
		bars = append(bars, sqlite.PriceBar{Date: d, Close: price, AdjClose: price})
	}
	return bars
}

func TestCompute_ConstantGrowthMatchesFormula(t *testing.T) {
	// Con crecimiento constante, el retorno de 3 y de 12 meses NO son
	// exactamente proporcionales (la capitalizacion compuesta es convexa),
	// asi que se verifica contra la formula documentada en vez de asumir 0.
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	const g = 0.01
	steps := make([]float64, 12)
	for i := range steps {
		steps[i] = g
	}
	bars := monthlyBarsFromSteps(asOf, steps, 100)

	got, err := Compute(bars, asOf, 3, 12)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	rShort := math.Pow(1+g, 3) - 1
	rLong := math.Pow(1+g, 12) - 1
	want := rShort - rLong*(3.0/12.0)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Compute() = %v, want %v", got, want)
	}
}

func TestCompute_RecentAccelerationIsPositive(t *testing.T) {
	// 9 meses "lentos" (0.1% mensual) seguidos de 3 meses "rapidos" (3%
	// mensual): el tramo reciente deberia rendir mas que su "cuota" del
	// retorno de 12 meses -> aceleracion positiva.
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	steps := make([]float64, 12)
	for i := range steps {
		if i < 9 {
			steps[i] = 0.001
		} else {
			steps[i] = 0.03
		}
	}
	bars := monthlyBarsFromSteps(asOf, steps, 100)

	got, err := Compute(bars, asOf, 3, 12)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if got <= 0 {
		t.Errorf("Compute() = %v, want > 0 (el tramo reciente acelero)", got)
	}
}

func TestCompute_InvalidMonthsParams(t *testing.T) {
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	steps := make([]float64, 12)
	bars := monthlyBarsFromSteps(asOf, steps, 100)

	if _, err := Compute(bars, asOf, 0, 12); err == nil {
		t.Error("esperaba error con shortMonths <= 0")
	}
	if _, err := Compute(bars, asOf, 12, 12); err == nil {
		t.Error("esperaba error con longMonths <= shortMonths")
	}
}
