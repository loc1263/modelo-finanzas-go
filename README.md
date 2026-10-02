# Modelo Finanzas Go

Pipeline 100% Go que descarga datos de la API gratuita de Yahoo Finance y de
la Kenneth R. French Data Library, y aplica 5 modelos de momentum sobre:

- **ETFs de Vanguard** (modelos 1 y 2)
- **Acciones mega-cap de EEUU** (modelos 1 a 5)

El resultado es un reporte HTML con el ranking de cada modelo.

Ver [`Descripcion_Proyecto.md`](Descripcion_Proyecto.md) para el detalle
conceptual de cada modelo y de los factores de Fama-French.

## Requisitos

- Go 1.26+
- Conexion a internet (Yahoo Finance y la Kenneth French Data Library de Dartmouth)
- No requiere compilador C (el driver de SQLite es 100% Go: `modernc.org/sqlite`)

## Estructura del proyecto

```
cmd/pipeline/           Comando principal (main.go)
internal/yahoo/         Cliente HTTP para la API publica de Yahoo Finance
internal/famafrench/    Descarga y parseo de los datasets de Kenneth French
internal/storage/sqlite Persistencia (precios, factores, resultados)
internal/models/        Los 5 modelos de momentum + helpers matematicos (common/)
internal/pipeline/      Orquestador: descarga -> calculo -> persistencia
internal/report/html/   Generacion del reporte HTML
internal/logging/       Logger a archivo + consola
internal/config/        Carga de configuracion (settings.yaml) y universos (CSV)
configs/                settings.yaml, settings.test.yaml, listas de tickers
data/                   Bases SQLite (gitignored)
output/                 Reportes HTML generados (gitignored)
logs/                   Logs de cada corrida (gitignored)
```

## Como correr el pipeline completo

```
go run ./cmd/pipeline
```

Esto, con la configuracion por defecto (`configs/settings.yaml`):

1. Descarga precios diarios de ~6 anios para todos los tickers de
   `configs/vanguard_etfs.csv` y `configs/megacap_stocks.csv`, con una pausa
   entre requests para no bloquear la API gratuita de Yahoo.
2. Descarga los factores mensuales de Fama-French (3 factores, 5 factores y
   momentum) desde la Kenneth French Data Library.
3. Calcula los modelos 1 y 2 para los ETFs, y 1 a 5 para las mega-caps.
4. Guarda todo en `data/modelo_finanzas.db` (SQLite).
5. Genera `output/reporte_<timestamp>.html` y actualiza
   `output/reporte_ultimo.html` (siempre apunta a la corrida mas reciente).
   Cada modelo muestra el autor/paper entre parentesis en el titulo, un
   icono "?" que al pasar el mouse o hacer click despliega un resumen de que
   hace el modelo, un mini-grafico de barras con el top 5 (clickeable, con
   tooltip del nombre de la empresa/fondo al pasar el mouse), y la tabla
   completa paginada (10 tickers por pagina) debajo. Cada ticker, en el
   grafico y en la tabla, es un link directo a su pagina en Yahoo Finance.
6. Escribe el detalle de cada paso en `logs/pipeline.log`.

Una corrida completa de todo el universo puede tardar bastante por las pausas
entre requests (ver `yahoo.request_pause_ms` en la config) — es esperado y
necesario para no ser bloqueado por Yahoo.

### Flags disponibles

| Flag            | Default                   | Que hace |
|-----------------|---------------------------|----------|
| `-config`       | `configs/settings.yaml`   | Ruta al archivo de configuracion a usar. |
| `-skip-fetch`   | `false`                   | No descarga nada nuevo; recalcula los modelos con lo que ya hay en la base. Util para iterar rapido sobre la logica de los modelos. |
| `-limit`        | `0` (sin limite)          | Si es > 0, usa solo los primeros N tickers de cada universo. Util para pruebas rapidas. |
| `-reset-today`  | `false`                   | Borra los precios y resultados **del dia de hoy** antes de correr, forzando una recorrida 100% limpia de hoy (ver mas abajo). |

Ejemplos:

```
# Recalcular los modelos sin volver a pegarle a la API (ya hay datos de hoy)
go run ./cmd/pipeline -skip-fetch

# Probar rapido con solo 5 tickers por universo
go run ./cmd/pipeline -limit 5

# Forzar una redescarga limpia de hoy (por ejemplo, si una corrida anterior
# quedo con datos parciales o Yahoo devolvio algo raro)
go run ./cmd/pipeline -reset-today
```

### Reejecucion / idempotencia

El pipeline esta pensado para correrse varias veces al dia sin acumular
datos duplicados:

- **Precios** (`prices`): la clave es `(symbol, date)`. Si corres el pipeline
  dos veces el mismo dia, la segunda corrida simplemente actualiza (UPSERT)
  la fila del dia de hoy para cada ticker; no se insertan filas nuevas ni se
  tocan los dias anteriores.
- **Resultados de los modelos** (`model_results`): la clave es
  `(run_date, universe, model, symbol)`, donde `run_date` es el dia (no la
  hora exacta). Reejecutar el pipeline el mismo dia sobreescribe los
  resultados de ese dia en vez de agregar filas nuevas. Los resultados de
  dias anteriores quedan intactos, por lo que la tabla sirve tambien como
  historial dia a dia.
- **Factores de Fama-French** (`factors`): clave `(dataset, date, factor)`,
  tambien UPSERT — no se duplican al reejecutar.
- Si necesitas garantizar un estado 100% limpio del dia (por ejemplo, Yahoo
  devolvio datos incompletos en una corrida anterior), usa `-reset-today`:
  borra los precios y resultados de hoy antes de volver a descargar y
  calcular todo desde cero. Los datos de dias anteriores nunca se tocan.

## Modo de pruebas (sin ensuciar los datos de produccion)

Para desarrollo/pruebas usa `configs/settings.test.yaml`, que apunta a una
base, carpeta de salida y carpeta de logs completamente separadas de las de
produccion:

```
go run ./cmd/pipeline -config configs/settings.test.yaml -limit 5
```

Esto escribe en `data/test/test.db`, `output/test/` y `logs/test/`, todas
ignoradas por git. **Nunca** apuntes una corrida de prueba a
`configs/settings.yaml` (la config de produccion).

## Mantenimiento de los universos de tickers

- `configs/vanguard_etfs.csv`: lista editable de ETFs de Vanguard (columnas
  `ticker,name`). Vanguard no publica una API con el listado completo, asi
  que esta lista se mantiene a mano.
- `configs/megacap_stocks.csv`: lista editable de acciones mega-cap de EEUU
  (capitalizacion > ~200B USD). Las capitalizaciones cambian con el tiempo;
  conviene revisar esta lista periodicamente. Los tickers deben estar en
  formato Yahoo Finance (ej. Berkshire Hathaway clase B = `BRK-B`).
- Lineas que empiezan con `#` se ignoran (comentarios).

## Supuestos y simplificaciones de los modelos

Documentados tambien como comentarios en el codigo de cada paquete en
`internal/models/`:

- **Convencion "12-1" global**: todos los modelos anclan sus calculos al
  ultimo mes calendario **completo** anterior a la fecha de la corrida (nunca
  miran el mes en curso, que esta incompleto). Esto es lo que implementa el
  "sin contar el ultimo mes" de forma consistente en los 5 modelos.
- **Modelo 3 (ajustado por volatilidad, estilo MSCI)**: la volatilidad se
  calcula como la desviacion estandar anualizada de retornos diarios en una
  ventana configurable (`voladjusted_vol_lookback_days`), en vez de 3 anios de
  retornos semanales como el indice MSCI real. Es una aproximacion razonable
  y ajustable via config.
- **Modelo 4 (momentum residual)**: usa el modelo de 3 factores de
  Fama-French (Mkt-RF, SMB, HML), tal como esta descrito en
  `Descripcion_Proyecto.md`. La regresion usa una ventana de 36 meses
  (`residual_regression_window_months`) y el score final es la suma de los
  ultimos 12 residuos (`residual_momentum_months`) dividida por su desviacion
  estandar.
- **Modelo 5 (escalado por volatilidad)**: no rankea acciones — calcula la
  volatilidad realizada de una cartera equal-weight con todas las mega-caps
  del universo y devuelve que fraccion del capital destinado a la estrategia
  conviene exponer. El factor esta **topeado en 100%** (no se usa
  apalancamiento), a diferencia del paper original de Barroso-Santa Clara
  que permite apalancar cuando la volatilidad cae por debajo del objetivo.

## Datos y fuentes

- Precios: `query1.finance.yahoo.com/v8/finance/chart/{symbol}` (la misma API
  que usa `yfinance` de Python por debajo). Cliente propio en
  `internal/yahoo`, con pausas y reintentos configurables para no ser
  bloqueado.
- Factores de Fama-French: Kenneth R. French Data Library (Dartmouth), en
  `internal/famafrench`.
