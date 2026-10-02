// Package accelmomentum implementa el modelo 6: Momentum Acelerado / Doble
// Velocidad (ap Gwilym, Clare, O'Sullivan y Walmsley, 2010).
//
// Compara el retorno reciente (corto plazo) contra el retorno "esperado"
// para ese mismo plazo si la tendencia de los ultimos `longMonths` meses se
// hubiera mantenido constante. Un valor positivo indica que el momentum se
// esta acelerando (el tramo reciente rinde mas que el promedio del tramo
// largo); un valor negativo indica que se esta desacelerando, aun si el
// retorno total todavia es positivo.
package accelmomentum

import (
	"fmt"

	"modelofinanzas/internal/models/common"
	"modelofinanzas/internal/storage/sqlite"
	"time"
)

// Compute devuelve retorno(shortMonths) menos la "cuota" de shortMonths
// dentro del retorno(longMonths), es decir: rShort - rLong*(shortMonths/longMonths).
func Compute(bars []sqlite.PriceBar, asOf time.Time, shortMonths, longMonths int) (float64, error) {
	if shortMonths <= 0 || longMonths <= shortMonths {
		return 0, fmt.Errorf("shortMonths debe ser > 0 y menor que longMonths")
	}

	closes := common.MonthEndCloses(bars, asOf)

	rShort, err := common.TrailingReturn(closes, shortMonths)
	if err != nil {
		return 0, fmt.Errorf("retorno de %d meses: %w", shortMonths, err)
	}
	rLong, err := common.TrailingReturn(closes, longMonths)
	if err != nil {
		return 0, fmt.Errorf("retorno de %d meses: %w", longMonths, err)
	}

	expectedShort := rLong * (float64(shortMonths) / float64(longMonths))
	return rShort - expectedShort, nil
}
