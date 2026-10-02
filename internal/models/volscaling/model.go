// Package volscaling implementa el modelo 5: Escalado por volatilidad
// (Barroso y Santa-Clara, 2015). No elige acciones: ajusta cuanto invertir
// en la estrategia de momentum segun que tan volatil estuvo la cartera.
//
// Simplificacion deliberada: el factor se limita a 1.0 como maximo (sin
// apalancamiento). El trabajo original de Barroso-Santa Clara permite
// apalancar cuando la volatilidad realizada cae por debajo del objetivo;
// aqui se usa como una capa de proteccion que solo puede REDUCIR exposicion,
// nunca aumentarla mas alla del 100%, para mantener el modelo conservador.
package volscaling

import (
	"errors"

	"modelofinanzas/internal/models/common"
)

// Result es la salida del modelo de escalado por volatilidad.
type Result struct {
	RealizedVolAnnual float64
	ScalingFactor     float64 // en [0, 1]: fraccion recomendada de exposicion
}

// Compute calcula el factor de escalado a partir de los retornos diarios de
// la cartera (o universo) de referencia y la volatilidad anual objetivo.
func Compute(portfolioDailyReturns []float64, targetVolAnnual float64) (Result, error) {
	vol := common.AnnualizedVol(portfolioDailyReturns, 252)
	if vol == 0 {
		return Result{}, errZeroVol
	}

	factor := targetVolAnnual / vol
	if factor > 1 {
		factor = 1
	}
	if factor < 0 {
		factor = 0
	}

	return Result{RealizedVolAnnual: vol, ScalingFactor: factor}, nil
}

var errZeroVol = errors.New("volatilidad realizada es cero o datos insuficientes")
