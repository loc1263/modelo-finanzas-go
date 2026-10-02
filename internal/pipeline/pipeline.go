// Package pipeline orquesta todo el proceso: descarga de datos, calculo de
// los modelos y preparacion de los resultados para el reporte.
package pipeline

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"

	"modelofinanzas/internal/config"
	"modelofinanzas/internal/famafrench"
	"modelofinanzas/internal/models/absolutemomentum"
	"modelofinanzas/internal/models/accelmomentum"
	"modelofinanzas/internal/models/common"
	"modelofinanzas/internal/models/earningsmomentum"
	"modelofinanzas/internal/models/frogpan"
	"modelofinanzas/internal/models/liquidityadjusted"
	"modelofinanzas/internal/models/markowitz"
	"modelofinanzas/internal/models/max52w"
	"modelofinanzas/internal/models/momentum12_1"
	"modelofinanzas/internal/models/residualmomentum"
	"modelofinanzas/internal/models/sectormomentum"
	"modelofinanzas/internal/models/voladjusted"
	"modelofinanzas/internal/models/volscaling"
	"modelofinanzas/internal/storage/sqlite"
	"modelofinanzas/internal/yahoo"
)

const (
	UniverseVanguardETF = "vanguard_etf"
	UniverseMegacap     = "megacap"

	ModelMomentum12_1      = "momentum_12_1"
	ModelMax52w            = "max_52w"
	ModelVolAdjusted       = "vol_adjusted"
	ModelResidualMomentum  = "residual_momentum"
	ModelAccelMomentum     = "accel_momentum"
	ModelAbsoluteMomentum  = "absolute_momentum"
	ModelEarningsMomentum  = "earnings_momentum"
	ModelFrogInThePan      = "frog_in_the_pan"
	ModelSectorMomentum    = "sector_momentum"
	ModelLiquidityAdjusted = "liquidity_adjusted_momentum"

	// Construccion de cartera (simulacion de Markowitz). No son modelos de
	// momentum — se muestran en una seccion aparte del reporte, no junto a
	// los modelos 1-11 (ver Modelos_de_Momentum.md).
	ModelMarkowitzTopMomentum  = "markowitz_top_momentum"
	ModelMarkowitzFullUniverse = "markowitz_full_universe"

	factorsDataset = "F-F_Research_Data_Factors"

	// Yahoo devuelve tipicamente los ultimos 4 trimestres de earningsHistory;
	// se usa el mismo numero como tope al leer de la base, para que el score
	// del modelo 8 siempre signifique "ultimos ~4 trimestres" y no crezca
	// solo con el paso del tiempo (ver Mejoras_Pendientes.md, MEJ-002).
	earningsMaxQuarters = 4
)

// UniverseModel identifica un par (universo, modelo) de los que persiste
// persistResults, usado por el flag -history de cmd/pipeline (MEJ-013) para
// saber que consultar en model_results sin adivinar nombres sueltos.
type UniverseModel struct {
	Universe string
	Model    string
}

// KnownUniverseModels devuelve, en el orden en que se calculan, todos los
// pares (universo, modelo) que una corrida puede llegar a guardar en
// model_results. El modelo 5 (escalado por volatilidad, Report.VolScaling)
// no aparece: es un resultado a nivel de universo, no por simbolo, y no se
// persiste en esta tabla.
func KnownUniverseModels() []UniverseModel {
	return []UniverseModel{
		{UniverseVanguardETF, ModelMomentum12_1},
		{UniverseVanguardETF, ModelMax52w},
		{UniverseMegacap, ModelMomentum12_1},
		{UniverseMegacap, ModelMax52w},
		{UniverseMegacap, ModelVolAdjusted},
		{UniverseMegacap, ModelResidualMomentum},
		{UniverseMegacap, ModelAbsoluteMomentum},
		{UniverseMegacap, ModelAccelMomentum},
		{UniverseMegacap, ModelFrogInThePan},
		{UniverseMegacap, ModelSectorMomentum},
		{UniverseMegacap, ModelLiquidityAdjusted},
		{UniverseMegacap, ModelEarningsMomentum},
		{UniverseMegacap, ModelMarkowitzTopMomentum},
		{UniverseMegacap, ModelMarkowitzFullUniverse},
	}
}

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

	// Construccion de cartera (simulacion de Markowitz): separado de
	// MegacapResults a proposito, no es un modelo de momentum mas (ver
	// Modelos_de_Momentum.md, seccion "Construccion de cartera"). *Stats
	// trae el retorno/volatilidad/Sharpe de la cartera optima encontrada
	// (para el resumen); *Ranked trae los mismos pesos como ranking, para
	// la tabla paginada y la persistencia en model_results.
	PortfolioTopMomentum        markowitz.Result
	PortfolioTopMomentumRanked  []sqlite.ModelResult
	PortfolioFullUniverse       markowitz.Result
	PortfolioFullUniverseRanked []sqlite.ModelResult
	// MarkowitzTopMomentumN es Cfg.Models.MarkowitzTopMomentumN, copiado aca
	// para que el template pueda mostrarlo en el heading aunque la
	// simulacion haya fallado (PortfolioTopMomentumRanked quedaria vacio).
	MarkowitzTopMomentumN int
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

		p.Logger.Printf("=== Descargando earnings: %d acciones mega-cap (modelo 8) ===", len(megacapTickers))
		p.fetchEarnings(megacapTickers)

		p.Logger.Printf("=== Descargando factores de Fama-French ===")
		if err := p.fetchFactors(); err != nil {
			p.Logger.Printf("ERROR descargando factores Fama-French: %v", err)
		}
	} else {
		p.Logger.Printf("=== SkipFetch activo: usando datos ya existentes en la base ===")
	}

	report := &Report{
		GeneratedAt:           time.Now().UTC(),
		AsOf:                  p.AsOf,
		ETFResults:            map[string][]sqlite.ModelResult{},
		MegacapResults:        map[string][]sqlite.ModelResult{},
		FailedFetches:         failures,
		TickerNames:           tickerNames(etfTickers, megacapTickers),
		MarkowitzTopMomentumN: p.Cfg.Models.MarkowitzTopMomentumN,
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

	// --- Mega-caps: modelos 1 a 11 ---
	p.Logger.Printf("=== Calculando modelos para mega-caps ===")
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

	// factorRows se usa tanto para el momentum residual (modelo 4) como para
	// el momentum absoluto (modelo 7): ambos necesitan el factor RF/3-factores
	// de Fama-French, asi que se descarga una sola vez.
	factorStart := p.AsOf.AddDate(-(p.Cfg.Models.ResidualRegressionWindowMonths/12 + 2), 0, 0)
	factorRows, err := p.DB.GetFactors(factorsDataset, factorStart, p.AsOf)
	if err != nil {
		p.Logger.Printf("ERROR leyendo factores de Fama-French: %v", err)
	} else {
		report.MegacapResults[ModelResidualMomentum] = p.rankBySymbol(megacapBars, UniverseMegacap, ModelResidualMomentum, func(symbol string, bars []sqlite.PriceBar) (float64, error) {
			return residualmomentum.Compute(bars, factorRows, p.AsOf, p.Cfg.Models.ResidualRegressionWindowMonths, p.Cfg.Models.ResidualMomentumMonths)
		})
		report.MegacapResults[ModelAbsoluteMomentum] = p.rankBySymbol(megacapBars, UniverseMegacap, ModelAbsoluteMomentum, func(symbol string, bars []sqlite.PriceBar) (float64, error) {
			return absolutemomentum.Compute(bars, factorRows, p.AsOf, p.Cfg.Models.LookbackMonthsLong)
		})

		// --- Construccion de cartera: simulacion de Markowitz ---
		// Comparte el requisito de datos de Fama-French con los modelos 4 y
		// 7 (necesita RF para el Sharpe ratio), por eso vive en esta misma
		// rama. rng se siembra de forma deterministica segun el dia de la
		// corrida (no la hora), para que reejecutar el pipeline el mismo
		// dia encuentre siempre la misma cartera optima.
		p.Logger.Printf("=== Calculando construccion de cartera (simulacion de Markowitz) ===")
		if riskFreeAnnual, err := trailingAnnualRF(factorRows, p.Cfg.Models.LookbackMonthsLong); err != nil {
			p.Logger.Printf("ERROR calculando tasa libre de riesgo para Markowitz: %v", err)
		} else {
			dayKey, _ := strconv.ParseInt(p.AsOf.Format("20060102"), 10, 64)
			rng := rand.New(rand.NewSource(dayKey))

			topMomentumBars := topNBySymbol(megacapBars, report.MegacapResults[ModelMomentum12_1], p.Cfg.Models.MarkowitzTopMomentumN)
			if result, ranked, err := p.computeMarkowitzPortfolio(topMomentumBars, ModelMarkowitzTopMomentum, rng, riskFreeAnnual); err != nil {
				p.Logger.Printf("ERROR en simulacion de Markowitz (top momentum): %v", err)
			} else {
				report.PortfolioTopMomentum = result
				report.PortfolioTopMomentumRanked = ranked
			}

			if result, ranked, err := p.computeMarkowitzPortfolio(megacapBars, ModelMarkowitzFullUniverse, rng, riskFreeAnnual); err != nil {
				p.Logger.Printf("ERROR en simulacion de Markowitz (universo completo): %v", err)
			} else {
				report.PortfolioFullUniverse = result
				report.PortfolioFullUniverseRanked = ranked
			}
		}
	}

	report.MegacapResults[ModelAccelMomentum] = p.rankBySymbol(megacapBars, UniverseMegacap, ModelAccelMomentum, func(symbol string, bars []sqlite.PriceBar) (float64, error) {
		return accelmomentum.Compute(bars, p.AsOf, p.Cfg.Models.AccelShortMonths, p.Cfg.Models.LookbackMonthsLong)
	})

	report.MegacapResults[ModelFrogInThePan] = p.rankBySymbol(megacapBars, UniverseMegacap, ModelFrogInThePan, func(symbol string, bars []sqlite.PriceBar) (float64, error) {
		return frogpan.Compute(bars, p.AsOf, p.Cfg.Models.LookbackMonthsLong)
	})

	report.MegacapResults[ModelSectorMomentum] = p.computeSectorMomentum(megacapTickers, etfBars)

	report.MegacapResults[ModelLiquidityAdjusted] = p.computeLiquidityAdjustedMomentum(megacapBars)

	report.MegacapResults[ModelEarningsMomentum] = p.computeEarningsMomentum(megacapTickers)

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

// parallelForEach corre fn una vez por cada elemento de items, con hasta
// workers goroutines activas al mismo tiempo (minimo 1; si items tiene
// menos elementos que workers, se usan solo los necesarios). Bloquea hasta
// que todas las llamadas terminan.
//
// Importante para no arriesgar que Yahoo bloquee el acceso: esto NO reparte
// ni achica ninguna pausa entre requests — cada goroutine que llama a fn
// sigue esperando su propia pausa completa (yahoo.Client.Pause) antes de
// cada request suyo, exactamente igual que en el modo secuencial (workers=1).
// Lo unico que cambia es que, con workers>1, varias de esas esperas y
// requests ocurren en simultaneo en vez de una despues de la otra, asi que
// el ritmo total de requests a Yahoo queda, a lo sumo, en
// aproximadamente workers/Pause — por eso Cfg.Yahoo.Concurrency se
// recomienda mantener chico (2-3).
func parallelForEach[T any](items []T, workers int, fn func(T)) {
	if workers < 1 {
		workers = 1
	}
	if workers > len(items) {
		workers = len(items)
	}
	if workers <= 1 {
		for _, item := range items {
			fn(item)
		}
		return
	}

	jobs := make(chan T)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for item := range jobs {
				fn(item)
			}
		})
	}
	for _, item := range items {
		jobs <- item
	}
	close(jobs)
	wg.Wait()
}

func (p *Pipeline) fetchPrices(tickers []config.Ticker, start time.Time) []FetchFailure {
	var mu sync.Mutex
	var failures []FetchFailure

	parallelForEach(tickers, p.Cfg.Yahoo.Concurrency, func(t config.Ticker) {
		bars, err := p.Yahoo.GetDailyHistory(t.Symbol, start, p.AsOf)
		if err != nil {
			p.Logger.Printf("FALLO descarga %s: %v", t.Symbol, err)
			mu.Lock()
			failures = append(failures, FetchFailure{Symbol: t.Symbol, Error: err.Error()})
			mu.Unlock()
			return
		}
		if len(bars) == 0 {
			p.Logger.Printf("AVISO %s: 0 barras devueltas", t.Symbol)
			return
		}
		if err := p.DB.UpsertBars(t.Symbol, bars); err != nil {
			p.Logger.Printf("ERROR guardando %s: %v", t.Symbol, err)
			mu.Lock()
			failures = append(failures, FetchFailure{Symbol: t.Symbol, Error: err.Error()})
			mu.Unlock()
			return
		}
		lastDate := bars[len(bars)-1].Date.Format("2006-01-02")
		p.Logger.Printf("OK %s: %d barras (hasta %s)", t.Symbol, len(bars), lastDate)
	})

	return failures
}

// fetchEarnings descarga el historial de earnings de cada ticker (solo se
// usa para mega-caps, modelo 8). A diferencia de fetchPrices, no devuelve
// FetchFailure: la cobertura de analistas puede faltar para algunas
// acciones sin que eso sea un fallo del pipeline (se loguea como AVISO y
// esa accion simplemente no aparece en el ranking del modelo 8).
func (p *Pipeline) fetchEarnings(tickers []config.Ticker) {
	parallelForEach(tickers, p.Cfg.Yahoo.Concurrency, func(t config.Ticker) {
		quarters, err := p.Yahoo.GetEarningsHistory(t.Symbol)
		if err != nil {
			p.Logger.Printf("AVISO earnings %s: %v", t.Symbol, err)
			return
		}
		if len(quarters) == 0 {
			p.Logger.Printf("AVISO earnings %s: sin trimestres devueltos", t.Symbol)
			return
		}
		if err := p.DB.UpsertEarnings(t.Symbol, quarters); err != nil {
			p.Logger.Printf("ERROR guardando earnings %s: %v", t.Symbol, err)
			return
		}
		p.Logger.Printf("OK earnings %s: %d trimestres", t.Symbol, len(quarters))
	})
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

	sortResultsDesc(results)
	for i := range results {
		results[i].Rank = i + 1
	}

	return results
}

// sortResultsDesc ordena por score descendente (mayor score = mejor rank),
// con el simbolo como desempate alfabetico para que el orden sea
// deterministico entre corridas cuando dos scores quedan exactamente
// iguales (ej. modelo sectorial, donde todas las acciones de un mismo
// sector comparten score).
func sortResultsDesc(results []sqlite.ModelResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Symbol < results[j].Symbol
	})
}

// computeEarningsMomentum implementa el modelo 8: lee el historial de
// earnings ya descargado (fetchEarnings) y calcula el score de cada accion.
// Las acciones sin historial de earnings en la base (sin cobertura de
// analistas, o que fallaron al descargar) quedan fuera del ranking.
func (p *Pipeline) computeEarningsMomentum(megacapTickers []config.Ticker) []sqlite.ModelResult {
	var results []sqlite.ModelResult
	for _, t := range megacapTickers {
		quarters, err := p.DB.GetEarnings(t.Symbol, earningsMaxQuarters)
		if err != nil {
			p.Logger.Printf("ERROR leyendo earnings de %s: %v", t.Symbol, err)
			continue
		}
		if len(quarters) == 0 {
			continue
		}

		score, err := earningsmomentum.Compute(quarters)
		if err != nil {
			p.Logger.Printf("AVISO %s/%s: %v", ModelEarningsMomentum, t.Symbol, err)
			continue
		}

		results = append(results, sqlite.ModelResult{
			Universe: UniverseMegacap,
			Model:    ModelEarningsMomentum,
			Symbol:   t.Symbol,
			AsOf:     p.AsOf,
			Score:    score,
		})
	}

	sortResultsDesc(results)
	for i := range results {
		results[i].Rank = i + 1
	}

	return results
}

// computeSectorMomentum implementa el modelo 10: a cada accion mega-cap le
// asigna el momentum 12-1 del ETF sectorial de Vanguard que la representa
// (columna sector_etf de configs/megacap_stocks.csv). etfBars son las
// barras ya cargadas del universo de ETFs (se reusan, no se vuelven a pedir).
func (p *Pipeline) computeSectorMomentum(megacapTickers []config.Ticker, etfBars map[string][]sqlite.PriceBar) []sqlite.ModelResult {
	sectorMomentumCache := map[string]float64{}

	var results []sqlite.ModelResult
	for _, t := range megacapTickers {
		if t.SectorETF == "" {
			p.Logger.Printf("AVISO %s/%s: sin sector_etf asignado en megacap_stocks.csv", ModelSectorMomentum, t.Symbol)
			continue
		}

		score, ok := sectorMomentumCache[t.SectorETF]
		if !ok {
			bars, hasBars := etfBars[t.SectorETF]
			if !hasBars {
				p.Logger.Printf("AVISO %s/%s: sin precios del ETF sectorial %s", ModelSectorMomentum, t.Symbol, t.SectorETF)
				continue
			}
			sectorScore, err := sectormomentum.Compute(bars, p.AsOf, p.Cfg.Models.LookbackMonthsLong)
			if err != nil {
				p.Logger.Printf("AVISO %s/%s: %v", ModelSectorMomentum, t.SectorETF, err)
				continue
			}
			sectorMomentumCache[t.SectorETF] = sectorScore
			score = sectorScore
		}

		results = append(results, sqlite.ModelResult{
			Universe: UniverseMegacap,
			Model:    ModelSectorMomentum,
			Symbol:   t.Symbol,
			AsOf:     p.AsOf,
			Score:    score,
		})
	}

	sortResultsDesc(results)
	for i := range results {
		results[i].Rank = i + 1
	}

	return results
}

// computeLiquidityAdjustedMomentum implementa el modelo 11: calcula el
// momentum 12-1 y la iliquidez de Amihud de cada accion, y pondera el
// momentum por un score de liquidez relativo al resto del universo (1 =
// la mas liquida). Es un calculo cruzado (necesita el universo completo
// antes de poder puntuar a cualquier accion), por eso no usa rankBySymbol.
func (p *Pipeline) computeLiquidityAdjustedMomentum(barsBySymbol map[string][]sqlite.PriceBar) []sqlite.ModelResult {
	type row struct {
		Symbol      string
		Momentum    float64
		Illiquidity float64
	}

	var rows []row
	for symbol, bars := range barsBySymbol {
		mom, err := momentum12_1.Compute(bars, p.AsOf, p.Cfg.Models.LookbackMonthsLong)
		if err != nil {
			p.Logger.Printf("AVISO %s/%s: %v", ModelLiquidityAdjusted, symbol, err)
			continue
		}

		window := common.LastNBars(common.ExcludeCurrentMonth(bars, p.AsOf), p.Cfg.Models.Max52wLookbackDays)
		illiq, err := liquidityadjusted.Illiquidity(window)
		if err != nil {
			p.Logger.Printf("AVISO %s/%s: %v", ModelLiquidityAdjusted, symbol, err)
			continue
		}

		rows = append(rows, row{Symbol: symbol, Momentum: mom, Illiquidity: illiq})
	}

	// Menor iliquidez primero (mas liquida), para asignar el score de liquidez.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Illiquidity != rows[j].Illiquidity {
			return rows[i].Illiquidity < rows[j].Illiquidity
		}
		return rows[i].Symbol < rows[j].Symbol
	})

	// Escala de la penalidad por iliquidez: el desvio estandar del momentum
	// del propio universo, para que la penalidad sea de una magnitud
	// comparable a la señal (ver liquidityadjusted.CombineWithMomentum).
	momentums := make([]float64, len(rows))
	for i, r := range rows {
		momentums[i] = r.Momentum
	}
	penaltyScale := common.StdDev(momentums)

	n := len(rows)
	results := make([]sqlite.ModelResult, 0, n)
	for i, r := range rows {
		liquidityScore := float64(n-i) / float64(n)
		results = append(results, sqlite.ModelResult{
			Universe: UniverseMegacap,
			Model:    ModelLiquidityAdjusted,
			Symbol:   r.Symbol,
			AsOf:     p.AsOf,
			Score:    liquidityadjusted.CombineWithMomentum(r.Momentum, liquidityScore, penaltyScale),
		})
	}

	sortResultsDesc(results)
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

// topNBySymbol filtra barsBySymbol dejando solo los symbols de las primeras
// n filas de ranked (ya ordenado por rank ascendente, ver sortResultsDesc).
// Usado para correr la simulacion de Markowitz sobre las acciones de mejor
// momentum en vez de sobre todo el universo.
func topNBySymbol(barsBySymbol map[string][]sqlite.PriceBar, ranked []sqlite.ModelResult, n int) map[string][]sqlite.PriceBar {
	if n > len(ranked) {
		n = len(ranked)
	}
	out := make(map[string][]sqlite.PriceBar, n)
	for _, r := range ranked[:n] {
		if bars, ok := barsBySymbol[r.Symbol]; ok {
			out[r.Symbol] = bars
		}
	}
	return out
}

// trailingAnnualRF compone la tasa libre de riesgo (factor RF de
// Fama-French) de los ultimos `months` meses disponibles, para usar como
// tasa de referencia del Sharpe ratio en la simulacion de Markowitz.
// factorRows debe venir ordenado por fecha ascendente (ver GetFactors).
func trailingAnnualRF(factorRows []sqlite.FactorRow, months int) (float64, error) {
	var rfValues []float64
	for _, fr := range factorRows {
		if rf, ok := fr.Values["RF"]; ok {
			rfValues = append(rfValues, rf)
		}
	}
	if len(rfValues) < months {
		return 0, fmt.Errorf("se necesitan %d meses de RF, hay %d", months, len(rfValues))
	}

	compounded := 1.0
	for _, rf := range rfValues[len(rfValues)-months:] {
		compounded *= 1 + rf
	}
	return compounded - 1, nil
}

// computeMarkowitzPortfolio alinea los retornos diarios de barsBySymbol (ver
// markowitz.Simulate) y corre la simulacion, devolviendo los pesos de la
// cartera optima como un ranking (mayor peso primero) para reusar el mismo
// formato de tabla que los modelos de momentum.
func (p *Pipeline) computeMarkowitzPortfolio(barsBySymbol map[string][]sqlite.PriceBar, modelName string, rng *rand.Rand, riskFreeAnnual float64) (markowitz.Result, []sqlite.ModelResult, error) {
	trimmed := make(map[string][]float64, len(barsBySymbol))
	minLen := -1
	for symbol, bars := range barsBySymbol {
		window := common.LastNBars(common.ExcludeCurrentMonth(bars, p.AsOf), p.Cfg.Models.MarkowitzLookbackDays)
		rets := common.DailyReturns(window)
		if len(rets) < 2 {
			continue
		}
		trimmed[symbol] = rets
		if minLen == -1 || len(rets) < minLen {
			minLen = len(rets)
		}
	}
	if len(trimmed) < 2 {
		return markowitz.Result{}, nil, fmt.Errorf("se necesitan al menos 2 simbolos con historia suficiente, hay %d", len(trimmed))
	}

	aligned := make(map[string][]float64, len(trimmed))
	for symbol, rets := range trimmed {
		aligned[symbol] = rets[len(rets)-minLen:]
	}

	result, err := markowitz.Simulate(aligned, p.Cfg.Models.MarkowitzSimulations, riskFreeAnnual, rng)
	if err != nil {
		return markowitz.Result{}, nil, err
	}

	ranked := make([]sqlite.ModelResult, 0, len(result.Weights))
	for symbol, weight := range result.Weights {
		ranked = append(ranked, sqlite.ModelResult{
			Universe: UniverseMegacap,
			Model:    modelName,
			Symbol:   symbol,
			AsOf:     p.AsOf,
			Score:    weight,
		})
	}
	sortResultsDesc(ranked)
	for i := range ranked {
		ranked[i].Rank = i + 1
	}

	return result, ranked, nil
}

func (p *Pipeline) persistResults(runDate time.Time, report *Report) error {
	var all []sqlite.ModelResult
	for _, results := range report.ETFResults {
		all = append(all, results...)
	}
	for _, results := range report.MegacapResults {
		all = append(all, results...)
	}
	all = append(all, report.PortfolioTopMomentumRanked...)
	all = append(all, report.PortfolioFullUniverseRanked...)
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
