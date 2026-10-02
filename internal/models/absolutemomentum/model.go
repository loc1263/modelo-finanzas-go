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

// Compute devuelve el retorno compuesto de la accion menos el retorno
// compuesto de la tasa libre de riesgo (RF), ambos sobre los ultimos
// lookbackMonths meses en que hay dato de la accion Y de RF al mismo tiempo.
//
// Alinear primero y recortar despues (en vez de exigir que los ultimos
// lookbackMonths meses de la accion tengan RF) importa en la practica: la
// Kenneth French Data Library suele publicar con 1-2 meses de demora
// respecto al precio mas reciente de la accion. Con ese desfasaje, exigir
// que la ventana de RF coincida exactamente con el mes mas reciente de la
// accion dejaria el modelo sin datos casi siempre. En cambio, igual que el
// modelo 4 (momentum residual), la ventana de "ultimos lookbackMonths
// meses" se toma sobre los meses ya alineados: si RF esta dos meses
// atrasado, la ventana completa se corre dos meses hacia atras, pero sigue
// siendo una ventana completa y comparable.
//
// factorRows debe venir de db.GetFactors("F-F_Research_Data_Factors", ...).
func Compute(bars []sqlite.PriceBar, factorRows []sqlite.FactorRow, asOf time.Time, lookbackMonths int) (float64, error) {
	closes := common.MonthEndCloses(bars, asOf)
	stockReturns := common.MonthlyReturns(closes)
	if len(stockReturns) == 0 {
		return 0, fmt.Errorf("sin retornos mensuales de la accion")
	}

	rfByMonth := make(map[string]float64, len(factorRows))
	for _, fr := range factorRows {
		if rf, ok := fr.Values["RF"]; ok {
			rfByMonth[common.MonthKey(fr.Date)] = rf
		}
	}

	var alignedStock, alignedRF []float64
	for i, r := range stockReturns {
		// stockReturns[i] es el retorno del mes closes[i+1].Date.
		if rf, ok := rfByMonth[common.MonthKey(closes[i+1].Date)]; ok {
			alignedStock = append(alignedStock, r)
			alignedRF = append(alignedRF, rf)
		}
	}

	if len(alignedStock) < lookbackMonths {
		return 0, fmt.Errorf("se necesitan %d meses alineados con RF, hay %d", lookbackMonths, len(alignedStock))
	}

	start := len(alignedStock) - lookbackMonths
	cumStock, cumRF := 1.0, 1.0
	for i := start; i < len(alignedStock); i++ {
		cumStock *= 1 + alignedStock[i]
		cumRF *= 1 + alignedRF[i]
	}

	return (cumStock - 1) - (cumRF - 1), nil
}
