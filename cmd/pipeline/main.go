// Comando principal: corre el pipeline completo de analisis de momentum
// sobre ETFs de Vanguard y acciones mega-cap de EEUU, y genera el reporte HTML.
package main

import (
	"flag"
	"log"
	"os"
	"time"

	"modelofinanzas/internal/config"
	"modelofinanzas/internal/famafrench"
	"modelofinanzas/internal/logging"
	"modelofinanzas/internal/pipeline"
	"modelofinanzas/internal/report/html"
	"modelofinanzas/internal/storage/sqlite"
	"modelofinanzas/internal/yahoo"
)

func main() {
	configPath := flag.String("config", "configs/settings.yaml", "ruta al archivo de configuracion")
	skipFetch := flag.Bool("skip-fetch", false, "si esta presente, no descarga datos nuevos; usa lo que ya hay en la base")
	limit := flag.Int("limit", 0, "si es > 0, limita la cantidad de tickers por universo (util para pruebas)")
	resetToday := flag.Bool("reset-today", false, "borra los precios y resultados del dia de hoy antes de correr, para forzar una recorrida limpia")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("cargando configuracion: %v", err)
	}

	logger, logFile, err := logging.New(cfg.Logging.Dir, cfg.Logging.File)
	if err != nil {
		log.Fatalf("iniciando logger: %v", err)
	}
	defer logFile.Close()

	logger.Println("=== Inicio del pipeline de analisis de momentum ===")

	db, err := sqlite.Open(cfg.Database.Path)
	if err != nil {
		logger.Fatalf("abriendo base de datos: %v", err)
	}
	defer db.Close()

	yc := yahoo.NewClient(
		time.Duration(cfg.Yahoo.TimeoutSeconds)*time.Second,
		time.Duration(cfg.Yahoo.RequestPauseMs)*time.Millisecond,
		time.Duration(cfg.Yahoo.RetryBackoffMs)*time.Millisecond,
		cfg.Yahoo.MaxRetries,
		logger,
	)
	fc := famafrench.NewClient(time.Duration(cfg.Yahoo.TimeoutSeconds) * time.Second)

	pl := &pipeline.Pipeline{
		Cfg:    cfg,
		DB:     db,
		Logger: logger,
		Yahoo:  yc,
		French: fc,
		AsOf:   time.Now(),
	}

	report, err := pl.Run(pipeline.Options{SkipFetch: *skipFetch, Limit: *limit, ResetToday: *resetToday})
	if err != nil {
		logger.Fatalf("corriendo pipeline: %v", err)
	}

	// Se guarda un archivo con timestamp (historial de corridas) y ademas se
	// sobreescribe "reporte_ultimo.html" como puntero estable a la corrida mas reciente.
	filename := "reporte_" + report.GeneratedAt.Format("20060102_150405") + ".html"
	path, err := html.Render(report, cfg.Output.HTMLDir, filename)
	if err != nil {
		logger.Fatalf("generando reporte HTML: %v", err)
	}
	if _, err := html.Render(report, cfg.Output.HTMLDir, "reporte_ultimo.html"); err != nil {
		logger.Printf("AVISO: no se pudo actualizar reporte_ultimo.html: %v", err)
	}

	logger.Printf("Reporte generado en: %s (y reporte_ultimo.html)", path)
	logger.Println("=== Fin del pipeline ===")

	if len(report.FailedFetches) > 0 {
		os.Exit(1)
	}
}
