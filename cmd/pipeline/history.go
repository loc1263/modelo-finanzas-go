package main

import (
	"fmt"
	"time"

	"modelofinanzas/internal/pipeline"
	"modelofinanzas/internal/storage/sqlite"
)

// runHistory implementa el flag -history (MEJ-013): en vez de correr el
// pipeline, imprime en stdout los resultados ya guardados en model_results
// para el dia dado, agrupados por universo y modelo.
func runHistory(db *sqlite.DB, dateStr string) error {
	runDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return fmt.Errorf("-history: fecha invalida %q, se espera formato YYYY-MM-DD: %w", dateStr, err)
	}

	anyResults := false
	for _, um := range pipeline.KnownUniverseModels() {
		results, err := db.GetResults(runDate, um.Universe, um.Model)
		if err != nil {
			return fmt.Errorf("consultando %s/%s: %w", um.Universe, um.Model, err)
		}
		if len(results) == 0 {
			continue
		}
		anyResults = true

		fmt.Printf("\n=== %s / %s (%s) ===\n", um.Universe, um.Model, dateStr)
		fmt.Printf("%-6s %-10s %12s\n", "Rank", "Symbol", "Score")
		for _, r := range results {
			fmt.Printf("%-6d %-10s %12.4f\n", r.Rank, r.Symbol, r.Score)
		}
	}

	if !anyResults {
		fmt.Printf("No hay resultados guardados para %s.\n", dateStr)
	}

	return nil
}
