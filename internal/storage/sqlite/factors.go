package sqlite

import (
	"time"

	"modelofinanzas/internal/famafrench"
)

// UpsertFactors guarda los factores mensuales de un dataset de Fama-French.
func (db *DB) UpsertFactors(dataset string, monthly []famafrench.MonthlyFactors) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO factors (dataset, date, factor, value)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(dataset, date, factor) DO UPDATE SET value=excluded.value
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range monthly {
		dateStr := m.Date.Format(dateFormat)
		for factor, value := range m.Values {
			if _, err := stmt.Exec(dataset, dateStr, factor, value); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// FactorRow es una fila de factores de un mes para un dataset dado.
type FactorRow struct {
	Date   time.Time
	Values map[string]float64
}

// GetFactors devuelve los factores de dataset entre start y end, uno por mes,
// con todos los valores de ese mes agrupados en Values.
func (db *DB) GetFactors(dataset string, start, end time.Time) ([]FactorRow, error) {
	rows, err := db.Query(`
		SELECT date, factor, value FROM factors
		WHERE dataset = ? AND date BETWEEN ? AND ?
		ORDER BY date ASC
	`, dataset, start.Format(dateFormat), end.Format(dateFormat))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byDate := make(map[string]*FactorRow)
	var order []string
	for rows.Next() {
		var dateStr, factor string
		var value float64
		if err := rows.Scan(&dateStr, &factor, &value); err != nil {
			return nil, err
		}
		fr, ok := byDate[dateStr]
		if !ok {
			d, err := time.Parse(dateFormat, dateStr)
			if err != nil {
				return nil, err
			}
			fr = &FactorRow{Date: d, Values: map[string]float64{}}
			byDate[dateStr] = fr
			order = append(order, dateStr)
		}
		fr.Values[factor] = value
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]FactorRow, 0, len(order))
	for _, d := range order {
		out = append(out, *byDate[d])
	}
	return out, nil
}
