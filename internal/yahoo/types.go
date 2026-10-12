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
