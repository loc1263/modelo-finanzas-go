package absolutemomentum

import (
	"math"
	"testing"
	"time"

	"modelofinanzas/internal/storage/sqlite"
)

// monthlyBarsFromSteps genera un cierre de fin de mes por cada paso de
// growthSteps, en los meses asOf-(n+1) .. asOf-1.
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

func factorRowsForMonths(dates []time.Time, rf float64) []sqlite.FactorRow {
	rows := make([]sqlite.FactorRow, len(dates))
	for i, d := range dates {
		rows[i] = sqlite.FactorRow{Date: d, Values: map[string]float64{"RF": rf}}
	}
	return rows
}

func TestCompute_MatchesExpectedExcessReturn(t *testing.T) {
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	const g = 0.02
	const rf = 0.003
	steps := make([]float64, 12)
	for i := range steps {
		steps[i] = g
	}
	bars := monthlyBarsFromSteps(asOf, steps, 100)

	// Fama-French al dia, con RF para cada uno de los 12 meses de la ventana.
	dates := make([]time.Time, 12)
	for i := range 12 {
		dates[i] = bars[i+1].Date
	}
	factorRows := factorRowsForMonths(dates, rf)

	got, err := Compute(bars, factorRows, asOf, 12)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	rTotal := math.Pow(1+g, 12) - 1
	cumRF := math.Pow(1+rf, 12) - 1
	want := rTotal - cumRF
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Compute() = %v, want %v", got, want)
	}
}

// TestCompute_ToleratesFamaFrenchLag reproduce el escenario real que motivo
// MEJ-003: la Kenneth French Data Library publica con 1-2 meses de atraso
// respecto al precio mas reciente de la accion. El modelo debe correr la
// ventana hacia atras (usar los ultimos 12 meses YA ALINEADOS) en vez de
// fallar porque falten los meses mas recientes de RF.
func TestCompute_ToleratesFamaFrenchLag(t *testing.T) {
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	const g = 0.01
	const rf = 0.002
	const lagMonths = 2

	// 14 meses de precios (2 mas de los que pide la ventana), para que
	// sobren meses recientes sin dato de RF.
	steps := make([]float64, 14)
	for i := range steps {
		steps[i] = g
	}
	bars := monthlyBarsFromSteps(asOf, steps, 100)

	// Factores de Fama-French solo hasta `lagMonths` meses atras del ultimo
	// precio disponible (el "atraso" real que se encontro con datos reales).
	allDates := make([]time.Time, len(bars)-1)
	for i := range allDates {
		allDates[i] = bars[i+1].Date
	}
	laggedDates := allDates[:len(allDates)-lagMonths]
	factorRows := factorRowsForMonths(laggedDates, rf)

	got, err := Compute(bars, factorRows, asOf, 12)
	if err != nil {
		t.Fatalf("error inesperado: el modelo deberia tolerar el atraso de RF, no fallar: %v", err)
	}

	// La ventana efectiva de 12 meses queda corrida 2 meses hacia atras
	// (termina en el ultimo mes con RF disponible, no en el ultimo precio).
	rTotal := math.Pow(1+g, 12) - 1 // el retorno de 12 meses es el mismo igual (crecimiento constante)
	cumRF := math.Pow(1+rf, 12) - 1
	want := rTotal - cumRF
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Compute() = %v, want %v", got, want)
	}
}

func TestCompute_InsufficientAlignedMonths(t *testing.T) {
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	steps := make([]float64, 12)
	bars := monthlyBarsFromSteps(asOf, steps, 100)

	// Solo 5 meses de RF disponibles, se piden 12.
	dates := make([]time.Time, 5)
	for i := range dates {
		dates[i] = bars[i+1].Date
	}
	factorRows := factorRowsForMonths(dates, 0.002)

	if _, err := Compute(bars, factorRows, asOf, 12); err == nil {
		t.Error("esperaba error por pocos meses alineados con RF")
	}
}

func TestCompute_NoFactorDataAtAll(t *testing.T) {
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	steps := make([]float64, 12)
	bars := monthlyBarsFromSteps(asOf, steps, 100)

	if _, err := Compute(bars, nil, asOf, 12); err == nil {
		t.Error("esperaba error sin ningun dato de RF")
	}
}
