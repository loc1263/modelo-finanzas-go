// Package config carga la configuracion del pipeline desde configs/settings.yaml.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`

	Output struct {
		HTMLDir string `yaml:"html_dir"`
		// Dias de retencion para los reporte_<timestamp>.html viejos (no
		// afecta a reporte_ultimo.html, que siempre se conserva). <= 0
		// desactiva el borrado automatico.
		RetentionDays int `yaml:"retention_days"`
	} `yaml:"output"`

	Logging struct {
		Dir  string `yaml:"dir"`
		File string `yaml:"file"`
		// Dias de retencion para los logs viejos (un archivo por dia,
		// ver internal/logging). <= 0 desactiva el borrado automatico.
		RetentionDays int `yaml:"retention_days"`
	} `yaml:"logging"`

	Universe struct {
		VanguardETFsCSV  string `yaml:"vanguard_etfs_csv"`
		MegacapStocksCSV string `yaml:"megacap_stocks_csv"`
	} `yaml:"universe"`

	Yahoo struct {
		RequestPauseMs int `yaml:"request_pause_ms"`
		MaxRetries     int `yaml:"max_retries"`
		RetryBackoffMs int `yaml:"retry_backoff_ms"`
		TimeoutSeconds int `yaml:"timeout_seconds"`
		HistoryYears   int `yaml:"history_years"`
		// Cuantos tickers se descargan en paralelo (precios y earnings).
		// Cada worker sigue respetando su propia pausa (request_pause_ms)
		// entre sus propios requests — no se reparte ni se achica la pausa
		// al subir la concurrencia. 1 = secuencial (comportamiento
		// original). Subir esto acerca el ritmo total de requests a Yahoo a
		// concurrency/request_pause_ms; mantenerlo chico (2-3) para no
		// arriesgar que Yahoo empiece a bloquear o devolver 429.
		Concurrency int `yaml:"concurrency"`
	} `yaml:"yahoo"`

	FamaFrench struct {
		ResearchFactors3 string `yaml:"research_factors_3"`
		ResearchFactors5 string `yaml:"research_factors_5"`
		MomentumFactor   string `yaml:"momentum_factor"`
	} `yaml:"famafrench"`

	Models struct {
		LookbackMonthsLong             int     `yaml:"lookback_months_long"`
		LookbackMonthsShort            int     `yaml:"lookback_months_short"`
		Max52wLookbackDays             int     `yaml:"max52w_lookback_days"`
		VolAdjustedVolLookbackDays     int     `yaml:"voladjusted_vol_lookback_days"`
		ResidualRegressionWindowMonths int     `yaml:"residual_regression_window_months"`
		ResidualMomentumMonths         int     `yaml:"residual_momentum_months"`
		VolScalingTargetVolAnnual      float64 `yaml:"vol_scaling_target_vol_annual"`
		VolScalingLookbackDays         int     `yaml:"vol_scaling_lookback_days"`
		// Modelo 6 (momentum acelerado): meses del tramo "corto" a comparar
		// contra el tramo largo (LookbackMonthsLong).
		AccelShortMonths int `yaml:"accel_short_months"`
	} `yaml:"models"`
}

// Load lee, parsea y valida el archivo de configuracion YAML en path.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("leyendo config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parseando config %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}

	return &cfg, nil
}

// Validate chequea que los valores cargados sean coherentes entre si, para
// fallar con un error claro al arrancar en vez de como docenas de AVISOs
// sueltos por ticker cuando un modelo no puede calcular nada por un valor
// mal puesto (ej. accel_short_months en 0, o lookback_months_short mayor
// que lookback_months_long). Junta TODOS los problemas encontrados en un
// solo error, no solo el primero, para no obligar a corregir de a uno.
func (c *Config) Validate() error {
	var errs []string
	check := func(cond bool, msg string) {
		if !cond {
			errs = append(errs, msg)
		}
	}

	check(c.Database.Path != "", "database.path no puede estar vacio")

	check(c.Output.HTMLDir != "", "output.html_dir no puede estar vacio")
	check(c.Output.RetentionDays >= 0, "output.retention_days no puede ser negativo (usar 0 para desactivar el borrado automatico)")

	check(c.Logging.Dir != "", "logging.dir no puede estar vacio")
	check(c.Logging.File != "", "logging.file no puede estar vacio")
	check(c.Logging.RetentionDays >= 0, "logging.retention_days no puede ser negativo (usar 0 para desactivar el borrado automatico)")

	check(c.Universe.VanguardETFsCSV != "", "universe.vanguard_etfs_csv no puede estar vacio")
	check(c.Universe.MegacapStocksCSV != "", "universe.megacap_stocks_csv no puede estar vacio")

	check(c.Yahoo.TimeoutSeconds > 0, "yahoo.timeout_seconds debe ser > 0")
	check(c.Yahoo.RequestPauseMs >= 0, "yahoo.request_pause_ms no puede ser negativo")
	check(c.Yahoo.MaxRetries >= 0, "yahoo.max_retries no puede ser negativo")
	check(c.Yahoo.RetryBackoffMs >= 0, "yahoo.retry_backoff_ms no puede ser negativo")
	check(c.Yahoo.HistoryYears > 0, "yahoo.history_years debe ser > 0")
	check(c.Yahoo.Concurrency >= 0, "yahoo.concurrency no puede ser negativo (0 o 1 = secuencial)")

	check(c.FamaFrench.ResearchFactors3 != "", "famafrench.research_factors_3 no puede estar vacio")
	check(c.FamaFrench.ResearchFactors5 != "", "famafrench.research_factors_5 no puede estar vacio")
	check(c.FamaFrench.MomentumFactor != "", "famafrench.momentum_factor no puede estar vacio")

	check(c.Models.LookbackMonthsLong > 0, "models.lookback_months_long debe ser > 0")
	check(c.Models.LookbackMonthsShort > 0, "models.lookback_months_short debe ser > 0")
	check(c.Models.LookbackMonthsShort < c.Models.LookbackMonthsLong,
		"models.lookback_months_short debe ser menor que models.lookback_months_long")
	check(c.Models.Max52wLookbackDays > 0, "models.max52w_lookback_days debe ser > 0")
	check(c.Models.VolAdjustedVolLookbackDays > 0, "models.voladjusted_vol_lookback_days debe ser > 0")
	check(c.Models.ResidualRegressionWindowMonths > 0, "models.residual_regression_window_months debe ser > 0")
	check(c.Models.ResidualMomentumMonths > 0, "models.residual_momentum_months debe ser > 0")
	check(c.Models.ResidualMomentumMonths <= c.Models.ResidualRegressionWindowMonths,
		"models.residual_momentum_months no puede ser mayor que models.residual_regression_window_months")
	check(c.Models.VolScalingTargetVolAnnual > 0, "models.vol_scaling_target_vol_annual debe ser > 0")
	check(c.Models.VolScalingLookbackDays > 0, "models.vol_scaling_lookback_days debe ser > 0")
	check(c.Models.AccelShortMonths > 0, "models.accel_short_months debe ser > 0")
	check(c.Models.AccelShortMonths < c.Models.LookbackMonthsLong,
		"models.accel_short_months debe ser menor que models.lookback_months_long")

	if len(errs) > 0 {
		return fmt.Errorf("valores invalidos:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}
