// Package sqlite maneja la persistencia del pipeline: precios historicos,
// factores de Fama-French y resultados de los modelos.
package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// migration es un paso incremental de esquema, aplicado una sola vez. Para
// cambiar el esquema en el futuro: agregar una migration NUEVA al final de
// esta lista (nunca editar ni borrar una ya publicada) con el proximo
// numero de version. Open() aplica las que falten, en orden, cada una en
// su propia transaccion, y registra cuales ya corrieron en
// schema_migrations — asi una base existente nunca queda con un esquema
// desactualizado despues de un `git pull` (lo que paso una vez con
// model_results antes de que existiera este mecanismo).
type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		sql: `
CREATE TABLE IF NOT EXISTS prices (
	symbol    TEXT NOT NULL,
	date      TEXT NOT NULL,
	open      REAL,
	high      REAL,
	low       REAL,
	close     REAL NOT NULL,
	adj_close REAL NOT NULL,
	volume    INTEGER,
	PRIMARY KEY (symbol, date)
);
CREATE INDEX IF NOT EXISTS idx_prices_symbol ON prices(symbol);

CREATE TABLE IF NOT EXISTS factors (
	dataset TEXT NOT NULL,
	date    TEXT NOT NULL,
	factor  TEXT NOT NULL,
	value   REAL NOT NULL,
	PRIMARY KEY (dataset, date, factor)
);

-- Historial trimestral de EPS real vs. estimado (modelo 8, momentum de
-- earnings). quarter es la fecha de cierre del trimestre que informa Yahoo.
CREATE TABLE IF NOT EXISTS earnings (
	symbol        TEXT NOT NULL,
	quarter       TEXT NOT NULL,
	eps_actual    REAL,
	eps_estimate  REAL,
	surprise_pct  REAL,
	PRIMARY KEY (symbol, quarter)
);

-- run_date (no la hora exacta) es parte de la clave: si el pipeline se
-- corre varias veces el mismo dia, la corrida mas reciente sobreescribe
-- (UPSERT) los resultados de ese dia en vez de acumular filas duplicadas.
CREATE TABLE IF NOT EXISTS model_results (
	run_date   TEXT NOT NULL,
	universe   TEXT NOT NULL,
	model      TEXT NOT NULL,
	symbol     TEXT NOT NULL,
	as_of      TEXT NOT NULL,
	score      REAL,
	rank       INTEGER,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (run_date, universe, model, symbol)
);
`,
	},
}

// DB envuelve la conexion a la base SQLite del pipeline.
type DB struct {
	*sql.DB
}

// Open abre (y crea si no existe) la base SQLite en path, y aplica las
// migraciones de esquema que falten.
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creando directorio de datos: %w", err)
		}
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("abriendo sqlite: %w", err)
	}

	// SQLite solo soporta un writer a la vez; una sola conexion evita
	// errores de "database is locked" dentro del mismo proceso.
	sqlDB.SetMaxOpenConns(1)

	if err := applyMigrations(sqlDB); err != nil {
		return nil, fmt.Errorf("aplicando migraciones de esquema: %w", err)
	}

	return &DB{sqlDB}, nil
}

// applyMigrations corre, en orden, las migrations que todavia no esten
// registradas en schema_migrations.
func applyMigrations(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("creando schema_migrations: %w", err)
	}

	applied := map[int]bool{}
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("leyendo schema_migrations: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("migracion %d: %w", m.version, err)
		}

		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("aplicando migracion %d: %w", m.version, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
			m.version, time.Now().UTC().Format(time.RFC3339),
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("registrando migracion %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("confirmando migracion %d: %w", m.version, err)
		}
	}

	return nil
}
