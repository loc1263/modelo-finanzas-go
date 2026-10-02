// Package sqlite maneja la persistencia del pipeline: precios historicos,
// factores de Fama-French y resultados de los modelos.
package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schema = `
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

CREATE TABLE IF NOT EXISTS fetch_status (
	symbol         TEXT PRIMARY KEY,
	last_date      TEXT,
	last_fetched_at TEXT,
	status         TEXT,
	message        TEXT
);

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
`

// DB envuelve la conexion a la base SQLite del pipeline.
type DB struct {
	*sql.DB
}

// Open abre (y crea si no existe) la base SQLite en path, y aplica el esquema.
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

	if _, err := sqlDB.Exec(schema); err != nil {
		return nil, fmt.Errorf("aplicando esquema: %w", err)
	}

	return &DB{sqlDB}, nil
}
