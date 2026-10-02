// Package sectormomentum implementa el modelo 10: Momentum Secundario /
// Sectorial (Moskowitz y Grinblatt, 1999).
//
// La idea original es que el momentum de una industria/sector es mas fuerte
// y persistente que el momentum de una accion individual. En vez de
// calcular un "sector" sintetico promediando las pocas mega-caps de este
// universo que comparten rubro (una muestra chica y sesgada), este modelo
// reusa el ETF sectorial de Vanguard correspondiente (ya descargado para el
// analisis de ETFs) como proxy del sector completo, y le asigna a cada
// accion el momentum 12-1 de SU sector. Dos acciones del mismo sector
// comparten el mismo score: este modelo elige sectores, no acciones dentro
// de un sector.
//
// El mapeo accion -> ETF sectorial esta en la columna "sector_etf" de
// configs/megacap_stocks.csv.
package sectormomentum

import (
	"time"

	"modelofinanzas/internal/models/momentum12_1"
	"modelofinanzas/internal/storage/sqlite"
)

// Compute es el momentum 12-1 (ver momentum12_1.Compute) del ETF sectorial
// que representa a la accion, no un calculo propio: se expone con su
// propio nombre para que el modelo quede explicito en el pipeline y en el reporte.
func Compute(sectorETFBars []sqlite.PriceBar, asOf time.Time, lookbackMonths int) (float64, error) {
	return momentum12_1.Compute(sectorETFBars, asOf, lookbackMonths)
}
