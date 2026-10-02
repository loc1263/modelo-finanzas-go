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
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
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
	httpClient *http.Client
}

func NewClient(timeout time.Duration) *Client {
	return &Client{httpClient: &http.Client{Timeout: timeout}}
}

var monthlyDateRe = regexp.MustCompile(`^\d{6}$`)

// GetMonthly descarga el dataset (ej. "F-F_Research_Data_Factors") y
// devuelve unicamente la seccion mensual, con los valores ya divididos por 100.
func (c *Client) GetMonthly(dataset string) ([]MonthlyFactors, error) {
	csvBytes, err := c.downloadCSV(dataset)
	if err != nil {
		return nil, fmt.Errorf("famafrench: %s: %w", dataset, err)
	}
	return parseMonthly(csvBytes)
}

func (c *Client) downloadCSV(dataset string) ([]byte, error) {
	url := fmt.Sprintf(zipURLTemplate, dataset)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")

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
	if len(zr.File) == 0 {
		return nil, fmt.Errorf("zip vacio")
	}

	f, err := zr.File[0].Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(f)
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
