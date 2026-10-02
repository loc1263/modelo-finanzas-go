// Package yahoo implementa un cliente HTTP liviano para la API publica y
// gratuita de Yahoo Finance (el mismo endpoint que usa la libreria yfinance
// de Python por debajo), con pausas y reintentos para no ser bloqueado.
package yahoo

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"
)

const chartURLTemplate = "https://query1.finance.yahoo.com/v8/finance/chart/%s?period1=%d&period2=%d&interval=1d&events=div,splits"

// browserUserAgent se manda en todos los requests: Yahoo exige un
// User-Agent de navegador, sin esto algunas respuestas vienen vacias o con error.
const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// Client descarga datos de Yahoo Finance: precios historicos (endpoint
// publico, sin autenticacion) y earnings (endpoint que exige una sesion
// cookie+crumb, ver earnings.go).
type Client struct {
	httpClient   *http.Client
	Pause        time.Duration
	MaxRetries   int
	RetryBackoff time.Duration
	Logger       *log.Logger

	crumbMu sync.Mutex
	crumb   string
}

// NewClient crea un cliente con los parametros de rate-limiting dados. El
// http.Client lleva un cookie jar porque el endpoint de earnings
// (quoteSummary) necesita mantener cookies de sesion entre requests.
func NewClient(timeout, pause, retryBackoff time.Duration, maxRetries int, logger *log.Logger) *Client {
	jar, _ := cookiejar.New(nil) // nunca falla con opts nil, segun la doc de net/http/cookiejar
	return &Client{
		httpClient:   &http.Client{Timeout: timeout, Jar: jar},
		Pause:        pause,
		MaxRetries:   maxRetries,
		RetryBackoff: retryBackoff,
		Logger:       logger,
	}
}

// GetDailyHistory descarga las barras diarias de symbol entre start y end
// (inclusive), reintentando con backoff ante errores transitorios o 429, y
// respetando una pausa fija antes del request para no bloquear la API.
func (c *Client) GetDailyHistory(symbol string, start, end time.Time) ([]Bar, error) {
	if c.Pause > 0 {
		time.Sleep(c.Pause)
	}

	url := fmt.Sprintf(chartURLTemplate, symbol, start.Unix(), end.Unix())

	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.RetryBackoff * time.Duration(attempt)
			if c.Logger != nil {
				c.Logger.Printf("[yahoo] %s: reintento %d/%d tras error (%v), esperando %s", symbol, attempt, c.MaxRetries, lastErr, backoff)
			}
			time.Sleep(backoff)
		}

		bars, err := c.doRequest(url)
		if err == nil {
			return bars, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("yahoo: %s: agotados los reintentos: %w", symbol, lastErr)
}

func (c *Client) doRequest(url string) ([]Bar, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("HTTP 429 too many requests")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var parsed chartResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parseando JSON: %w", err)
	}

	if parsed.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo error: %s: %s", parsed.Chart.Error.Code, parsed.Chart.Error.Description)
	}
	if len(parsed.Chart.Result) == 0 {
		return nil, fmt.Errorf("respuesta sin resultados")
	}

	return toBars(parsed.Chart.Result[0]), nil
}

func toBars(result chartResult) []Bar {
	bars := make([]Bar, 0, len(result.Timestamp))
	if len(result.Indicators.Quote) == 0 {
		return bars
	}
	q := result.Indicators.Quote[0]

	var adj []*float64
	if len(result.Indicators.AdjClose) > 0 {
		adj = result.Indicators.AdjClose[0].AdjClose
	}

	for i, ts := range result.Timestamp {
		if !has(q.Close, i) || q.Close[i] == nil {
			// Dia sin dato de cierre (ej. feriado reportado erroneamente): se omite.
			continue
		}

		bar := Bar{
			Date:  time.Unix(ts, 0).UTC(),
			Close: *q.Close[i],
		}
		if has(q.Open, i) && q.Open[i] != nil {
			bar.Open = *q.Open[i]
		}
		if has(q.High, i) && q.High[i] != nil {
			bar.High = *q.High[i]
		}
		if has(q.Low, i) && q.Low[i] != nil {
			bar.Low = *q.Low[i]
		}
		if has(q.Volume, i) && q.Volume[i] != nil {
			bar.Volume = *q.Volume[i]
		}
		if has(adj, i) && adj[i] != nil {
			bar.AdjClose = *adj[i]
		} else {
			bar.AdjClose = bar.Close
		}

		bars = append(bars, bar)
	}

	return bars
}

func has[T any](s []T, i int) bool { return i < len(s) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
