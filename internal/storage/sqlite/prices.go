package sqlite

import (
	"fmt"
	"time"

	"modelofinanzas/internal/yahoo"
)

const dateFormat = "2006-01-02"

// UpsertBars inserta o reemplaza las barras de precios de symbol.
func (db *DB) UpsertBars(symbol string, bars []yahoo.Bar) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO prices (symbol, date, open, high, low, close, adj_close, volume)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(symbol, date) DO UPDATE SET
			open=excluded.open, high=excluded.high, low=excluded.low,
			close=excluded.close, adj_close=excluded.adj_close, volume=excluded.volume
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, b := range bars {
		if _, err := stmt.Exec(symbol, b.Date.Format(dateFormat), b.Open, b.High, b.Low, b.Close, b.AdjClose, b.Volume); err != nil {
			return fmt.Errorf("insertando barra %s %s: %w", symbol, b.Date.Format(dateFormat), err)
		}
	}

	return tx.Commit()
}

// PriceBar es una fila de la tabla prices para consumo de los modelos.
type PriceBar struct {
	Date     time.Time
	Open     float64
	High     float64
	Low      float64
	Close    float64
	AdjClose float64
	Volume   int64
}

// GetPrices devuelve las barras de symbol entre start y end, ordenadas por fecha ascendente.
func (db *DB) GetPrices(symbol string, start, end time.Time) ([]PriceBar, error) {
	rows, err := db.Query(`
		SELECT date, open, high, low, close, adj_close, volume
		FROM prices
		WHERE symbol = ? AND date BETWEEN ? AND ?
		ORDER BY date ASC
	`, symbol, start.Format(dateFormat), end.Format(dateFormat))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PriceBar
	for rows.Next() {
		var dateStr string
		var b PriceBar
		if err := rows.Scan(&dateStr, &b.Open, &b.High, &b.Low, &b.Close, &b.AdjClose, &b.Volume); err != nil {
			return nil, err
		}
		b.Date, err = time.Parse(dateFormat, dateStr)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// SetFetchStatus registra el resultado de intentar descargar un symbol, para
// poder revisar fallas y evitar re-descargar lo ya obtenido en una corrida posterior.
func (db *DB) SetFetchStatus(symbol, lastDate, status, message string) error {
	_, err := db.Exec(`
		INSERT INTO fetch_status (symbol, last_date, last_fetched_at, status, message)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(symbol) DO UPDATE SET
			last_date=excluded.last_date, last_fetched_at=excluded.last_fetched_at,
			status=excluded.status, message=excluded.message
	`, symbol, lastDate, time.Now().UTC().Format(time.RFC3339), status, message)
	return err
}
