// Package momentum12_1 implementa el modelo 1: Momentum 12-1 clasico
// (Jegadeesh y Titman, 1993). Ordena por el retorno de los ultimos 12 meses,
// sin contar el ultimo.
package momentum12_1

import (
	"time"

	"modelofinanzas/internal/models/common"
	"modelofinanzas/internal/storage/sqlite"
)

// Compute devuelve el retorno acumulado de los 12 meses completos previos a
// asOf (el mes de asOf se excluye por construccion, ver paquete common).
func Compute(bars []sqlite.PriceBar, asOf time.Time, lookbackMonths int) (float64, error) {
	closes := common.MonthEndCloses(bars, asOf)
	return common.TrailingReturn(closes, lookbackMonths)
}
