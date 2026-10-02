package sqlite

import "time"

// ResetDay borra los precios y los resultados de modelos asociados al dia
// `date` (precios cuyo campo date == ese dia, y resultados cuyo run_date ==
// ese dia). Se usa para forzar una re-descarga/recalculo totalmente limpio
// del dia en curso, por ejemplo si una corrida anterior quedo con datos
// parciales o incorrectos.
//
// No afecta precios de otros dias ni los factores de Fama-French (que son
// mensuales y se sobreescriben solos via upsert).
func (db *DB) ResetDay(date time.Time) error {
	dateStr := date.Format(dateFormat)

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM prices WHERE date = ?`, dateStr); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM model_results WHERE run_date = ?`, dateStr); err != nil {
		return err
	}

	return tx.Commit()
}
