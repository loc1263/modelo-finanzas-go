package earningsmomentum

import (
	"math"
	"testing"

	"modelofinanzas/internal/storage/sqlite"
)

func TestCompute_MeanOverStdDev(t *testing.T) {
	quarters := []sqlite.EarningsQuarter{
		{SurprisePct: 0.02},
		{SurprisePct: 0.04},
		{SurprisePct: 0.06},
		{SurprisePct: 0.08},
	}
	got, err := Compute(quarters)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	mean := 0.05
	// Desvio estandar muestral (n-1) de {0.02,0.04,0.06,0.08}.
	sd := 0.02581988897
	want := mean / sd
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("Compute() = %v, want %v", got, want)
	}
}

func TestCompute_ConstantSurpriseReturnsJustTheMean(t *testing.T) {
	// Sorpresas identicas -> desvio estandar 0 -> Compute debe devolver el
	// promedio en vez de dividir por cero.
	quarters := []sqlite.EarningsQuarter{
		{SurprisePct: 0.03}, {SurprisePct: 0.03}, {SurprisePct: 0.03},
	}
	got, err := Compute(quarters)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if math.Abs(got-0.03) > 1e-9 {
		t.Errorf("Compute() = %v, want 0.03 (el promedio, sin dividir)", got)
	}
}

func TestCompute_EmptyQuartersErrors(t *testing.T) {
	if _, err := Compute(nil); err == nil {
		t.Error("esperaba error sin historial de earnings")
	}
}

func TestCompute_NegativeSurprisesGiveNegativeScore(t *testing.T) {
	quarters := []sqlite.EarningsQuarter{
		{SurprisePct: -0.01}, {SurprisePct: -0.03}, {SurprisePct: -0.02}, {SurprisePct: -0.05},
	}
	got, err := Compute(quarters)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if got >= 0 {
		t.Errorf("Compute() = %v, want < 0 (todas las sorpresas fueron negativas)", got)
	}
}
