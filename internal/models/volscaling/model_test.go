package volscaling

import (
	"math"
	"testing"
)

func TestCompute_CapsAtOne(t *testing.T) {
	// Volatilidad realizada baja (pero por encima de common.MinMeaningfulVol)
	// -> el factor objetivo/realizada daria mas de 1, pero debe quedar
	// topeado en 1 (sin apalancamiento).
	returns := []float64{0.003, -0.003, 0.003, -0.003, 0.003}
	result, err := Compute(returns, 0.50) // objetivo absurdamente alto a proposito
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if result.ScalingFactor != 1 {
		t.Errorf("ScalingFactor = %v, want 1 (topeado)", result.ScalingFactor)
	}
}

func TestCompute_ReducesExposureWhenVolatile(t *testing.T) {
	returns := []float64{0.05, -0.05, 0.06, -0.04, 0.05, -0.06}
	target := 0.12
	result, err := Compute(returns, target)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if result.ScalingFactor <= 0 || result.ScalingFactor >= 1 {
		t.Errorf("ScalingFactor = %v, want algo en (0,1) con esta volatilidad alta", result.ScalingFactor)
	}
	want := target / result.RealizedVolAnnual
	if math.Abs(result.ScalingFactor-want) > 1e-9 {
		t.Errorf("ScalingFactor = %v, want %v (objetivo/realizada)", result.ScalingFactor, want)
	}
}

func TestCompute_ZeroVolatilityErrors(t *testing.T) {
	if _, err := Compute([]float64{0, 0, 0, 0}, 0.12); err == nil {
		t.Error("esperaba error por volatilidad cero")
	}
}

// TestCompute_BelowMinMeaningfulVolErrors cubre MEJ-020: una volatilidad
// real pero muy por debajo del piso minimo debe tratarse igual que cero.
func TestCompute_BelowMinMeaningfulVolErrors(t *testing.T) {
	returns := []float64{0.0001, -0.0001, 0.0001, -0.0001, 0.0001}
	if _, err := Compute(returns, 0.12); err == nil {
		t.Error("esperaba error por volatilidad por debajo del piso minimo")
	}
}
