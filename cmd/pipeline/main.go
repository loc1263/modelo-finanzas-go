// Autor: Luis Oyanader C
//
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
	os.Exit(run())
}

// run contiene toda la logica del comando y devuelve el codigo de salida del
// proceso (0 = ok, 1 = error). Centralizar la salida aca, en vez de llamar a
// log.Fatal/os.Exit desde varios puntos de main, garantiza que los `defer`
// de cierre de recursos (base de datos, archivo de log) siempre corran antes
// de que el proceso termine.
func run() int {
	configPath := flag.String("config", "configs/settings.yaml", "ruta al archivo de configuracion")
	skipFetch := flag.Bool("skip-fetch", false, "si esta presente, no descarga datos nuevos; usa lo que ya hay en la base")
	limit := flag.Int("limit", 0, "si es > 0, limita la cantidad de tickers por universo (util para pruebas)")
	resetToday := flag.Bool("reset-today", false, "borra los precios y resultados del dia de hoy antes de correr, para forzar una recorrida limpia")
	historyDate := flag.String("history", "", "si esta presente (formato YYYY-MM-DD), no corre el pipeline: imprime en stdout los resultados ya guardados para ese dia y termina")
	flag.Parse()

	now := time.Now()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Printf("cargando configuracion: %v", err)
		return 1
	}

	if *historyDate != "" {
		db, err := sqlite.Open(cfg.Database.Path)
		if err != nil {
			log.Printf("abriendo base de datos: %v", err)
			return 1
		}
		defer db.Close()

		if err := runHistory(db, *historyDate); err != nil {
			log.Printf("%v", err)
			return 1
		}
		return 0
	}

	logger, logFile, err := logging.New(cfg.Logging.Dir, cfg.Logging.File, now, cfg.Logging.RetentionDays)
	if err != nil {
		log.Printf("iniciando logger: %v", err)
		return 1
	}
	defer logFile.Close()

	logger.Println("=== Inicio del pipeline de analisis de momentum ===")

	db, err := sqlite.Open(cfg.Database.Path)
	if err != nil {
		logger.Printf("abriendo base de datos: %v", err)
		return 1
	}
	defer db.Close()

	yc := yahoo.NewClient(
		time.Duration(cfg.Yahoo.TimeoutSeconds)*time.Second,
		time.Duration(cfg.Yahoo.RequestPauseMs)*time.Millisecond,
		time.Duration(cfg.Yahoo.RetryBackoffMs)*time.Millisecond,
		cfg.Yahoo.MaxRetries,
		logger,
	)
	fc := famafrench.NewClient(
		time.Duration(cfg.Yahoo.TimeoutSeconds)*time.Second,
		time.Duration(cfg.Yahoo.RetryBackoffMs)*time.Millisecond,
		cfg.Yahoo.MaxRetries,
		logger,
	)

	pl := &pipeline.Pipeline{
		Cfg:    cfg,
		DB:     db,
		Logger: logger,
		Yahoo:  yc,
		French: fc,
		AsOf:   now,
	}

	report, err := pl.Run(pipeline.Options{SkipFetch: *skipFetch, Limit: *limit, ResetToday: *resetToday})
	if err != nil {
		logger.Printf("corriendo pipeline: %v", err)
		return 1
	}

	// Se guarda un archivo con timestamp (historial de corridas) y ademas se
	// sobreescribe "reporte_ultimo.html" como puntero estable a la corrida mas reciente.
	filename := html.ReportFilePrefix + report.GeneratedAt.Format(html.ReportTimestampLayout) + ".html"
	path, err := html.Render(report, cfg.Output.HTMLDir, filename)
	if err != nil {
		logger.Printf("generando reporte HTML: %v", err)
		return 1
	}
	if _, err := html.Render(report, cfg.Output.HTMLDir, html.LatestReportFilename); err != nil {
		logger.Printf("AVISO: no se pudo actualizar %s: %v", html.LatestReportFilename, err)
	}

	logger.Printf("Reporte generado en: %s (y %s)", path, html.LatestReportFilename)

	html.PruneOldReports(cfg.Output.HTMLDir, report.GeneratedAt, cfg.Output.RetentionDays, logger)

	logger.Println("=== Fin del pipeline ===")

	if len(report.FailedFetches) > 0 {
		return 1
	}
	return 0
}
