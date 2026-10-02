package sqlite

import (
	"time"

	"modelofinanzas/internal/yahoo"
)

// EarningsQuarter es una fila de la tabla earnings para consumo de los
// modelos (igual forma que yahoo.EarningsQuarter, pero definida aca para
// que los paquetes de modelos no tengan que importar internal/yahoo).
type EarningsQuarter struct {
	Quarter     time.Time
	EPSActual   float64
	EPSEstimate float64
	SurprisePct float64
}

// UpsertEarnings guarda el historial de earnings de symbol.
func (db *DB) UpsertEarnings(symbol string, quarters []yahoo.EarningsQuarter) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO earnings (symbol, quarter, eps_actual, eps_estimate, surprise_pct)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(symbol, quarter) DO UPDATE SET
			eps_actual=excluded.eps_actual, eps_estimate=excluded.eps_estimate,
			surprise_pct=excluded.surprise_pct
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, q := range quarters {
		if _, err := stmt.Exec(symbol, q.Quarter.Format(dateFormat), q.EPSActual, q.EPSEstimate, q.SurprisePct); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetEarnings devuelve el historial de earnings de symbol, ordenado por
// trimestre ascendente (el mas reciente al final).
func (db *DB) GetEarnings(symbol string) ([]EarningsQuarter, error) {
	rows, err := db.Query(`
		SELECT quarter, eps_actual, eps_estimate, surprise_pct
		FROM earnings
		WHERE symbol = ?
		ORDER BY quarter ASC
	`, symbol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EarningsQuarter
	for rows.Next() {
		var quarterStr string
		var q EarningsQuarter
		if err := rows.Scan(&quarterStr, &q.EPSActual, &q.EPSEstimate, &q.SurprisePct); err != nil {
			return nil, err
		}
		q.Quarter, err = time.Parse(dateFormat, quarterStr)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}
