// Package absolutemomentum implementa el modelo 7: Momentum Absoluto /
// Time-Series Momentum (Moskowitz, Ooi y Pedersen, 2012).
//
// A diferencia del momentum 12-1 clasico (que es "relativo": compara una
// accion contra las demas del universo), el momentum de esta familia es
// "absoluto" en el sentido de TSMOM: compara el retorno propio de la accion
// contra un umbral, en vez de contra sus pares. Aqui se usa como umbral la
// tasa libre de riesgo compuesta durante la misma ventana (factor RF de
// Fama-French), que ya se descarga para el modelo de momentum residual.
//
// Simplificacion respecto al paper original: TSMOM original arma una
// cartera long/short con el signo del retorno pasado, escalada por
// volatilidad, para cada instrumento por separado. Como este proyecto
// produce un ranking cruzado de acciones (no una cartera apalancada), se
// usa el exceso de retorno sobre la tasa libre de riesgo como score: mide
// lo mismo que el signo/umbral del paper (¿la accion le gano al "no hacer
// nada"?) de forma comparable entre acciones.
package absolutemomentum

import (
	"fmt"
	"time"

	"modelofinanzas/internal/models/common"
	"modelofinanzas/internal/storage/sqlite"
)

// Compute devuelve el retorno de lookbackMonths meses menos el retorno
// compuesto de la tasa libre de riesgo (RF) en la misma ventana.
//
// factorRows debe venir de db.GetFactors("F-F_Research_Data_Factors", ...)
// cubriendo al menos lookbackMonths meses hasta el mes completo anterior a asOf.
func Compute(bars []sqlite.PriceBar, factorRows []sqlite.FactorRow, asOf time.Time, lookbackMonths int) (float64, error) {
	closes := common.MonthEndCloses(bars, asOf)

	rTotal, err := common.TrailingReturn(closes, lookbackMonths)
	if err != nil {
		return 0, fmt.Errorf("retorno de %d meses: %w", lookbackMonths, err)
	}

	n := len(closes)
	if n < lookbackMonths+1 {
		return 0, fmt.Errorf("se necesitan al menos %d meses de historia, hay %d", lookbackMonths+1, n)
	}

	rfByMonth := make(map[string]float64, len(factorRows))
	for _, fr := range factorRows {
		if rf, ok := fr.Values["RF"]; ok {
			rfByMonth[common.MonthKey(fr.Date)] = rf
		}
	}

	cumRF := 1.0
	matched := 0
	for i := n - lookbackMonths; i < n; i++ {
		if rf, ok := rfByMonth[common.MonthKey(closes[i].Date)]; ok {
			cumRF *= 1 + rf
			matched++
		}
	}
	if matched == 0 {
		return 0, fmt.Errorf("sin datos de RF alineados con la ventana de la accion")
	}

	return rTotal - (cumRF - 1), nil
}
