package sqlite

import "time"

// ModelResult es el score/ranking de un symbol para un modelo y universo dados.
type ModelResult struct {
	Universe string
	Model    string
	Symbol   string
	AsOf     time.Time
	Score    float64
	Rank     int
}

// InsertResults guarda los resultados de una corrida del pipeline.
//
// runDate identifica el DIA de la corrida (no la hora exacta): si el
// pipeline se corre varias veces el mismo dia, cada corrida sobreescribe
// (UPSERT) los resultados de ese dia en vez de acumular filas nuevas, por
// lo que es seguro reejecutar el pipeline repetidas veces sin duplicar datos.
func (db *DB) InsertResults(runDate time.Time, results []ModelResult) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO model_results (run_date, universe, model, symbol, as_of, score, rank, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_date, universe, model, symbol) DO UPDATE SET
			score=excluded.score, rank=excluded.rank, as_of=excluded.as_of, updated_at=excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	runDateStr := runDate.Format(dateFormat)
	updatedAt := time.Now().UTC().Format(time.RFC3339)
	for _, r := range results {
		if _, err := stmt.Exec(runDateStr, r.Universe, r.Model, r.Symbol, r.AsOf.Format(dateFormat), r.Score, r.Rank, updatedAt); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetResults devuelve los resultados de la corrida del dia runDate para un universo y modelo.
func (db *DB) GetResults(runDate time.Time, universe, model string) ([]ModelResult, error) {
	rows, err := db.Query(`
		SELECT symbol, as_of, score, rank FROM model_results
		WHERE run_date = ? AND universe = ? AND model = ?
		ORDER BY rank ASC
	`, runDate.Format(dateFormat), universe, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ModelResult
	for rows.Next() {
		var r ModelResult
		var asOfStr string
		r.Universe, r.Model = universe, model
		if err := rows.Scan(&r.Symbol, &asOfStr, &r.Score, &r.Rank); err != nil {
			return nil, err
		}
		r.AsOf, err = time.Parse(dateFormat, asOfStr)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
