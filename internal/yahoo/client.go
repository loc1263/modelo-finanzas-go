// Package yahoo es un adaptador fino sobre github.com/loc1263/gohoo, el
// cliente de los endpoints no oficiales de Yahoo Finance (chart y
// quoteSummary). gohoo se encarga de la sesion cookie+crumb, los reintentos
// con backoff y los 429; este paquete conserva la pausa por request y la
// forma de los datos que consume el pipeline (Bar, EarningsQuarter).
package yahoo

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/loc1263/gohoo"
)

// Client descarga precios historicos y earnings de Yahoo Finance. Es seguro
// para uso concurrente: gohoo.Client lo es, y Pause se aplica por llamada.
type Client struct {
	Pause  time.Duration
	Logger *log.Logger

	gc *gohoo.Client
}

// NewClient crea un cliente con los parametros de rate-limiting dados.
// maxRetries son reintentos adicionales al primer intento. El bootstrap de
// cookie+crumb es perezoso: no hay red hasta el primer request que lo necesite.
func NewClient(timeout, pause, retryBackoff time.Duration, maxRetries int, logger *log.Logger) *Client {
	policy := gohoo.RetryPolicy{
		MaxAttempts:     maxRetries + 1,
		BaseDelay:       retryBackoff,
		RetryRateLimit:  true,
		HonorRetryAfter: true,
		OnRetry: func(attempt int, err error, delay time.Duration) {
			if logger != nil {
				logger.Printf("[yahoo] reintento tras intento %d (%v), esperando %s", attempt, err, delay)
			}
		},
	}
	// Con opciones validas el constructor perezoso no hace red ni falla.
	gc, err := gohoo.NewClientWithOptions(
		gohoo.WithLazyBootstrap(),
		gohoo.WithHTTPClient(&http.Client{Timeout: timeout}),
		gohoo.WithRetryPolicy(policy),
	)
	if err != nil {
		panic(fmt.Sprintf("yahoo: creando cliente gohoo: %v", err))
	}
	return &Client{Pause: pause, Logger: logger, gc: gc}
}

func (c *Client) pause() {
	if c.Pause > 0 {
		time.Sleep(c.Pause)
	}
}

// GetDailyHistory descarga las barras diarias de symbol entre start y end,
// con una pausa fija antes del request (cada llamada espera la suya completa,
// ver MEJ-017 en CLAUDE.md). Los dias sin cierre se omiten.
func (c *Client) GetDailyHistory(symbol string, start, end time.Time) ([]Bar, error) {
	c.pause()

	res, err := c.gc.ChartRange(context.Background(), symbol, gohoo.ChartParams{
		Start:    start,
		End:      end,
		Interval: "1d",
	})
	if err != nil {
		return nil, fmt.Errorf("yahoo: %s: %w", symbol, err)
	}

	bars := make([]Bar, 0, len(res.Bars))
	for _, b := range res.Bars {
		if b.Close == 0 {
			continue
		}
		adj := b.AdjClose
		if adj == 0 {
			adj = b.Close
		}
		bars = append(bars, Bar{
			Date:     b.Time.UTC(),
			Open:     b.Open,
			High:     b.High,
			Low:      b.Low,
			Close:    b.Close,
			AdjClose: adj,
			Volume:   int64(b.Volume),
		})
	}
	return bars, nil
}
