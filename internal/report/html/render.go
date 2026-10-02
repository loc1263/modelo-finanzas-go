// Package html genera el reporte final en HTML a partir del resultado del pipeline.
package html

import (
	_ "embed"
	"html/template"
	"math"
	"os"
	"path/filepath"

	"modelofinanzas/internal/pipeline"
	"modelofinanzas/internal/storage/sqlite"
)

//go:embed templates/report.html.tmpl
var templateSource string

// TableTab es una tabla de resultados identificada por un ID unico de DOM,
// usado por el template para poder paginarla en el navegador (10 tickers
// por pagina) sin depender de ningun framework.
type TableTab struct {
	ID      string
	Results []sqlite.ModelResult
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
	n := len(results)
	if n > 5 {
		n = 5
	}
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
func Render(report *pipeline.Report, outputDir, filename string) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}

	tmpl, err := template.New("report").Funcs(funcMap).Parse(templateSource)
	if err != nil {
		return "", err
	}

	path := filepath.Join(outputDir, filename)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if err := tmpl.Execute(f, report); err != nil {
		return "", err
	}

	return path, nil
}
