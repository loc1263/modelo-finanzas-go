// Package residualmomentum implementa el modelo 4: Momentum residual
// (Blitz, Huij y Martens, 2011). Para cada accion se regresionan sus
// retornos mensuales excedentes contra los factores de Fama-French
// (Mkt-RF, SMB, HML) en una ventana de 36 meses, y se rankea por la suma de
// los residuos de los ultimos 12 meses de esa ventana dividida por su
// desviacion estandar.
//
// Como la ventana de regresion ya termina en el ultimo mes calendario
// completo anterior a asOf (ver paquete common), el mes en curso nunca
// entra en el calculo: es el equivalente "12-1" dentro de este modelo.
package residualmomentum

import (
	"fmt"
	"time"

	"modelofinanzas/internal/models/common"
	"modelofinanzas/internal/storage/sqlite"
)

// FactorNames son las columnas del dataset F-F_Research_Data_Factors usadas
// como regresores (RF se usa aparte para calcular el retorno excedente).
var FactorNames = []string{"Mkt-RF", "SMB", "HML"}

// Compute devuelve el score de momentum residual de una accion.
//
// factorRows debe venir de db.GetFactors("F-F_Research_Data_Factors", ...)
// cubriendo al menos regressionWindowMonths meses hasta el mes completo
// anterior a asOf.
func Compute(bars []sqlite.PriceBar, factorRows []sqlite.FactorRow, asOf time.Time, regressionWindowMonths, residualMonths int) (float64, error) {
	closes := common.MonthEndCloses(bars, asOf)
	stockReturns := common.MonthlyReturns(closes)
	if len(stockReturns) == 0 {
		return 0, fmt.Errorf("sin retornos mensuales de la accion")
	}
	// stockReturns[i] es el retorno del mes closes[i+1].Date.
	stockDates := make([]time.Time, len(stockReturns))
	for i := range stockReturns {
		stockDates[i] = closes[i+1].Date
	}

	factorByMonth := make(map[string]sqlite.FactorRow, len(factorRows))
	for _, fr := range factorRows {
		factorByMonth[common.MonthKey(fr.Date)] = fr
	}

	var y []float64
	var x [][]float64
	var alignedDates []time.Time
	for i, d := range stockDates {
		fr, ok := factorByMonth[common.MonthKey(d)]
		if !ok {
			continue
		}
		rf, hasRF := fr.Values["RF"]
		mktRF, hasMkt := fr.Values["Mkt-RF"]
		smb, hasSMB := fr.Values["SMB"]
		hml, hasHML := fr.Values["HML"]
		if !hasRF || !hasMkt || !hasSMB || !hasHML {
			continue
		}

		excessReturn := stockReturns[i] - rf
		y = append(y, excessReturn)
		x = append(x, []float64{mktRF, smb, hml})
		alignedDates = append(alignedDates, d)
	}

	if len(y) < regressionWindowMonths {
		return 0, fmt.Errorf("se necesitan %d meses alineados con factores, hay %d", regressionWindowMonths, len(y))
	}

	// Ultima ventana de regressionWindowMonths meses.
	start := len(y) - regressionWindowMonths
	result, err := common.OLS(y[start:], x[start:])
	if err != nil {
		return 0, fmt.Errorf("regresion OLS: %w", err)
	}

	if residualMonths > len(result.Residuals) {
		return 0, fmt.Errorf("residualMonths (%d) mayor que la ventana de residuos (%d)", residualMonths, len(result.Residuals))
	}
	recentResiduals := result.Residuals[len(result.Residuals)-residualMonths:]

	sd := common.StdDev(recentResiduals)
	if sd == 0 {
		return 0, fmt.Errorf("desviacion estandar de residuos es cero")
	}

	return common.Sum(recentResiduals) / sd, nil
}
