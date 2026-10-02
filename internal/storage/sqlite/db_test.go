package sqlite

import (
	"path/filepath"
	"testing"
	"time"
)

// TestOpen_MigrationsAreIdempotent es la prueba central de MEJ-010: abrir
// la misma base dos veces (ej. dos corridas del pipeline) no debe fallar
// ni reaplicar una migracion ya registrada.
func TestOpen_MigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db1, err := Open(path)
	if err != nil {
		t.Fatalf("primer Open: %v", err)
	}

	var countAfterFirst int
	if err := db1.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&countAfterFirst); err != nil {
		t.Fatalf("contando schema_migrations: %v", err)
	}
	if countAfterFirst != len(migrations) {
		t.Errorf("schema_migrations tiene %d filas, want %d (una por migration)", countAfterFirst, len(migrations))
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("cerrando db1: %v", err)
	}

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("segundo Open: %v", err)
	}
	defer db2.Close()

	var countAfterSecond int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&countAfterSecond); err != nil {
		t.Fatalf("contando schema_migrations (2do Open): %v", err)
	}
	if countAfterSecond != countAfterFirst {
		t.Errorf("schema_migrations tiene %d filas tras el 2do Open, want %d (no deberia reaplicar nada)", countAfterSecond, countAfterFirst)
	}

	// Las tablas de negocio deben existir y ser usables despues de ambos Opens.
	now := time.Now()
	if _, err := db2.GetPrices("AAPL", now, now); err != nil {
		t.Errorf("GetPrices fallo despues de reabrir la base: %v", err)
	}
}
