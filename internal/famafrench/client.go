// Package famafrench descarga y parsea los datasets mensuales de la
// Kenneth R. French Data Library (Dartmouth), usados para el modelo de
// momentum residual.
package famafrench

import (
	"archive/zip"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"modelofinanzas/internal/httpclient"
)

const zipURLTemplate = "https://mba.tuck.dartmouth.edu/pages/faculty/ken.french/ftp/%s_CSV.zip"

// MonthlyFactors son los valores de los factores para un mes dado.
// Values ya esta expresado en fraccion (no en porcentaje): dividido por 100.
type MonthlyFactors struct {
	Date   time.Time
	Values map[string]float64
}

// Client descarga datasets de la Kenneth French Data Library.
type Client struct {
	httpClient   *http.Client
	MaxRetries   int
	RetryBackoff time.Duration
	Logger       *log.Logger

	// urlTemplate es zipURLTemplate por defecto; los tests lo pisan para
	// apuntar a un httptest.Server en vez de Dartmouth.
	urlTemplate string
}

// NewClient crea un cliente con reintento-con-backoff, reusando el mismo
// patron que internal/yahoo.Client (MEJ-011): una falla de red momentanea
// al bajar un dataset no deberia abortar la corrida entera.
func NewClient(timeout, retryBackoff time.Duration, maxRetries int, logger *log.Logger) *Client {
	return &Client{
		httpClient:   &http.Client{Timeout: timeout},
		MaxRetries:   maxRetries,
		RetryBackoff: retryBackoff,
		Logger:       logger,
		urlTemplate:  zipURLTemplate,
	}
}

var monthlyDateRe = regexp.MustCompile(`^\d{6}$`)

// GetMonthly descarga el dataset (ej. "F-F_Research_Data_Factors"),
// reintentando con backoff ante errores transitorios, y devuelve unicamente
// la seccion mensual, con los valores ya divididos por 100.
func (c *Client) GetMonthly(dataset string) ([]MonthlyFactors, error) {
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.RetryBackoff * time.Duration(attempt)
			if c.Logger != nil {
				c.Logger.Printf("[famafrench] %s: reintento %d/%d tras error (%v), esperando %s", dataset, attempt, c.MaxRetries, lastErr, backoff)
			}
			time.Sleep(backoff)
		}

		csvBytes, err := c.downloadCSV(dataset)
		if err == nil {
			return parseMonthly(csvBytes)
		}
		lastErr = err
	}

	return nil, fmt.Errorf("famafrench: %s: agotados los reintentos: %w", dataset, lastErr)
}

func (c *Client) downloadCSV(dataset string) ([]byte, error) {
	url := fmt.Sprintf(c.urlTemplate, dataset)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", httpclient.BrowserUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d descargando %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("abriendo zip: %w", err)
	}

	csvFile, err := findCSVFile(zr)
	if err != nil {
		return nil, err
	}

	f, err := csvFile.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(f)
}

// findCSVFile busca el unico archivo .csv dentro del zip (en vez de asumir
// que es el primero por posicion, ver MEJ-016). Dartmouth hoy siempre
// empaqueta un solo archivo, pero si eso cambiara (ej. agregan un README) es
// mejor fallar con un mensaje claro que leer el archivo equivocado.
func findCSVFile(zr *zip.Reader) (*zip.File, error) {
	var matches []*zip.File
	for _, f := range zr.File {
		if strings.EqualFold(filepath.Ext(f.Name), ".csv") {
			matches = append(matches, f)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("el zip no contiene ningun archivo .csv")
	case 1:
		return matches[0], nil
	default:
		names := make([]string, len(matches))
		for i, f := range matches {
			names[i] = f.Name
		}
		return nil, fmt.Errorf("el zip contiene mas de un archivo .csv: %s", strings.Join(names, ", "))
	}
}

// parseMonthly busca la fila de encabezado (la que empieza con ",") y luego
// lee filas "YYYYMM,v1,v2,..." hasta que el formato de fecha deja de calzar
// (lo que marca el inicio de la seccion anual o el fin del archivo).
func parseMonthly(csvBytes []byte) ([]MonthlyFactors, error) {
	scanner := bufio.NewScanner(bytes.NewReader(csvBytes))

	var columns []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, ",") {
			columns = splitTrim(line)[1:]
			break
		}
	}
	if columns == nil {
		return nil, fmt.Errorf("no se encontro la fila de encabezado")
	}

	var out []MonthlyFactors
	for scanner.Scan() {
		line := scanner.Text()
		fields := splitTrim(line)
		if len(fields) == 0 || !monthlyDateRe.MatchString(fields[0]) {
			break
		}

		date, err := time.Parse("200601", fields[0])
		if err != nil {
			break
		}

		values := make(map[string]float64, len(columns))
		for i, colName := range columns {
			if i+1 >= len(fields) {
				continue
			}
			v, err := strconv.ParseFloat(fields[i+1], 64)
			if err != nil {
				continue
			}
			values[colName] = v / 100.0
		}

		out = append(out, MonthlyFactors{Date: date, Values: values})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func splitTrim(line string) []string {
	parts := strings.Split(line, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}
