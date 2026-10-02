package markowitz

import (
	"math"
	"math/rand"
	"testing"

	"modelofinanzas/internal/models/common"
)

// syntheticReturns genera n dias de retornos diarios alrededor de dailyMean
// con ruido alternante +-noise (nunca constante, para que la volatilidad
// sea siempre distinta de cero), igual que el patron ya usado en
// internal/models/voladjusted/model_test.go.
func syntheticReturns(n int, dailyMean, noise float64) []float64 {
	out := make([]float64, n)
	for i := range n {
		if i%2 == 0 {
			out[i] = dailyMean + noise
		} else {
			out[i] = dailyMean - noise
		}
	}
	return out
}

func TestSimulate_WeightsAreLongOnlyAndSumToOne(t *testing.T) {
	returns := map[string][]float64{
		"AAA": syntheticReturns(300, 0.001, 0.01),
		"BBB": syntheticReturns(300, 0.0005, 0.02),
		"CCC": syntheticReturns(300, 0.0008, 0.015),
	}
	rng := rand.New(rand.NewSource(1))

	result, err := Simulate(returns, 2000, 0.02, rng)
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}

	sum := 0.0
	for symbol, w := range result.Weights {
		if w < 0 {
			t.Errorf("peso negativo para %s: %v (la cartera debe ser long-only)", symbol, w)
		}
		sum += w
	}
	if math.Abs(sum-1) > 1e-9 {
		t.Errorf("suma de pesos = %v, want 1", sum)
	}
}

func TestSimulate_IsDeterministicGivenSeed(t *testing.T) {
	returns := map[string][]float64{
		"AAA": syntheticReturns(300, 0.001, 0.01),
		"BBB": syntheticReturns(300, 0.0005, 0.02),
		"CCC": syntheticReturns(300, 0.0008, 0.015),
	}

	r1, err := Simulate(returns, 2000, 0.02, rand.New(rand.NewSource(42)))
	if err != nil {
		t.Fatalf("primera corrida: %v", err)
	}
	r2, err := Simulate(returns, 2000, 0.02, rand.New(rand.NewSource(42)))
	if err != nil {
		t.Fatalf("segunda corrida: %v", err)
	}

	if r1.SharpeRatio != r2.SharpeRatio {
		t.Errorf("SharpeRatio no es deterministico: %v vs %v", r1.SharpeRatio, r2.SharpeRatio)
	}
	for symbol, w1 := range r1.Weights {
		if w2 := r2.Weights[symbol]; w1 != w2 {
			t.Errorf("peso de %s no es deterministico: %v vs %v", symbol, w1, w2)
		}
	}
}

// TestSimulate_FindsBetterSharpeThanEqualWeight cubre la invariante central
// del modelo: con suficientes simulaciones, la cartera encontrada no deberia
// ser peor (en Sharpe ratio) que la cartera equal-weight ingenua, sobre
// datos sinteticos donde claramente conviene pesar mas un activo que otro.
func TestSimulate_FindsBetterSharpeThanEqualWeight(t *testing.T) {
	// AAA: buen retorno, poco ruido. BBB: retorno negativo, mucho ruido.
	// Una buena cartera deberia concentrarse en AAA.
	returns := map[string][]float64{
		"AAA": syntheticReturns(400, 0.002, 0.005),
		"BBB": syntheticReturns(400, -0.001, 0.02),
	}
	rng := rand.New(rand.NewSource(7))

	result, err := Simulate(returns, 20000, 0.0, rng)
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}

	equalWeightReturn := 0.0
	for _, rets := range returns {
		equalWeightReturn += common.Mean(rets) * 252
	}
	equalWeightReturn /= float64(len(returns))

	if result.Weights["AAA"] <= result.Weights["BBB"] {
		t.Errorf("se esperaba mayor peso en AAA (mejor retorno, menos ruido): AAA=%v BBB=%v", result.Weights["AAA"], result.Weights["BBB"])
	}
	if result.ExpectedReturn < equalWeightReturn {
		t.Errorf("ExpectedReturn de la cartera optima (%v) es peor que el de equal-weight (%v)", result.ExpectedReturn, equalWeightReturn)
	}
}

func TestSimulate_ErrorsOnTooFewSymbols(t *testing.T) {
	returns := map[string][]float64{"AAA": syntheticReturns(300, 0.001, 0.01)}
	if _, err := Simulate(returns, 1000, 0.02, rand.New(rand.NewSource(1))); err == nil {
		t.Error("esperaba error con un solo simbolo")
	}
}

func TestSimulate_ErrorsOnMismatchedLengths(t *testing.T) {
	returns := map[string][]float64{
		"AAA": syntheticReturns(300, 0.001, 0.01),
		"BBB": syntheticReturns(250, 0.001, 0.01),
	}
	if _, err := Simulate(returns, 1000, 0.02, rand.New(rand.NewSource(1))); err == nil {
		t.Error("esperaba error con series de retornos de distinta longitud")
	}
}

func TestSimulate_ErrorsWhenAllPortfoliosBelowVolFloor(t *testing.T) {
	zero := make([]float64, 300) // retornos diarios constantes en cero: volatilidad cero para cualquier cartera
	returns := map[string][]float64{"AAA": zero, "BBB": zero}
	if _, err := Simulate(returns, 1000, 0.02, rand.New(rand.NewSource(1))); err == nil {
		t.Error("esperaba error: ninguna cartera deberia superar el piso minimo de volatilidad")
	}
}
