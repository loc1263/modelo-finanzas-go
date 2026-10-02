// Package max52w implementa el modelo 2: Maximo de 52 semanas (George y
// Hwang, 2004). Ordena por precio actual / maximo del ultimo anio.
package max52w

import (
	"fmt"

	"modelofinanzas/internal/models/common"
	"modelofinanzas/internal/storage/sqlite"
)

// Compute devuelve current/max donde max es el maximo cierre ajustado dentro
// de las ultimas lookbackDays barras (tipicamente ~252 dias de trading = 52
// semanas) y current es el cierre ajustado mas reciente de bars.
func Compute(bars []sqlite.PriceBar, lookbackDays int) (float64, error) {
	if len(bars) == 0 {
		return 0, fmt.Errorf("sin datos de precios")
	}

	current := common.LastAdjClose(bars)
	window := common.LastNBars(bars, lookbackDays)
	max := common.MaxAdjClose(window)
	if max <= 0 {
		return 0, fmt.Errorf("maximo de 52 semanas invalido")
	}

	return current / max, nil
}
