// Package html genera el reporte final en HTML a partir del resultado del pipeline.
package html

import (
	_ "embed"
	"fmt"
	"html/template"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"modelofinanzas/internal/pipeline"
	"modelofinanzas/internal/storage/sqlite"
)

// ReportFilePrefix y ReportTimestampLayout describen el patron de nombre
// de los reportes con historial ("reporte_<timestamp>.html"). Exportados
// para que cmd/pipeline/main.go arme el nombre del archivo con el mismo
// formato que PruneOldReports sabe reconocer y limpiar, sin duplicar el
// formato en los dos lugares.
const (
	ReportFilePrefix      = "reporte_"
	ReportTimestampLayout = "20060102_150405"
	LatestReportFilename  = "reporte_ultimo.html"
)

//go:embed templates/report.html.tmpl
var templateSource string

// TableTab es una tabla de resultados identificada por un ID unico de DOM,
// usado por el template para poder paginarla en el navegador (10 tickers
// por pagina) sin depender de ningun framework.
type TableTab struct {
	ID      string
	Results []sqlite.ModelResult
	// IsPercent indica que Score es en realidad un peso de cartera
	// (fraccion entre 0 y 1) y debe mostrarse como porcentaje con su propio
	// encabezado ("Peso"), en vez de un score crudo — ver portfolioTab.
	IsPercent bool
}

// ChartBar es una barra del mini-grafico de los primeros 5 tickers de un
// modelo. WidthPct ya viene calculado (0-100) en relacion al mayor |score|
// de ese top 5, para que el template solo tenga que pintar el ancho.
type ChartBar struct {
	Rank     int
	Symbol   string
	Name     string
	Score    float64
	WidthPct float64
	Positive bool
}

// ChartTab es el mini-grafico de un modelo, identificado por un ID unico de DOM.
type ChartTab struct {
	ID   string
	Bars []ChartBar
}

var funcMap = template.FuncMap{
	"mulf": func(a, b float64) float64 { return a * b },
	"tableTab": func(id string, results []sqlite.ModelResult) TableTab {
		return TableTab{ID: id, Results: results}
	},
	"portfolioTab": func(id string, results []sqlite.ModelResult) TableTab {
		return TableTab{ID: id, Results: results, IsPercent: true}
	},
	"chartTab": buildChartTab,
	"yahooQuoteURL": func(symbol string) string {
		return "https://finance.yahoo.com/quote/" + symbol
	},
}

// buildChartTab toma el top 5 de results y calcula el ancho relativo de
// cada barra (en % del mayor |score| del top 5), con una sola barra
// creciendo desde cero hacia la derecha (score positivo) o hacia la
// izquierda (score negativo) — ver internal/report/html/templates.
func buildChartTab(id string, results []sqlite.ModelResult, names map[string]string) ChartTab {
	n := min(len(results), 5)
	top := results[:n]

	maxAbs := 0.0
	for _, r := range top {
		if a := math.Abs(r.Score); a > maxAbs {
			maxAbs = a
		}
	}

	bars := make([]ChartBar, 0, n)
	for _, r := range top {
		widthPct := 0.0
		if maxAbs > 0 {
			widthPct = math.Abs(r.Score) / maxAbs * 100
		}
		name := names[r.Symbol]
		if name == "" {
			name = r.Symbol
		}
		bars = append(bars, ChartBar{
			Rank:     r.Rank,
			Symbol:   r.Symbol,
			Name:     name,
			Score:    r.Score,
			WidthPct: widthPct,
			Positive: r.Score >= 0,
		})
	}

	return ChartTab{ID: id, Bars: bars}
}

// Render escribe el reporte HTML de report en outputDir/filename.
//
// Escribe primero a un archivo temporal en el mismo directorio y recien lo
// mueve (os.Rename, atomico dentro del mismo volumen) al nombre final si
// tmpl.Execute no fallo. Esto importa sobre todo para "reporte_ultimo.html"
// (el puntero estable que documenta el README): sin esto, un error a mitad
// de la escritura del template dejaria ese archivo truncado/corrupto hasta
// la proxima corrida exitosa.
func Render(report *pipeline.Report, outputDir, filename string) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}

	tmpl, err := template.New("report").Funcs(funcMap).Parse(templateSource)
	if err != nil {
		return "", err
	}

	path := filepath.Join(outputDir, filename)

	tmp, err := os.CreateTemp(outputDir, ".tmp-"+filename+"-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	// Si el rename de abajo tiene exito, esto es un no-op (el archivo ya no
	// esta en tmpPath); si algo falla antes, limpia el temporal a medio escribir.
	defer os.Remove(tmpPath)

	if err := tmpl.Execute(tmp, report); err != nil {
		tmp.Close()
		return "", fmt.Errorf("renderizando template: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("moviendo reporte a destino final: %w", err)
	}

	return path, nil
}

// PruneOldReports borra, dentro de outputDir, los "reporte_<timestamp>.html"
// con mas de retentionDays dias (nunca borra reporte_ultimo.html, el
// puntero estable a la corrida mas reciente). Un nombre que no calce
// exactamente con el patron "reporte_YYYYMMDD_HHMMSS.html" se ignora
// (defensivo: evita tocar archivos de otro origen que compartan la
// carpeta). retentionDays <= 0 desactiva el borrado.
//
// No hace fallar al pipeline: los errores de borrado quedan logueados
// como AVISO y se sigue con el resto.
func PruneOldReports(outputDir string, now time.Time, retentionDays int, logger *log.Logger) {
	if retentionDays <= 0 {
		return
	}

	matches, err := filepath.Glob(filepath.Join(outputDir, ReportFilePrefix+"*.html"))
	if err != nil {
		if logger != nil {
			logger.Printf("AVISO: no se pudieron listar reportes viejos: %v", err)
		}
		return
	}

	cutoff := now.AddDate(0, 0, -retentionDays)
	for _, m := range matches {
		name := filepath.Base(m)
		if name == LatestReportFilename {
			continue
		}

		ts := strings.TrimSuffix(strings.TrimPrefix(name, ReportFilePrefix), ".html")
		t, err := time.Parse(ReportTimestampLayout, ts)
		if err != nil {
			continue // no calza con el patron exacto, no se toca
		}
		if !t.Before(cutoff) {
			continue
		}

		if err := os.Remove(m); err != nil {
			if logger != nil {
				logger.Printf("AVISO: no se pudo borrar el reporte viejo %s: %v", m, err)
			}
			continue
		}
		if logger != nil {
			logger.Printf("Borrado reporte viejo: %s", m)
		}
	}
}
