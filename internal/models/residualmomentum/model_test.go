package residualmomentum

import (
	"math"
	"testing"
	"time"

	"modelofinanzas/internal/models/common"
	"modelofinanzas/internal/storage/sqlite"
)

// buildMonthlyBars genera un cierre de fin de mes por cada entrada de
// returns (el primero es el precio base), de forma que
// common.MonthlyReturns(...) reproduzca exactamente `returns`.
func buildMonthlyBars(asOf time.Time, returns []float64) []sqlite.PriceBar {
	n := len(returns)
	start := asOf.AddDate(0, -(n + 1), 0)
	price := 100.0
	bars := make([]sqlite.PriceBar, 0, n+1)
	d := time.Date(start.Year(), start.Month(), 28, 0, 0, 0, 0, time.UTC)
	bars = append(bars, sqlite.PriceBar{Date: d, Close: price, AdjClose: price})
	for _, r := range returns {
		price *= 1 + r
		d = d.AddDate(0, 1, 0)
		bars = append(bars, sqlite.PriceBar{Date: d, Close: price, AdjClose: price})
	}
	return bars
}

func factorRowsFromStockDates(bars []sqlite.PriceBar, mktRF, smb, hml, rf []float64) []sqlite.FactorRow {
	// bars[0] es el precio base (sin retorno asociado); los factores van
	// alineados con bars[1:], igual que stockReturns en Compute().
	rows := make([]sqlite.FactorRow, len(mktRF))
	for i := range mktRF {
		rows[i] = sqlite.FactorRow{
			Date: bars[i+1].Date,
			Values: map[string]float64{
				"Mkt-RF": mktRF[i], "SMB": smb[i], "HML": hml[i], "RF": rf[i],
			},
		}
	}
	return rows
}

// syntheticFactorScenario arma 36 meses de factores y retornos de una
// accion con beta=1.5 a Mkt-RF y una "sorpresa" residual que varia mes a
// mes (nunca constante, para no toparse con matrices singulares ni con
// desvios estandar de residuos iguales a cero).
func syntheticFactorScenario(asOf time.Time, windowMonths int) (bars []sqlite.PriceBar, factorRows []sqlite.FactorRow) {
	const beta = 1.5

	mktRF := make([]float64, windowMonths)
	smb := make([]float64, windowMonths)
	hml := make([]float64, windowMonths)
	rf := make([]float64, windowMonths)
	stockReturns := make([]float64, windowMonths)
	for i := range windowMonths {
		mktRF[i] = 0.01 * float64(i%5-2)
		smb[i] = 0.005 * float64(i%3-1)
		hml[i] = 0.004 * float64((i+1)%4)
		rf[i] = 0.002
		surprise := 0.002 * float64((i*7)%11-5) // "ruido" deterministico, nunca constante
		stockReturns[i] = beta*mktRF[i] + rf[i] + surprise
	}

	bars = buildMonthlyBars(asOf, stockReturns)
	factorRows = factorRowsFromStockDates(bars, mktRF, smb, hml, rf)
	return bars, factorRows
}

// TestCompute_MatchesManualOLS reconstruye paso a paso lo que
// residualmomentum.Compute deberia hacer internamente (alinear accion y
// factores mes a mes, correr OLS sobre la ultima ventana, y resumir los
// ultimos residualMonths residuos) usando common.OLS directamente, y
// verifica que Compute() llegue al mismo numero. Esto no vuelve a probar
// la matematica de OLS (eso ya lo cubre el paquete common) — prueba que
// Compute arma la alineacion y la ventana correctamente.
func TestCompute_MatchesManualOLS(t *testing.T) {
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	const windowMonths = 36
	const residualMonths = 12

	bars, factorRows := syntheticFactorScenario(asOf, windowMonths)

	got, err := Compute(bars, factorRows, asOf, windowMonths, residualMonths)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	// Reconstruccion manual, independiente de la implementacion interna de Compute.
	closes := common.MonthEndCloses(bars, asOf)
	stockReturns := common.MonthlyReturns(closes)
	factorByDate := make(map[string]sqlite.FactorRow, len(factorRows))
	for _, fr := range factorRows {
		factorByDate[common.MonthKey(fr.Date)] = fr
	}

	var y []float64
	var x [][]float64
	for i, r := range stockReturns {
		fr, ok := factorByDate[common.MonthKey(closes[i+1].Date)]
		if !ok {
			continue
		}
		y = append(y, r-fr.Values["RF"])
		x = append(x, []float64{fr.Values["Mkt-RF"], fr.Values["SMB"], fr.Values["HML"]})
	}

	start := len(y) - windowMonths
	result, err := common.OLS(y[start:], x[start:])
	if err != nil {
		t.Fatalf("OLS manual fallo: %v", err)
	}
	recent := result.Residuals[len(result.Residuals)-residualMonths:]
	want := common.Sum(recent) / common.StdDev(recent)

	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Compute() = %v, want %v (calculo manual)", got, want)
	}
}

func TestCompute_InsufficientAlignedMonths(t *testing.T) {
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	returns := make([]float64, 10) // menos de los 36 meses que pide la ventana
	bars := buildMonthlyBars(asOf, returns)
	mktRF := make([]float64, 10)
	smb := make([]float64, 10)
	hml := make([]float64, 10)
	rf := make([]float64, 10)
	factorRows := factorRowsFromStockDates(bars, mktRF, smb, hml, rf)

	if _, err := Compute(bars, factorRows, asOf, 36, 12); err == nil {
		t.Error("esperaba error por pocos meses alineados con factores")
	}
}

func TestCompute_NoFactorDataAtAll(t *testing.T) {
	asOf := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	returns := make([]float64, 36)
	bars := buildMonthlyBars(asOf, returns)
	if _, err := Compute(bars, nil, asOf, 36, 12); err == nil {
		t.Error("esperaba error sin ningun factor para alinear")
	}
}

// El caso "desviacion estandar de residuos es cero" (otro error posible de
// Compute) no se prueba aca: con aritmetica de punto flotante, una
// regresion casi nunca da residuos EXACTAMENTE iguales entre si, asi que
// construir un caso confiable para ese branch especifico no es practico.
// Ver Mejoras_Pendientes.md (MEJ-020) sobre chequeos de igualdad exacta con
// floats en este tipo de validaciones.
