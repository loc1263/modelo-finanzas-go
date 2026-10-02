// Package logging provee un logger simple que escribe a consola y a un
// archivo de log, para poder revisar cada paso del pipeline en caso de caidas.
package logging

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// New crea un *log.Logger que escribe simultaneamente a stdout y al archivo
// indicado (dir/file), creando el directorio si no existe.
func New(dir, file string) (*log.Logger, *os.File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}

	path := filepath.Join(dir, file)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}

	writer := io.MultiWriter(os.Stdout, f)
	logger := log.New(writer, "", log.Ldate|log.Ltime)
	return logger, f, nil
}
