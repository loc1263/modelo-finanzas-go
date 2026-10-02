package common

import (
	"math"
	"testing"
	"time"

	"modelofinanzas/internal/storage/sqlite"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func bar(y int, m time.Month, d int, adjClose float64) sqlite.PriceBar {
	return sqlite.PriceBar{Date: date(y, m, d), Close: adjClose, AdjClose: adjClose}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestMonthKey(t *testing.T) {
	got := MonthKey(date(2026, time.March, 15))
	want := "2026-03"
	if got != want {
		t.Errorf("MonthKey() = %q, want %q", got, want)
	}
}

func TestBarsBetween(t *testing.T) {
	bars := []sqlite.PriceBar{
		bar(2026, time.January, 1, 10),
		bar(2026, time.January, 15, 11),
		bar(2026, time.February, 1, 12),
		bar(2026, time.February, 15, 13),
		bar(2026, time.March, 1, 14),
	}

	got := BarsBetween(bars, date(2026, time.January, 15), date(2026, time.February, 15))
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0].AdjClose != 11 || got[len(got)-1].AdjClose != 13 {
		t.Errorf("rango incorrecto: %v", got)
	}
}

func TestMonthEndCloses_ExcludesCurrentMonth(t *testing.T) {
	bars := []sqlite.PriceBar{
		bar(2026, time.January, 5, 100),
		bar(2026, time.January, 30, 110), // ultimo de enero
		bar(2026, time.February, 2, 111),
		bar(2026, time.February, 27, 120), // ultimo de febrero
		bar(2026, time.March, 1, 121),     // mes en curso (asOf = marzo)
		bar(2026, time.March, 2, 122),
	}
	asOf := date(2026, time.March, 10)

	got := MonthEndCloses(bars, asOf)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (enero y febrero, marzo excluido)", len(got))
	}
	if got[0].AdjClose != 110 {
		t.Errorf("cierre de enero = %v, want 110", got[0].AdjClose)
	}
	if got[1].AdjClose != 120 {
		t.Errorf("cierre de febrero = %v, want 120", got[1].AdjClose)
	}
}

func TestExcludeCurrentMonth(t *testing.T) {
	bars := []sqlite.PriceBar{
		bar(2026, time.February, 27, 1),
		bar(2026, time.March, 1, 2),
		bar(2026, time.March, 15, 3),
	}
	got := ExcludeCurrentMonth(bars, date(2026, time.March, 20))
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
}

func TestLastNBars(t *testing.T) {
	bars := []sqlite.PriceBar{bar(2026, 1, 1, 1), bar(2026, 1, 2, 2), bar(2026, 1, 3, 3)}

	if got := LastNBars(bars, 2); len(got) != 2 || got[0].AdjClose != 2 {
		t.Errorf("LastNBars(2) = %v", got)
	}
	if got := LastNBars(bars, 10); len(got) != 3 {
		t.Errorf("LastNBars(10) deberia devolver todas las barras, devolvio %d", len(got))
	}
}

func TestMonthlyReturns(t *testing.T) {
	closes := []MonthClose{
		{Date: date(2026, 1, 31), AdjClose: 100},
		{Date: date(2026, 2, 28), AdjClose: 110},
		{Date: date(2026, 3, 31), AdjClose: 99},
	}
	got := MonthlyReturns(closes)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if !almostEqual(got[0], 0.10) {
		t.Errorf("got[0] = %v, want 0.10", got[0])
	}
	if !almostEqual(got[1], -0.10) {
		t.Errorf("got[1] = %v, want -0.10", got[1])
	}
}

func TestTrailingReturn(t *testing.T) {
	closes := make([]MonthClose, 13)
	for i := range closes {
		closes[i] = MonthClose{Date: date(2025, time.Month(i+1), 1), AdjClose: 100}
	}
	closes[0].AdjClose = 100  // hace 12 meses
	closes[12].AdjClose = 150 // mes mas reciente

	got, err := TrailingReturn(closes, 12)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !almostEqual(got, 0.5) {
		t.Errorf("TrailingReturn = %v, want 0.5", got)
	}

	if _, err := TrailingReturn(closes, 20); err == nil {
		t.Error("esperaba error por historia insuficiente")
	}

	zeroBase := []MonthClose{{AdjClose: 0}, {AdjClose: 100}}
	if _, err := TrailingReturn(zeroBase, 1); err == nil {
		t.Error("esperaba error por precio base cero")
	}
}

func TestDailyReturns(t *testing.T) {
	bars := []sqlite.PriceBar{bar(2026, 1, 1, 100), bar(2026, 1, 2, 101), bar(2026, 1, 3, 99.99)}
	got := DailyReturns(bars)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if !almostEqual(got[0], 0.01) {
		t.Errorf("got[0] = %v, want 0.01", got[0])
	}
}

func TestMaxAdjCloseAndLastAdjClose(t *testing.T) {
	bars := []sqlite.PriceBar{bar(2026, 1, 1, 50), bar(2026, 1, 2, 70), bar(2026, 1, 3, 60)}
	if got := MaxAdjClose(bars); got != 70 {
		t.Errorf("MaxAdjClose = %v, want 70", got)
	}
	if got := LastAdjClose(bars); got != 60 {
		t.Errorf("LastAdjClose = %v, want 60", got)
	}
	if got := LastAdjClose(nil); got != 0 {
		t.Errorf("LastAdjClose(nil) = %v, want 0", got)
	}
}

func TestMeanAndStdDev(t *testing.T) {
	xs := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	if got := Mean(xs); !almostEqual(got, 5) {
		t.Errorf("Mean = %v, want 5", got)
	}
	// Desvio estandar muestral (n-1) conocido para este dataset clasico: 2.138...
	if got := StdDev(xs); math.Abs(got-2.13809) > 1e-4 {
		t.Errorf("StdDev = %v, want ~2.13809", got)
	}
	if got := StdDev([]float64{42}); got != 0 {
		t.Errorf("StdDev de un solo elemento = %v, want 0", got)
	}
}

func TestAnnualizedVol(t *testing.T) {
	xs := []float64{0.01, -0.01, 0.01, -0.01, 0.01}
	sd := StdDev(xs)
	got := AnnualizedVol(xs, 252)
	want := sd * math.Sqrt(252)
	if !almostEqual(got, want) {
		t.Errorf("AnnualizedVol = %v, want %v", got, want)
	}
}

func TestSum(t *testing.T) {
	if got := Sum([]float64{1, 2, 3.5}); !almostEqual(got, 6.5) {
		t.Errorf("Sum = %v, want 6.5", got)
	}
	if got := Sum(nil); got != 0 {
		t.Errorf("Sum(nil) = %v, want 0", got)
	}
}
