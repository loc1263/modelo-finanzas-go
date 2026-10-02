// Package voladjusted implementa el modelo 3: Momentum ajustado por
// volatilidad (metodo MSCI, simplificado). Divide el retorno de 12-1 y de
// 6-1 meses por la volatilidad de la accion, y promedia ambos.
//
// Simplificacion respecto al indice MSCI real: la volatilidad se calcula
// como la desviacion estandar anualizada de los retornos diarios en una
// ventana configurable (volLookbackDays), en vez de 3 anios de retornos
// semanales. Es una aproximacion razonable y documentada; se puede ajustar
// via configs/settings.yaml.
package voladjusted

import (
	"fmt"

	"modelofinanzas/internal/models/common"
	"modelofinanzas/internal/storage/sqlite"
	"time"
)

// Compute devuelve el promedio de (retorno 12-1 / vol) y (retorno 6-1 / vol).
func Compute(bars []sqlite.PriceBar, asOf time.Time, longMonths, shortMonths, volLookbackDays int) (float64, error) {
	closes := common.MonthEndCloses(bars, asOf)

	rLong, err := common.TrailingReturn(closes, longMonths)
	if err != nil {
		return 0, fmt.Errorf("retorno %d-1 meses: %w", longMonths, err)
	}
	rShort, err := common.TrailingReturn(closes, shortMonths)
	if err != nil {
		return 0, fmt.Errorf("retorno %d-1 meses: %w", shortMonths, err)
	}

	dailyBars := common.ExcludeCurrentMonth(bars, asOf)
	window := common.LastNBars(dailyBars, volLookbackDays)
	vol := common.AnnualizedVol(common.DailyReturns(window), 252)
	if vol < common.MinMeaningfulVol {
		return 0, fmt.Errorf("volatilidad cero, insuficiente o por debajo del piso minimo")
	}

	return ((rLong / vol) + (rShort / vol)) / 2, nil
}
