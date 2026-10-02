// Package pipeline orquesta todo el proceso: descarga de datos, calculo de
// los modelos y preparacion de los resultados para el reporte.
package pipeline

import (
	"log"
	"sort"
	"time"

	"modelofinanzas/internal/config"
	"modelofinanzas/internal/famafrench"
	"modelofinanzas/internal/models/common"
	"modelofinanzas/internal/models/max52w"
	"modelofinanzas/internal/models/momentum12_1"
	"modelofinanzas/internal/models/residualmomentum"
	"modelofinanzas/internal/models/voladjusted"
	"modelofinanzas/internal/models/volscaling"
	"modelofinanzas/internal/storage/sqlite"
	"modelofinanzas/internal/yahoo"
)

const (
	UniverseVanguardETF = "vanguard_etf"
	UniverseMegacap     = "megacap"

	ModelMomentum12_1     = "momentum_12_1"
	ModelMax52w           = "max_52w"
	ModelVolAdjusted      = "vol_adjusted"
	ModelResidualMomentum = "residual_momentum"
	ModelVolScaling       = "vol_scaling"

	factorsDataset = "F-F_Research_Data_Factors"
)

// Options controla el alcance de una corrida (util para pruebas rapidas).
type Options struct {
	SkipFetch  bool // si es true, no se descarga nada nuevo de Yahoo/French; usa lo que ya hay en la DB
	Limit      int  // si es > 0, limita la cantidad de tickers por universo (para pruebas)
	ResetToday bool // si es true, borra precios y resultados del dia de AsOf antes de correr, para forzar una recorrida 100% limpia
}

// Pipeline agrupa las dependencias necesarias para correr el proceso completo.
type Pipeline struct {
	Cfg    *config.Config
	DB     *sqlite.DB
	Logger *log.Logger
	Yahoo  *yahoo.Client
	French *famafrench.Client
	AsOf   time.Time // momento de la corrida; los modelos nunca miran datos del mes de AsOf
}

// Report es el resultado consolidado de una corrida, listo para renderizar.
type Report struct {
	GeneratedAt    time.Time
	AsOf           time.Time
	ETFResults     map[string][]sqlite.ModelResult // keyed by model
	MegacapResults map[string][]sqlite.ModelResult
	VolScaling     volscaling.Result
	FailedFetches  []FetchFailure
	TickerNames    map[string]string // symbol -> nombre de la empresa/fondo, para mostrar en el reporte
}

type FetchFailure struct {
	Symbol string
	Error  string
}

// Run ejecuta el pipeline completo y devuelve el reporte consolidado.
func (p *Pipeline) Run(opts Options) (*Report, error) {
	etfTickers, err := config.LoadTickers(p.Cfg.Universe.VanguardETFsCSV)
	if err != nil {
		return nil, err
	}
	megacapTickers, err := config.LoadTickers(p.Cfg.Universe.MegacapStocksCSV)
	if err != nil {
		return nil, err
	}
	if opts.Limit > 0 {
		etfTickers = limitTickers(etfTickers, opts.Limit)
		megacapTickers = limitTickers(megacapTickers, opts.Limit)
	}

	if opts.ResetToday {
		p.Logger.Printf("=== ResetToday activo: borrando precios y resultados de %s ===", p.AsOf.Format("2006-01-02"))
		if err := p.DB.ResetDay(p.AsOf); err != nil {
			return nil, err
		}
	}

	historyStart := p.AsOf.AddDate(-p.Cfg.Yahoo.HistoryYears, 0, 0)

	var failures []FetchFailure
	if !opts.SkipFetch {
		p.Logger.Printf("=== Descargando precios: %d ETFs Vanguard ===", len(etfTickers))
		failures = append(failures, p.fetchPrices(etfTickers, historyStart)...)

		p.Logger.Printf("=== Descargando precios: %d acciones mega-cap ===", len(megacapTickers))
		failures = append(failures, p.fetchPrices(megacapTickers, historyStart)...)

		p.Logger.Printf("=== Descargando factores de Fama-French ===")
		if err := p.fetchFactors(); err != nil {
			p.Logger.Printf("ERROR descargando factores Fama-French: %v", err)
		}
	} else {
		p.Logger.Printf("=== SkipFetch activo: usando datos ya existentes en la base ===")
	}

	report := &Report{
		GeneratedAt:    time.Now().UTC(),
		AsOf:           p.AsOf,
		ETFResults:     map[string][]sqlite.ModelResult{},
		MegacapResults: map[string][]sqlite.ModelResult{},
		FailedFetches:  failures,
		TickerNames:    tickerNames(etfTickers, megacapTickers),
	}

	// --- ETFs: modelos 1 y 2 ---
	p.Logger.Printf("=== Calculando modelos 1-2 para ETFs ===")
	etfBars := p.loadAllPrices(etfTickers, historyStart)

	report.ETFResults[ModelMomentum12_1] = p.rankBySymbol(etfBars, UniverseVanguardETF, ModelMomentum12_1, func(symbol string, bars []sqlite.PriceBar) (float64, error) {
		return momentum12_1.Compute(bars, p.AsOf, p.Cfg.Models.LookbackMonthsLong)
	})
	report.ETFResults[ModelMax52w] = p.rankBySymbol(etfBars, UniverseVanguardETF, ModelMax52w, func(symbol string, bars []sqlite.PriceBar) (float64, error) {
		return max52w.Compute(bars, p.Cfg.Models.Max52wLookbackDays)
	})

	// --- Mega-caps: modelos 1 a 5 ---
	p.Logger.Printf("=== Calculando modelos 1-5 para mega-caps ===")
	megacapBars := p.loadAllPrices(megacapTickers, historyStart)

	report.MegacapResults[ModelMomentum12_1] = p.rankBySymbol(megacapBars, UniverseMegacap, ModelMomentum12_1, func(symbol string, bars []sqlite.PriceBar) (float64, error) {
		return momentum12_1.Compute(bars, p.AsOf, p.Cfg.Models.LookbackMonthsLong)
	})
	report.MegacapResults[ModelMax52w] = p.rankBySymbol(megacapBars, UniverseMegacap, ModelMax52w, func(symbol string, bars []sqlite.PriceBar) (float64, error) {
		return max52w.Compute(bars, p.Cfg.Models.Max52wLookbackDays)
	})
	report.MegacapResults[ModelVolAdjusted] = p.rankBySymbol(megacapBars, UniverseMegacap, ModelVolAdjusted, func(symbol string, bars []sqlite.PriceBar) (float64, error) {
		return voladjusted.Compute(bars, p.AsOf, p.Cfg.Models.LookbackMonthsLong, p.Cfg.Models.LookbackMonthsShort, p.Cfg.Models.VolAdjustedVolLookbackDays)
	})

	factorStart := p.AsOf.AddDate(-(p.Cfg.Models.ResidualRegressionWindowMonths/12 + 2), 0, 0)
	factorRows, err := p.DB.GetFactors(factorsDataset, factorStart, p.AsOf)
	if err != nil {
		p.Logger.Printf("ERROR leyendo factores para momentum residual: %v", err)
	} else {
		report.MegacapResults[ModelResidualMomentum] = p.rankBySymbol(megacapBars, UniverseMegacap, ModelResidualMomentum, func(symbol string, bars []sqlite.PriceBar) (float64, error) {
			return residualmomentum.Compute(bars, factorRows, p.AsOf, p.Cfg.Models.ResidualRegressionWindowMonths, p.Cfg.Models.ResidualMomentumMonths)
		})
	}

	// --- Modelo 5: escalado por volatilidad (a nivel de universo mega-cap) ---
	p.Logger.Printf("=== Calculando modelo 5 (escalado por volatilidad) sobre mega-caps ===")
	vsResult, err := p.computeVolScaling(megacapBars)
	if err != nil {
		p.Logger.Printf("ERROR calculando escalado por volatilidad: %v", err)
	} else {
		report.VolScaling = vsResult
	}

	// Se usa p.AsOf (el dia de la corrida) como clave, no time.Now(): asi,
	// si el pipeline se reejecuta el mismo dia, los resultados se
	// sobreescriben en vez de acumularse.
	if err := p.persistResults(p.AsOf, report); err != nil {
		p.Logger.Printf("ERROR guardando resultados: %v", err)
	}

	return report, nil
}

func (p *Pipeline) fetchPrices(tickers []config.Ticker, start time.Time) []FetchFailure {
	var failures []FetchFailure
	for _, t := range tickers {
		bars, err := p.Yahoo.GetDailyHistory(t.Symbol, start, p.AsOf)
		if err != nil {
			p.Logger.Printf("FALLO descarga %s: %v", t.Symbol, err)
			_ = p.DB.SetFetchStatus(t.Symbol, "", "error", err.Error())
			failures = append(failures, FetchFailure{Symbol: t.Symbol, Error: err.Error()})
			continue
		}
		if len(bars) == 0 {
			p.Logger.Printf("AVISO %s: 0 barras devueltas", t.Symbol)
			_ = p.DB.SetFetchStatus(t.Symbol, "", "empty", "0 barras devueltas")
			continue
		}
		if err := p.DB.UpsertBars(t.Symbol, bars); err != nil {
			p.Logger.Printf("ERROR guardando %s: %v", t.Symbol, err)
			failures = append(failures, FetchFailure{Symbol: t.Symbol, Error: err.Error()})
			continue
		}
		lastDate := bars[len(bars)-1].Date.Format("2006-01-02")
		_ = p.DB.SetFetchStatus(t.Symbol, lastDate, "ok", "")
		p.Logger.Printf("OK %s: %d barras (hasta %s)", t.Symbol, len(bars), lastDate)
	}
	return failures
}

func (p *Pipeline) fetchFactors() error {
	datasets := []string{p.Cfg.FamaFrench.ResearchFactors3, p.Cfg.FamaFrench.ResearchFactors5, p.Cfg.FamaFrench.MomentumFactor}
	for _, ds := range datasets {
		monthly, err := p.French.GetMonthly(ds)
		if err != nil {
			p.Logger.Printf("ERROR descargando dataset %s: %v", ds, err)
			continue
		}
		if err := p.DB.UpsertFactors(ds, monthly); err != nil {
			p.Logger.Printf("ERROR guardando dataset %s: %v", ds, err)
			continue
		}
		p.Logger.Printf("OK factores %s: %d meses", ds, len(monthly))
	}
	return nil
}

func (p *Pipeline) loadAllPrices(tickers []config.Ticker, start time.Time) map[string][]sqlite.PriceBar {
	out := make(map[string][]sqlite.PriceBar, len(tickers))
	for _, t := range tickers {
		bars, err := p.DB.GetPrices(t.Symbol, start, p.AsOf)
		if err != nil {
			p.Logger.Printf("ERROR leyendo precios de %s: %v", t.Symbol, err)
			continue
		}
		if len(bars) == 0 {
			continue
		}
		out[t.Symbol] = bars
	}
	return out
}

// rankBySymbol aplica scoreFn a cada simbolo, ordena descendente por score
// (mayor score = mejor rank) y devuelve los resultados ya rankeados.
func (p *Pipeline) rankBySymbol(barsBySymbol map[string][]sqlite.PriceBar, universe, model string, scoreFn func(symbol string, bars []sqlite.PriceBar) (float64, error)) []sqlite.ModelResult {
	var results []sqlite.ModelResult
	for symbol, bars := range barsBySymbol {
		score, err := scoreFn(symbol, bars)
		if err != nil {
			p.Logger.Printf("AVISO %s/%s: %v", model, symbol, err)
			continue
		}
		results = append(results, sqlite.ModelResult{
			Universe: universe,
			Model:    model,
			Symbol:   symbol,
			AsOf:     p.AsOf,
			Score:    score,
		})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	for i := range results {
		results[i].Rank = i + 1
	}

	return results
}

// computeVolScaling calcula los retornos diarios de una cartera equal-weight
// con todos los simbolos del universo mega-cap y aplica el modelo 5.
func (p *Pipeline) computeVolScaling(barsBySymbol map[string][]sqlite.PriceBar) (volscaling.Result, error) {
	trimmed := make(map[string][]sqlite.PriceBar, len(barsBySymbol))
	for symbol, bars := range barsBySymbol {
		window := common.LastNBars(common.ExcludeCurrentMonth(bars, p.AsOf), p.Cfg.Models.VolScalingLookbackDays)
		if len(window) >= 2 {
			trimmed[symbol] = window
		}
	}

	portfolioReturns := equalWeightedReturns(trimmed)
	return volscaling.Compute(portfolioReturns, p.Cfg.Models.VolScalingTargetVolAnnual)
}

// equalWeightedReturns promedia los retornos diarios de todos los simbolos
// que tengan dato en cada fecha (alineando por fecha calendario).
func equalWeightedReturns(barsBySymbol map[string][]sqlite.PriceBar) []float64 {
	returnsBySymbol := make(map[string][]float64, len(barsBySymbol))
	minLen := -1
	for symbol, bars := range barsBySymbol {
		rets := common.DailyReturns(bars)
		returnsBySymbol[symbol] = rets
		if minLen == -1 || len(rets) < minLen {
			minLen = len(rets)
		}
	}
	if minLen <= 0 {
		return nil
	}

	out := make([]float64, minLen)
	for _, rets := range returnsBySymbol {
		tail := rets[len(rets)-minLen:]
		for i, r := range tail {
			out[i] += r
		}
	}
	n := float64(len(returnsBySymbol))
	for i := range out {
		out[i] /= n
	}
	return out
}

func (p *Pipeline) persistResults(runDate time.Time, report *Report) error {
	var all []sqlite.ModelResult
	for _, results := range report.ETFResults {
		all = append(all, results...)
	}
	for _, results := range report.MegacapResults {
		all = append(all, results...)
	}
	if len(all) == 0 {
		return nil
	}
	return p.DB.InsertResults(runDate, all)
}

func limitTickers(tickers []config.Ticker, limit int) []config.Ticker {
	if limit <= 0 || limit >= len(tickers) {
		return tickers
	}
	return tickers[:limit]
}

// tickerNames junta todas las listas de tickers en un mapa symbol -> nombre,
// usado por el reporte HTML para mostrar el nombre de la empresa/fondo.
func tickerNames(tickerLists ...[]config.Ticker) map[string]string {
	names := map[string]string{}
	for _, list := range tickerLists {
		for _, t := range list {
			names[t.Symbol] = t.Name
		}
	}
	return names
}
