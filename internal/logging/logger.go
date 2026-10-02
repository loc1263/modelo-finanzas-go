// Package logging provee un logger simple que escribe a consola y a un
// archivo de log, para poder revisar cada paso del pipeline en caso de caidas.
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// New crea un *log.Logger que escribe simultaneamente a stdout y a un
// archivo con fecha (dir/<base>_YYYY-MM-DD<ext>, derivado de `file`), para
// que corridas de dias distintos queden en archivos separados en vez de
// un unico log que crece para siempre. Corridas repetidas el mismo dia
// quedan en el mismo archivo (se abre en modo append).
//
// Si retentionDays > 0, antes de devolver el logger borra los logs de este
// mismo patron con mas de retentionDays dias (nunca toca archivos de otro
// patron que compartan el directorio). retentionDays <= 0 desactiva el
// borrado.
func New(dir, file string, asOf time.Time, retentionDays int) (*log.Logger, *os.File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}

	datedFile := datedFilename(file, asOf)
	path := filepath.Join(dir, datedFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}

	writer := io.MultiWriter(os.Stdout, f)
	logger := log.New(writer, "", log.Ldate|log.Ltime)

	pruneOldLogs(logger, dir, file, asOf, retentionDays)

	return logger, f, nil
}

// datedFilename inserta la fecha antes de la extension: "pipeline.log" +
// 2026-10-02 -> "pipeline_2026-10-02.log".
func datedFilename(file string, asOf time.Time) string {
	ext := filepath.Ext(file)
	base := strings.TrimSuffix(file, ext)
	return fmt.Sprintf("%s_%s%s", base, asOf.Format("2006-01-02"), ext)
}

// pruneOldLogs borra, dentro de dir, los logs con el mismo patron de
// nombre que `file` (mismo prefijo y extension) cuya fecha sea anterior a
// asOf-retentionDays. Nunca borra un archivo cuyo nombre no calce
// exactamente con el patron "<base>_YYYY-MM-DD<ext>" (defensivo: evita
// tocar archivos de otro origen que compartan el directorio). Los errores
// se loguean como AVISO; nunca hacen fallar al pipeline.
func pruneOldLogs(logger *log.Logger, dir, file string, asOf time.Time, retentionDays int) {
	if retentionDays <= 0 {
		return
	}

	ext := filepath.Ext(file)
	base := strings.TrimSuffix(file, ext)
	prefix := base + "_"

	matches, err := filepath.Glob(filepath.Join(dir, prefix+"*"+ext))
	if err != nil {
		logger.Printf("AVISO: no se pudieron listar logs viejos: %v", err)
		return
	}

	cutoff := asOf.AddDate(0, 0, -retentionDays)
	for _, m := range matches {
		name := filepath.Base(m)
		dateStr := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ext)
		d, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue // no calza con el patron exacto, no se toca
		}
		if !d.Before(cutoff) {
			continue
		}
		if err := os.Remove(m); err != nil {
			logger.Printf("AVISO: no se pudo borrar el log viejo %s: %v", m, err)
			continue
		}
		logger.Printf("Borrado log viejo: %s", m)
	}
}
