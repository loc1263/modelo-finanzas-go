# Modelo Finanzas Go

Pipeline 100% Go que descarga datos de la API gratuita de Yahoo Finance y de
la Kenneth R. French Data Library, y aplica modelos de momentum sobre:

- **ETFs de Vanguard** (modelos 1 y 2)
- **Acciones mega-cap de EEUU** (modelos 1 a 11)

El resultado es un reporte HTML con el ranking de cada modelo.

Ver [`Descripcion_Proyecto.md`](Descripcion_Proyecto.md) para el alcance
general del proyecto, y [`Modelos_de_Momentum.md`](Modelos_de_Momentum.md)
para el detalle conceptual de cada modelo (que mide, formula, y como esta
implementado) y de los factores de Fama-French.

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
internal/models/        Los modelos de momentum (uno por paquete) + helpers matematicos (common/)
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
2. Descarga el historial de earnings (EPS real vs. estimado) de las
   mega-caps, para el modelo 8.
3. Descarga los factores mensuales de Fama-French (3 factores, 5 factores y
   momentum) desde la Kenneth French Data Library.
4. Calcula los modelos 1 y 2 para los ETFs, y 1 a 11 para las mega-caps.
5. Guarda todo en `data/modelo_finanzas.db` (SQLite).
6. Genera `output/reporte_<timestamp>.html` y actualiza
   `output/reporte_ultimo.html` (siempre apunta a la corrida mas reciente).
   Cada modelo muestra el autor/paper entre parentesis en el titulo, un
   icono "?" que al pasar el mouse o hacer click despliega un resumen de que
   hace el modelo, un mini-grafico de barras con el top 5 (clickeable, con
   tooltip del nombre de la empresa/fondo al pasar el mouse), y la tabla
   completa paginada (10 tickers por pagina) debajo. Cada ticker, en el
   grafico y en la tabla, es un link directo a su pagina en Yahoo Finance.
   Arriba a la derecha hay un toggle de modo claro/oscuro (oscuro por
   defecto), que recuerda la eleccion en el navegador.
7. Escribe el detalle de cada paso en `logs/pipeline.log`.

Una corrida completa de todo el universo puede tardar bastante por las pausas
entre requests (ver `yahoo.request_pause_ms` en la config) — es esperado y
necesario para no ser bloqueado por Yahoo. Para acelerarla sin perder esa
pausa por request, hay `yahoo.concurrency` (default `2`): descarga varios
tickers en simultaneo, pero cada uno igual espera su propia pausa completa
antes de pedir el suyo — no se reparte la pausa entre ellos. Subir
`concurrency` acerca el ritmo total de requests a Yahoo a
`concurrency / request_pause_ms`; mantenerlo chico (2-3) para no arriesgar
que Yahoo empiece a bloquear o devolver errores 429. `concurrency: 1`
vuelve al comportamiento 100% secuencial.

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
- **Earnings** (`earnings`, modelo 8): clave `(symbol, quarter)`, tambien
  UPSERT. A diferencia de precios y factores, se vuelve a descargar en cada
  corrida sin `-skip-fetch` (son pocos requests, no hace falta cachear por dia).
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
  (capitalizacion > ~200B USD), con columnas `ticker,name,sector_etf`. Las
  capitalizaciones cambian con el tiempo; conviene revisar esta lista
  periodicamente. Los tickers deben estar en formato Yahoo Finance (ej.
  Berkshire Hathaway clase B = `BRK-B`). La columna `sector_etf` es el
  ticker del ETF sectorial de Vanguard (de `vanguard_etfs.csv`) que
  representa el rubro de esa accion, usado por el modelo 10 (momentum
  sectorial) — si se agrega una accion nueva, hay que asignarle un
  `sector_etf` para que ese modelo la incluya.
- Lineas que empiezan con `#` se ignoran (comentarios).

## Supuestos y simplificaciones de los modelos

El detalle de cada modelo (que mide, formula exacta, y que simplificacion
se tomo respecto al paper original) esta en
[`Modelos_de_Momentum.md`](Modelos_de_Momentum.md) y en los comentarios de
cada paquete en `internal/models/`. Dos convenciones transversales:

- **Convencion "12-1" global**: todos los modelos anclan sus calculos al
  ultimo mes calendario **completo** anterior a la fecha de la corrida (nunca
  miran el mes en curso, que esta incompleto).
- **Modelo 8 (earnings momentum)**: usa un endpoint de Yahoo distinto al de
  precios, que exige una sesion autenticada (cookie + "crumb") en vez de
  ser totalmente publico — ver la seccion "Datos y fuentes" mas abajo y
  `Modelos_de_Momentum.md` para el detalle y el riesgo de fragilidad.

## Datos y fuentes

- Precios: `query1.finance.yahoo.com/v8/finance/chart/{symbol}` (la misma API
  que usa `yfinance` de Python por debajo). Cliente propio en
  `internal/yahoo`, con pausas y reintentos configurables para no ser
  bloqueado.
- Factores de Fama-French: Kenneth R. French Data Library (Dartmouth), en
  `internal/famafrench`.
- Earnings (modelo 8): `query2.finance.yahoo.com/v10/finance/quoteSummary/{symbol}`
  (modulo `earningsHistory`). A diferencia de los precios, este endpoint
  exige una sesion autenticada (cookie + "crumb" de Yahoo, el mismo
  mecanismo que usa `yfinance` por debajo) — implementado en
  `internal/yahoo/earnings.go`. No esta documentado oficialmente por Yahoo,
  asi que es mas fragil que el resto del pipeline; ver
  `Modelos_de_Momentum.md` si empieza a fallar.
