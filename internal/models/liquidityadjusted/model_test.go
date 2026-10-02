package liquidityadjusted

import (
	"math"
	"testing"
	"time"

	"modelofinanzas/internal/storage/sqlite"
)

func TestIlliquidity_MatchesHandComputedRatio(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	bars := []sqlite.PriceBar{
		{Date: start, Close: 100, AdjClose: 100, Volume: 1_000_000},
		{Date: start.AddDate(0, 0, 1), Close: 101, AdjClose: 101, Volume: 1_000_000},
		{Date: start.AddDate(0, 0, 2), Close: 99, AdjClose: 99, Volume: 2_000_000},
	}

	got, err := Illiquidity(bars)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	// Dia 2: |101/100-1| / (101*1_000_000)
	r1 := math.Abs(101.0/100.0-1) / (101 * 1_000_000)
	// Dia 3: |99/101-1| / (99*2_000_000)
	r2 := math.Abs(99.0/101.0-1) / (99 * 2_000_000)
	want := (r1 + r2) / 2

	if math.Abs(got-want) > 1e-15 {
		t.Errorf("Illiquidity() = %v, want %v", got, want)
	}
}

func TestIlliquidity_InsufficientData(t *testing.T) {
	if _, err := Illiquidity(nil); err == nil {
		t.Error("esperaba error con menos de 2 dias de precios")
	}
}

func TestIlliquidity_ZeroVolumeDaysAreSkipped(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	bars := []sqlite.PriceBar{
		{Date: start, Close: 100, AdjClose: 100, Volume: 1_000_000},
		{Date: start.AddDate(0, 0, 1), Close: 100, AdjClose: 100, Volume: 0}, // sin volumen: se ignora
	}
	if _, err := Illiquidity(bars); err == nil {
		t.Error("esperaba error: ningun dia con volumen valido")
	}
}

// TestCombineWithMomentum_NeverImprovesScoreForIlliquidStock es la prueba
// que habria atrapado MEJ-001: para cualquier momentum (positivo o
// negativo), una accion mas iliquida (liquidityScore mas bajo) nunca debe
// quedar con un score MEJOR que una accion identica pero mas liquida.
func TestCombineWithMomentum_NeverImprovesScoreForIlliquidStock(t *testing.T) {
	const penaltyScale = 0.15

	cases := []struct {
		name     string
		momentum float64
	}{
		{"momentum positivo", 0.20},
		{"momentum negativo", -0.20},
		{"momentum cero", 0.0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			liquid := CombineWithMomentum(c.momentum, 1.0, penaltyScale)   // la mas liquida del universo
			illiquid := CombineWithMomentum(c.momentum, 0.1, penaltyScale) // bastante iliquida

			if illiquid > liquid {
				t.Errorf("accion iliquida (score=%v) quedo MEJOR que la liquida (score=%v) con %s — exactamente el bug de MEJ-001",
					illiquid, liquid, c.name)
			}
		})
	}
}

func TestCombineWithMomentum_FullLiquidityIsANoOp(t *testing.T) {
	// liquidityScore=1 (la mas liquida del universo) no deberia aplicar
	// ninguna penalidad, sin importar penaltyScale.
	got := CombineWithMomentum(-0.33, 1.0, 0.5)
	if math.Abs(got-(-0.33)) > 1e-12 {
		t.Errorf("CombineWithMomentum() = %v, want -0.33 (sin penalidad)", got)
	}
}

func TestCombineWithMomentum_PenaltyMagnitude(t *testing.T) {
	got := CombineWithMomentum(0.10, 0.4, 0.2)
	want := 0.10 - (1-0.4)*0.2
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("CombineWithMomentum() = %v, want %v", got, want)
	}
}
