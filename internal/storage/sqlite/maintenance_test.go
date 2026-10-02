package sqlite

import (
	"testing"
	"time"

	"modelofinanzas/internal/yahoo"
)

func TestResetDay_DeletesOnlyTargetDay(t *testing.T) {
	db := openTestDB(t)

	day1 := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC)

	bars := []yahoo.Bar{
		{Date: day1, Close: 10, AdjClose: 10},
		{Date: day2, Close: 11, AdjClose: 11},
	}
	if err := db.UpsertBars("AAPL", bars); err != nil {
		t.Fatalf("UpsertBars: %v", err)
	}
	if err := db.InsertResults(day1, sampleResults(day1)); err != nil {
		t.Fatalf("InsertResults day1: %v", err)
	}
	if err := db.InsertResults(day2, sampleResults(day2)); err != nil {
		t.Fatalf("InsertResults day2: %v", err)
	}

	if err := db.ResetDay(day1); err != nil {
		t.Fatalf("ResetDay: %v", err)
	}

	prices, err := db.GetPrices("AAPL", day1, day2)
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	if len(prices) != 1 || prices[0].Close != 11 {
		t.Errorf("prices = %+v, deberia quedar solo el precio de day2 (11)", prices)
	}

	resultsDay1, err := db.GetResults(day1, "megacap", "momentum_12_1")
	if err != nil {
		t.Fatalf("GetResults day1: %v", err)
	}
	if len(resultsDay1) != 0 {
		t.Errorf("resultsDay1 = %+v, deberian haberse borrado", resultsDay1)
	}

	resultsDay2, err := db.GetResults(day2, "megacap", "momentum_12_1")
	if err != nil {
		t.Fatalf("GetResults day2: %v", err)
	}
	if len(resultsDay2) != 2 {
		t.Errorf("resultsDay2 = %+v, no deberian haberse tocado", resultsDay2)
	}
}
