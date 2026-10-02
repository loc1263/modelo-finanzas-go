// Package config carga la configuracion del pipeline desde configs/settings.yaml.
package config

import (
	"fmt"
	"os"

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

// Load lee y parsea el archivo de configuracion YAML en path.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("leyendo config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parseando config %s: %w", path, err)
	}

	return &cfg, nil
}
