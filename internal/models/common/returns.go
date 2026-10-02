// Package common tiene las funciones matematicas compartidas por los 5
// modelos de momentum: retornos mensuales/diarios, volatilidad y regresion OLS.
//
// Convencion de "12-1" usada en todo el paquete: para evitar el efecto de
// reversion de corto plazo del ultimo mes (parcial o recien cerrado), todos
// los calculos de momentum se anclan en el ultimo mes calendario COMPLETO
// anterior a asOf, nunca en el mes en curso. Por eso MonthEndCloses ya
// excluye el mes de asOf: un retorno "12 meses" calculado sobre esa serie es,
// por construccion, un "12-1".
package common

import (
	"fmt"
	"math"
	"sort"
	"time"

	"modelofinanzas/internal/storage/sqlite"
)

// MonthKey da una clave "YYYY-MM" para alinear una fecha de barra (dia
// especifico) con una fecha de factor Fama-French (siempre dia 1 del mes).
func MonthKey(t time.Time) string {
	return fmt.Sprintf("%04d-%02d", t.Year(), t.Month())
}

// BarsBetween devuelve las barras de bars cuya fecha esta en [start, end],
// ambos inclusive. bars debe venir ordenado ascendentemente por fecha.
func BarsBetween(bars []sqlite.PriceBar, start, end time.Time) []sqlite.PriceBar {
	out := make([]sqlite.PriceBar, 0, len(bars))
	for _, b := range bars {
		if (b.Date.After(start) || b.Date.Equal(start)) && (b.Date.Before(end) || b.Date.Equal(end)) {
			out = append(out, b)
		}
	}
	return out
}

// MonthClose es el precio de cierre ajustado del ultimo dia habil de un mes.
type MonthClose struct {
	Date     time.Time
	AdjClose float64
}

// MonthEndCloses agrupa las barras diarias por mes calendario y devuelve el
// cierre ajustado del ultimo dia de cada mes, en orden ascendente,
// EXCLUYENDO el mes calendario de asOf (que se considera incompleto).
func MonthEndCloses(bars []sqlite.PriceBar, asOf time.Time) []MonthClose {
	type key struct{ y, m int }
	currentMonth := key{asOf.Year(), int(asOf.Month())}

	latest := map[key]sqlite.PriceBar{}
	for _, b := range bars {
		k := key{b.Date.Year(), int(b.Date.Month())}
		if k == currentMonth {
			continue
		}
		if existing, ok := latest[k]; !ok || b.Date.After(existing.Date) {
			latest[k] = b
		}
	}

	keys := make([]key, 0, len(latest))
	for k := range latest {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].y != keys[j].y {
			return keys[i].y < keys[j].y
		}
		return keys[i].m < keys[j].m
	})

	out := make([]MonthClose, 0, len(keys))
	for _, k := range keys {
		b := latest[k]
		out = append(out, MonthClose{Date: b.Date, AdjClose: b.AdjClose})
	}
	return out
}

// ExcludeCurrentMonth devuelve las barras de bars cuyo mes calendario es
// anterior al de asOf (se usa para que los calculos diarios, igual que los
// mensuales, nunca miren el mes en curso/incompleto).
func ExcludeCurrentMonth(bars []sqlite.PriceBar, asOf time.Time) []sqlite.PriceBar {
	out := make([]sqlite.PriceBar, 0, len(bars))
	for _, b := range bars {
		if b.Date.Year() == asOf.Year() && b.Date.Month() == asOf.Month() {
			continue
		}
		out = append(out, b)
	}
	return out
}

// LastNBars devuelve las ultimas n barras de bars (o todas si hay menos de n).
func LastNBars(bars []sqlite.PriceBar, n int) []sqlite.PriceBar {
	if len(bars) <= n {
		return bars
	}
	return bars[len(bars)-n:]
}

// MonthlyReturns calcula el retorno simple mes a mes de una serie de cierres.
func MonthlyReturns(closes []MonthClose) []float64 {
	if len(closes) < 2 {
		return nil
	}
	out := make([]float64, 0, len(closes)-1)
	for i := 1; i < len(closes); i++ {
		prev := closes[i-1].AdjClose
		if prev == 0 {
			out = append(out, 0)
			continue
		}
		out = append(out, closes[i].AdjClose/prev-1)
	}
	return out
}

// TrailingReturn devuelve el retorno acumulado de los ultimos `months` meses
// terminando en el ultimo mes completo disponible en closes (que ya es un
// "-1" respecto a asOf por construccion de MonthEndCloses).
func TrailingReturn(closes []MonthClose, months int) (float64, error) {
	if len(closes) < months+1 {
		return 0, fmt.Errorf("se necesitan al menos %d meses de historia, hay %d", months+1, len(closes))
	}
	last := closes[len(closes)-1].AdjClose
	base := closes[len(closes)-1-months].AdjClose
	if base == 0 {
		return 0, fmt.Errorf("precio base es cero")
	}
	return last/base - 1, nil
}

// DailyReturns calcula el retorno simple diario de una serie de barras ya
// ordenada ascendentemente por fecha.
func DailyReturns(bars []sqlite.PriceBar) []float64 {
	if len(bars) < 2 {
		return nil
	}
	out := make([]float64, 0, len(bars)-1)
	for i := 1; i < len(bars); i++ {
		prev := bars[i-1].AdjClose
		if prev == 0 {
			out = append(out, 0)
			continue
		}
		out = append(out, bars[i].AdjClose/prev-1)
	}
	return out
}

// MaxAdjClose devuelve el maximo cierre ajustado dentro de bars.
func MaxAdjClose(bars []sqlite.PriceBar) float64 {
	max := math.Inf(-1)
	for _, b := range bars {
		if b.AdjClose > max {
			max = b.AdjClose
		}
	}
	return max
}

// LastAdjClose devuelve el cierre ajustado de la barra mas reciente.
func LastAdjClose(bars []sqlite.PriceBar) float64 {
	if len(bars) == 0 {
		return 0
	}
	return bars[len(bars)-1].AdjClose
}

// Mean calcula el promedio de una serie.
func Mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// StdDev calcula la desviacion estandar muestral (n-1) de una serie.
func StdDev(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := Mean(xs)
	sumSq := 0.0
	for _, x := range xs {
		d := x - m
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(xs)-1))
}

// AnnualizedVol anualiza la desviacion estandar de una serie de retornos
// periodicos, dado el numero de periodos por anio (252 para diario, 12 para mensual).
func AnnualizedVol(periodicReturns []float64, periodsPerYear float64) float64 {
	return StdDev(periodicReturns) * math.Sqrt(periodsPerYear)
}

// MinMeaningfulVol es el piso minimo de volatilidad anualizada (en fraccion,
// ej. 0.01 = 1%) por debajo del cual un modelo que divide por volatilidad
// debe tratarla como si fuera cero, en vez de dividir por un numero casi nulo.
// Una accion con volatilidad real pero muy baja en la ventana (no exactamente
// cero) puede producir un score desproporcionado que es un artefacto numerico,
// no un reflejo real de momentum.
const MinMeaningfulVol = 0.01

// Sum suma los elementos de una serie.
func Sum(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s
}
