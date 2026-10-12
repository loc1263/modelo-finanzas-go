package yahoo

import (
	"context"
	"fmt"
	"time"

	"github.com/loc1263/gohoo"
)

// EarningsQuarter es un trimestre de historial de earnings: EPS real
// reportado vs. el estimado por los analistas, y la sorpresa porcentual.
type EarningsQuarter struct {
	Quarter     time.Time
	EPSActual   float64
	EPSEstimate float64
	SurprisePct float64
}

// GetEarningsHistory descarga los ultimos trimestres de EPS real vs.
// estimado de symbol (modulo earningsHistory de quoteSummary). gohoo maneja
// el crumb y su renovacion.
func (c *Client) GetEarningsHistory(symbol string) ([]EarningsQuarter, error) {
	c.pause()

	data, err := c.gc.QuoteSummaryContext(context.Background(), symbol, []string{"earningsHistory"})
	if err != nil {
		return nil, fmt.Errorf("yahoo: earnings %s: %w", symbol, err)
	}

	history, _ := gohoo.GetPath(data, "earningsHistory", "history").([]any)
	if len(history) == 0 {
		return nil, fmt.Errorf("yahoo: earnings %s: sin datos de earnings (posiblemente sin cobertura de analistas)", symbol)
	}

	quarters := make([]EarningsQuarter, 0, len(history))
	for _, item := range history {
		h, ok := item.(map[string]any)
		if !ok {
			continue
		}
		q, _ := gohoo.AsInt64(gohoo.Field(h, "quarter"))
		actual, _ := gohoo.AsFloat(gohoo.Field(h, "epsActual"))
		estimate, _ := gohoo.AsFloat(gohoo.Field(h, "epsEstimate"))
		surprise, _ := gohoo.AsFloat(gohoo.Field(h, "surprisePercent"))
		quarters = append(quarters, EarningsQuarter{
			Quarter:     time.Unix(q, 0).UTC(),
			EPSActual:   actual,
			EPSEstimate: estimate,
			SurprisePct: surprise,
		})
	}
	return quarters, nil
}
