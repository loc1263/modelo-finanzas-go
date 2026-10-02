// Package markowitz implementa una simulacion Monte Carlo de carteras
// media-varianza (Markowitz, 1952, "Portfolio Selection", Journal of
// Finance), long-only y sin apalancamiento.
//
// A diferencia de los 11 modelos de momentum del proyecto (que rankean
// acciones una por una), esto no es un modelo de momentum: no mide ninguna
// señal de tendencia, construye una cartera. Por eso se documenta y se
// muestra en el reporte como una seccion aparte ("Construccion de
// cartera"), no como un modelo mas de la lista 1-11 — ver
// Modelos_de_Momentum.md.
//
// Simplificacion deliberada: en vez de resolver el problema de
// optimizacion cuadratica de forma analitica (inversion de la matriz de
// covarianza), se generan carteras aleatorias long-only y se evalua cada
// una, quedandose con la de mayor Sharpe ratio. Es el enfoque de
// "simulacion de Markowitz" mas comun en la practica para explorar la
// frontera eficiente sin depender de un solver externo.
package markowitz

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"

	"modelofinanzas/internal/models/common"
)

// Result es la cartera de mayor Sharpe ratio encontrada por la simulacion.
type Result struct {
	Weights        map[string]float64 // symbol -> peso en la cartera optima; suma 1
	ExpectedReturn float64            // retorno anual esperado de la cartera optima
	Volatility     float64            // volatilidad (desvio estandar) anual de la cartera optima
	SharpeRatio    float64
}

// Simulate genera numSimulations carteras aleatorias long-only (pesos en
// [0,1], suman 1) sobre los simbolos de returnsBySymbol, y devuelve la de
// mayor Sharpe ratio: (retorno anual − riskFreeAnnual) / volatilidad anual.
//
// returnsBySymbol debe tener la misma cantidad de retornos diarios para
// cada simbolo, ya alineados por fecha (ver pipeline.alignedDailyReturns).
//
// rng se recibe como parametro en vez de usar una fuente global para que
// el caller pueda sembrarlo de forma deterministica segun el dia de la
// corrida: con la misma semilla, reejecutar el pipeline el mismo dia
// encuentra siempre la misma cartera optima, en vez de una distinta en
// cada corrida.
func Simulate(returnsBySymbol map[string][]float64, numSimulations int, riskFreeAnnual float64, rng *rand.Rand) (Result, error) {
	symbols := make([]string, 0, len(returnsBySymbol))
	for s := range returnsBySymbol {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols) // orden deterministico: un map en Go no lo garantiza

	n := len(symbols)
	if n < 2 {
		return Result{}, fmt.Errorf("se necesitan al menos 2 simbolos para simular una cartera, hay %d", n)
	}

	numDays := len(returnsBySymbol[symbols[0]])
	if numDays < 2 {
		return Result{}, fmt.Errorf("se necesitan al menos 2 dias de retornos, hay %d", numDays)
	}
	for _, s := range symbols {
		if len(returnsBySymbol[s]) != numDays {
			return Result{}, fmt.Errorf("retornos no alineados: %s tiene %d dias, %s tiene %d", symbols[0], numDays, s, len(returnsBySymbol[s]))
		}
	}

	meanAnnual := make([]float64, n)
	for i, s := range symbols {
		meanAnnual[i] = common.Mean(returnsBySymbol[s]) * 252
	}
	cov := annualizedCovarianceMatrix(symbols, returnsBySymbol)

	var best Result
	bestSharpe := math.Inf(-1)

	for sim := 0; sim < numSimulations; sim++ {
		weights := randomSimplexWeights(rng, n)

		ret := dot(weights, meanAnnual)
		vol := math.Sqrt(quadForm(weights, cov))
		if vol < common.MinMeaningfulVol {
			// Volatilidad numericamente nula: no es una cartera real
			// aprovechable (ver common.MinMeaningfulVol / MEJ-020), se
			// descarta en vez de dividir por un numero casi cero.
			continue
		}

		sharpe := (ret - riskFreeAnnual) / vol
		if sharpe > bestSharpe {
			bestSharpe = sharpe
			w := make(map[string]float64, n)
			for i, s := range symbols {
				w[s] = weights[i]
			}
			best = Result{Weights: w, ExpectedReturn: ret, Volatility: vol, SharpeRatio: sharpe}
		}
	}

	if best.Weights == nil {
		return Result{}, errors.New("ninguna cartera simulada supero el piso minimo de volatilidad")
	}

	return best, nil
}

// randomSimplexWeights genera un vector de n pesos long-only que suman 1,
// distribuidos uniformemente sobre el simplex (Dirichlet(1,...,1)).
//
// Importante: normalizar n valores Uniforme(0,1) por su suma NO da una
// distribucion uniforme sobre el simplex (sesga hacia carteras mas
// repartidas). La forma correcta es tomar n variables Exponencial(1) —
// obtenidas aca por la transformada inversa, -log(U) — y normalizarlas por
// su suma.
func randomSimplexWeights(rng *rand.Rand, n int) []float64 {
	w := make([]float64, n)
	sum := 0.0
	for i := range w {
		// 1-rng.Float64() en vez de rng.Float64() para evitar log(0) en el
		// caso extremo (poco probable pero posible) de que Float64() devuelva
		// exactamente 0: Float64() devuelve [0,1), 1-Float64() devuelve (0,1].
		w[i] = -math.Log(1 - rng.Float64())
		sum += w[i]
	}
	for i := range w {
		w[i] /= sum
	}
	return w
}

// annualizedCovarianceMatrix calcula la matriz de covarianza muestral (n-1)
// de los retornos diarios de cada simbolo, anualizada (×252, igual que
// common.AnnualizedVol anualiza el desvio estandar con ×sqrt(252): la
// varianza escala con el tiempo, no con su raiz).
func annualizedCovarianceMatrix(symbols []string, returnsBySymbol map[string][]float64) [][]float64 {
	n := len(symbols)
	numDays := len(returnsBySymbol[symbols[0]])

	means := make([]float64, n)
	for i, s := range symbols {
		means[i] = common.Mean(returnsBySymbol[s])
	}

	cov := make([][]float64, n)
	for i := range cov {
		cov[i] = make([]float64, n)
	}

	for i := 0; i < n; i++ {
		ri := returnsBySymbol[symbols[i]]
		for j := i; j < n; j++ {
			rj := returnsBySymbol[symbols[j]]
			sum := 0.0
			for d := 0; d < numDays; d++ {
				sum += (ri[d] - means[i]) * (rj[d] - means[j])
			}
			c := sum / float64(numDays-1) * 252
			cov[i][j] = c
			cov[j][i] = c
		}
	}

	return cov
}

func dot(a, b []float64) float64 {
	sum := 0.0
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// quadForm calcula w^T * m * w.
func quadForm(w []float64, m [][]float64) float64 {
	sum := 0.0
	for i := range w {
		for j := range w {
			sum += w[i] * m[i][j] * w[j]
		}
	}
	return sum
}
