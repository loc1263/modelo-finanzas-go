// Package earningsmomentum implementa el modelo 8: Momentum de Fundamentos
// / Earnings Momentum (Chan, Jegadeesh y Lakonishok, 1996).
//
// Ordena por la sorpresa de ganancias (earnings): las acciones cuyas
// ganancias reportadas vienen sorprendiendo al alza a los analistas tienden
// a seguir haciendolo por un tiempo (post-earnings-announcement drift).
//
// Implementacion: version simplificada de las "Standardized Unexpected
// Earnings" (SUE) academicas. En vez de la sorpresa en dolares de EPS
// dividida por el precio o por la dispersion de estimados, se usa
// directamente el "surprisePercent" que reporta Yahoo (sorpresa como % del
// EPS estimado) en los ultimos trimestres disponibles (tipicamente 4): el
// score es el promedio de esas sorpresas, dividido por su desviacion
// estandar cuando hay suficiente variacion (mismo patron que el modelo 4,
// momentum residual).
package earningsmomentum

import (
	"fmt"

	"modelofinanzas/internal/models/common"
	"modelofinanzas/internal/storage/sqlite"
)

// Compute calcula el score de momentum de earnings a partir del historial
// trimestral de sorpresas de la accion.
func Compute(quarters []sqlite.EarningsQuarter) (float64, error) {
	if len(quarters) == 0 {
		return 0, fmt.Errorf("sin historial de earnings")
	}

	surprises := make([]float64, len(quarters))
	for i, q := range quarters {
		surprises[i] = q.SurprisePct
	}

	mean := common.Mean(surprises)
	sd := common.StdDev(surprises)
	if sd == 0 {
		return mean, nil
	}
	return mean / sd, nil
}
