// Package frogpan implementa el modelo 9: Momentum Consecutivo /
// Frog-in-the-Pan (Da, Peng y Engelberg; Da, Gurun y Warachka, 2014).
//
// La idea es que el mercado reacciona mas lento (y el momentum dura mas)
// cuando la informacion llega de forma continua y gradual, en vez de en
// pocos saltos grandes ("lumpy"). Se mide con el "Information Discreteness"
// (ID) de Da-Gurun-Warachka: ID = signo(retorno) * (%dias negativos -
// %dias positivos) en la ventana de formacion. Un ID bajo (negativo) indica
// una tendencia pareja y continua (el "sapo que no nota que el agua se
// calienta" — de ahi el nombre); un ID alto indica un retorno "a saltos".
//
// El score devuelto es -ID: mas alto es mejor (tendencia mas continua y,
// segun el paper, con momentum mas persistente).
package frogpan

import (
	"fmt"
	"time"

	"modelofinanzas/internal/models/common"
	"modelofinanzas/internal/storage/sqlite"
)

// Compute calcula -ID sobre la ventana de los ultimos lookbackMonths meses
// completos anteriores a asOf (la misma ventana que usaria un momentum
// 12-1 con lookbackMonths=12).
func Compute(bars []sqlite.PriceBar, asOf time.Time, lookbackMonths int) (float64, error) {
	closes := common.MonthEndCloses(bars, asOf)

	totalReturn, err := common.TrailingReturn(closes, lookbackMonths)
	if err != nil {
		return 0, fmt.Errorf("retorno de %d meses: %w", lookbackMonths, err)
	}

	n := len(closes)
	start := closes[n-1-lookbackMonths].Date
	end := closes[n-1].Date
	window := common.BarsBetween(bars, start, end)

	daily := common.DailyReturns(window)
	if len(daily) == 0 {
		return 0, fmt.Errorf("sin retornos diarios en la ventana de formacion")
	}

	pos, neg := 0, 0
	for _, r := range daily {
		switch {
		case r > 0:
			pos++
		case r < 0:
			neg++
		}
	}
	total := pos + neg
	if total == 0 {
		return 0, fmt.Errorf("sin variacion de precio en la ventana de formacion")
	}

	pctPos := float64(pos) / float64(total)
	pctNeg := float64(neg) / float64(total)

	sign := 0.0
	switch {
	case totalReturn > 0:
		sign = 1
	case totalReturn < 0:
		sign = -1
	}

	id := sign * (pctNeg - pctPos)
	return -id, nil
}
