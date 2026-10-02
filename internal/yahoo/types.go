package yahoo

import "time"

// Bar es una barra de precios diaria (OHLCV + cierre ajustado).
type Bar struct {
	Date     time.Time
	Open     float64
	High     float64
	Low      float64
	Close    float64
	AdjClose float64
	Volume   int64
}

// Los siguientes tipos mapean el JSON devuelto por
// https://query1.finance.yahoo.com/v8/finance/chart/{symbol}

type chartResponse struct {
	Chart struct {
		Result []chartResult `json:"result"`
		Error  *chartError   `json:"error"`
	} `json:"chart"`
}

type chartError struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type chartResult struct {
	Timestamp  []int64         `json:"timestamp"`
	Indicators chartIndicators `json:"indicators"`
}

type chartIndicators struct {
	Quote    []chartQuote    `json:"quote"`
	AdjClose []chartAdjClose `json:"adjclose"`
}

type chartQuote struct {
	Open   []*float64 `json:"open"`
	High   []*float64 `json:"high"`
	Low    []*float64 `json:"low"`
	Close  []*float64 `json:"close"`
	Volume []*int64   `json:"volume"`
}

type chartAdjClose struct {
	AdjClose []*float64 `json:"adjclose"`
}
