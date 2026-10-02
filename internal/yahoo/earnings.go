package yahoo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// A diferencia de v8/finance/chart (precios), el endpoint de earnings
// (v10/finance/quoteSummary) exige una sesion: primero una cookie de
// fc.yahoo.com, despues un "crumb" (token) pedido con esa cookie. Es el
// mismo mecanismo que usa yfinance (Python) por debajo. Verificado a mano
// con curl antes de implementarlo (ver CLAUDE.md): Yahoo no lo documenta
// oficialmente, asi que podria cambiar sin aviso, igual que paso con
// piquette/finance-go.
const (
	crumbCookieURL          = "https://fc.yahoo.com"
	crumbURL                = "https://query1.finance.yahoo.com/v1/test/getcrumb"
	quoteSummaryURLTemplate = "https://query2.finance.yahoo.com/v10/finance/quoteSummary/%s?modules=earningsHistory&crumb=%s"
)

// EarningsQuarter es un trimestre de historial de earnings: EPS real
// reportado vs. el estimado por los analistas, y la sorpresa porcentual.
type EarningsQuarter struct {
	Quarter     time.Time
	EPSActual   float64
	EPSEstimate float64
	SurprisePct float64
}

type quoteSummaryResponse struct {
	QuoteSummary struct {
		Result []struct {
			EarningsHistory struct {
				History []struct {
					EPSActual       rawFloat `json:"epsActual"`
					EPSEstimate     rawFloat `json:"epsEstimate"`
					SurprisePercent rawFloat `json:"surprisePercent"`
					Quarter         rawInt   `json:"quarter"`
				} `json:"history"`
			} `json:"earningsHistory"`
		} `json:"result"`
		Error *chartError `json:"error"`
	} `json:"quoteSummary"`
}

type rawFloat struct {
	Raw float64 `json:"raw"`
}

type rawInt struct {
	Raw int64 `json:"raw"`
}

// GetEarningsHistory descarga los ultimos trimestres de EPS real vs.
// estimado de symbol, reintentando con backoff. Si el crumb cacheado quedo
// invalido (Yahoo lo rota), lo vuelve a pedir una vez antes de reintentar.
func (c *Client) GetEarningsHistory(symbol string) ([]EarningsQuarter, error) {
	if c.Pause > 0 {
		time.Sleep(c.Pause)
	}

	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.RetryBackoff * time.Duration(attempt)
			if c.Logger != nil {
				c.Logger.Printf("[yahoo] earnings %s: reintento %d/%d tras error (%v), esperando %s", symbol, attempt, c.MaxRetries, lastErr, backoff)
			}
			time.Sleep(backoff)
		}

		crumb, err := c.ensureCrumb(attempt > 0)
		if err != nil {
			lastErr = fmt.Errorf("obteniendo crumb: %w", err)
			continue
		}

		quarters, err := c.doEarningsRequest(symbol, crumb)
		if err == nil {
			return quarters, nil
		}
		lastErr = err
		if isCrumbError(err) {
			c.invalidateCrumb()
		}
	}

	return nil, fmt.Errorf("yahoo: earnings %s: agotados los reintentos: %w", symbol, lastErr)
}

// ensureCrumb devuelve el crumb cacheado, o pide uno nuevo si no hay o si
// forceRefresh es true (el anterior quedo invalido).
func (c *Client) ensureCrumb(forceRefresh bool) (string, error) {
	c.crumbMu.Lock()
	defer c.crumbMu.Unlock()

	if c.crumb != "" && !forceRefresh {
		return c.crumb, nil
	}

	// Paso 1: pisar la cookie de sesion. Esta URL responde 404, pero igual
	// trae el Set-Cookie que necesita el paso 2 (confirmado con curl).
	req, err := http.NewRequest(http.MethodGet, crumbCookieURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUserAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("pidiendo cookie de sesion: %w", err)
	}
	resp.Body.Close()

	// Paso 2: pedir el crumb usando esa cookie.
	req2, err := http.NewRequest(http.MethodGet, crumbURL, nil)
	if err != nil {
		return "", err
	}
	req2.Header.Set("User-Agent", browserUserAgent)
	resp2, err := c.httpClient.Do(req2)
	if err != nil {
		return "", fmt.Errorf("pidiendo crumb: %w", err)
	}
	defer resp2.Body.Close()

	body, err := io.ReadAll(resp2.Body)
	if err != nil {
		return "", err
	}

	crumb := strings.TrimSpace(string(body))
	if crumb == "" || strings.HasPrefix(crumb, "{") {
		return "", fmt.Errorf("crumb invalido: %s", truncate(crumb, 200))
	}

	c.crumb = crumb
	return crumb, nil
}

func (c *Client) invalidateCrumb() {
	c.crumbMu.Lock()
	c.crumb = ""
	c.crumbMu.Unlock()
}

func (c *Client) doEarningsRequest(symbol, crumb string) ([]EarningsQuarter, error) {
	url := fmt.Sprintf(quoteSummaryURLTemplate, symbol, crumb)
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

	var parsed quoteSummaryResponse
	if jsonErr := json.Unmarshal(body, &parsed); jsonErr != nil {
		return nil, fmt.Errorf("parseando JSON: %w", jsonErr)
	}

	if parsed.QuoteSummary.Error != nil {
		return nil, fmt.Errorf("yahoo error: %s: %s", parsed.QuoteSummary.Error.Code, parsed.QuoteSummary.Error.Description)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	if len(parsed.QuoteSummary.Result) == 0 {
		return nil, fmt.Errorf("sin datos de earnings (posiblemente sin cobertura de analistas)")
	}

	history := parsed.QuoteSummary.Result[0].EarningsHistory.History
	quarters := make([]EarningsQuarter, 0, len(history))
	for _, h := range history {
		quarters = append(quarters, EarningsQuarter{
			Quarter:     time.Unix(h.Quarter.Raw, 0).UTC(),
			EPSActual:   h.EPSActual.Raw,
			EPSEstimate: h.EPSEstimate.Raw,
			SurprisePct: h.SurprisePercent.Raw,
		})
	}
	return quarters, nil
}

func isCrumbError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Invalid Crumb")
}
