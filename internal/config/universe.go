package config

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
)

// Ticker representa un instrumento del universo de inversion (ETF o accion).
type Ticker struct {
	Symbol string
	Name   string
}

// LoadTickers lee un CSV con columnas "ticker,name", ignorando lineas vacias
// y comentarios que empiezan con '#'.
func LoadTickers(path string) ([]Ticker, error) {
	records, err := readCSVSkippingComments(path)
	if err != nil {
		return nil, err
	}

	var tickers []Ticker
	first := true
	for _, rec := range records {
		if len(rec) == 0 || strings.TrimSpace(rec[0]) == "" {
			continue
		}
		if first {
			first = false
			if strings.EqualFold(strings.TrimSpace(rec[0]), "ticker") {
				continue
			}
		}
		symbol := strings.TrimSpace(rec[0])
		name := ""
		if len(rec) > 1 {
			name = strings.TrimSpace(rec[1])
		}
		tickers = append(tickers, Ticker{Symbol: symbol, Name: name})
	}

	return tickers, nil
}

// readCSVSkippingComments filtra lineas que empiezan con '#' antes de parsear el CSV,
// para poder documentar los archivos de configuracion con comentarios.
func readCSVSkippingComments(path string) ([][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("leyendo universo %s: %w", path, err)
	}

	var cleaned strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		cleaned.WriteString(line)
		cleaned.WriteString("\n")
	}

	reader := csv.NewReader(strings.NewReader(cleaned.String()))
	reader.FieldsPerRecord = -1
	return reader.ReadAll()
}
