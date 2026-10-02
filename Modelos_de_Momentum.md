# Modelos de Momentum

Detalle conceptual de los 11 modelos de momentum y de cómo se implementó
cada uno en este proyecto (paquete Go, fórmula exacta y supuestos), más la
simulación de Markowitz (construcción de cartera, no es un modelo de
momentum). Para el alcance general del proyecto ver
[`Descripcion_Proyecto.md`](Descripcion_Proyecto.md); para cómo correr el
pipeline, [`README.md`](README.md).

Se aplican los modelos 1 y 2 a los ETFs de Vanguard, y los modelos 1 al 11
a las acciones mega-cap de EEUU. Todos los modelos de momentum siguen la
misma convención: nunca miran el mes calendario en curso (incompleto),
solo el último mes completo anterior a la fecha de la corrida — ver
`internal/models/common/returns.go`.



## 1. Momentum 12-1 clásico

**Jegadeesh y Titman (1993).** Paquete: `internal/models/momentum12_1`.

Ordena por el retorno de los últimos 12 meses, sin contar el último. Es la
referencia: sirve para medir si los modelos más complejos realmente aportan
algo.

**Implementación:** retorno acumulado entre el cierre de hace 13 meses y el
cierre del último mes completo (`common.TrailingReturn(closes, 12)`).



## 2. Máximo de 52 semanas

**George y Hwang (2004).** Paquete: `internal/models/max52w`.

Ordena por precio actual ÷ máximo del último año. Se usa como filtro
secundario, para descartar acciones que se alejaron mucho de su máximo y
cuya tendencia podría estar rota.

**Implementación:** cierre ajustado más reciente dividido por el máximo
cierre ajustado de las últimas 252 barras diarias (`max52w_lookback_days`
en `configs/settings.yaml`).



## 3. Momentum ajustado por volatilidad (método MSCI)

Paquete: `internal/models/voladjusted`.

Divide el retorno de 12-1 y de 6-1 meses por la volatilidad de cada acción,
y promedia ambos. Modelo base recomendado: es simple, lo usa un índice real
y evita que el ranking lo dominen las acciones más volátiles.

**Implementación:** `((R12-1 / vol) + (R6-1 / vol)) / 2`, donde `vol` es la
desviación estándar anualizada de los retornos diarios de los últimos 252
días (`voladjusted_vol_lookback_days`).

**Simplificación:** el índice MSCI real usa 3 años de retornos semanales
para la volatilidad; acá se usa 1 año de retornos diarios, una aproximación
razonable y ajustable por configuración.



## 4. Momentum residual

**Blitz, Huij y Martens (2011).** Paquete: `internal/models/residualmomentum`.

Ordena por la parte del retorno que no explican los factores comunes del
mercado. Tiene menos colapsos y menos concentración en un solo tema que el
momentum clásico. Es el que usa los factores de Fama-French.

### Los factores de Fama-French

Modelo de 3 factores (1993):

- **Mkt-RF** (mercado): retorno del mercado de EE.UU. menos la tasa libre
  de riesgo. Captura el movimiento general.
- **SMB** (pequeñas menos grandes): retorno de las empresas pequeñas menos
  el de las grandes. Captura el "efecto tamaño".
- **HML** (valor menos crecimiento): retorno de las empresas baratas, con
  alto valor libro sobre precio, menos el de las caras. Captura el "efecto
  valor".

Modelo de 5 factores (2015) agrega dos (**RMW**, rentables menos débiles;
**CMA**, conservadoras menos agresivas). En la base de datos también está
**Mom**, el factor de momentum de Carhart (1997). El pipeline descarga los
tres datasets para referencia, pero el momentum residual solo usa el de 3
factores.

### Por qué se necesitan para el momentum residual

Para cada acción se hace una regresión de sus retornos mensuales de los
últimos 36 meses:

```
rᵢ − rf = α + β·(Mkt−RF) + s·SMB + h·HML + ε
```

β, s, h dicen cuánto se expone la acción a cada factor. NVIDIA, por
ejemplo, tendría β alto y h negativo, porque es una acción de crecimiento.
ε (el residuo) es lo que queda: el retorno que no explican ni el mercado,
ni el tamaño, ni el estilo.

El momentum residual rankea por la suma de esos residuos en 12-1 meses,
dividida por su desviación estándar. Así compara la fuerza propia de cada
empresa, no la ola que la está empujando. Cuando la ola cambia de
dirección, como en los colapsos de momentum, el ranking no se da vuelta de
golpe.

Una aclaración: en mega-caps, el factor SMB aporta poco, porque todas son
grandes. Muchos practicantes usan solo mercado y valor, o reemplazan los
factores por el S&P 500 y el ETF del sector de cada acción. En esta
implementación se usa el modelo de 3 factores completo (Mkt-RF, SMB, HML).

### Cómo se obtienen los datos

Están en la Kenneth R. French Data Library (sitio de Dartmouth), en
archivos zip con un CSV de frecuencia mensual (entre otras). El pipeline
(`internal/famafrench`) descarga el zip correspondiente, lo descomprime en
memoria, y parsea el CSV quedándose solo con la sección mensual (hay otra
sección anual más abajo en el mismo archivo, que se ignora). Los valores
vienen en porcentaje y se dividen por 100 al guardarlos.

Datasets: `F-F_Research_Data_Factors` (3 factores, el que usa este
modelo), `F-F_Research_Data_5_Factors_2x3` (5 factores) y
`F-F_Momentum_Factor` (Mom/Carhart).

### Implementación

Ventana de regresión OLS de 36 meses (`residual_regression_window_months`)
terminando en el último mes completo; score = suma de los últimos 12
residuos de esa ventana (`residual_momentum_months`) dividida por su
desviación estándar.



## 5. Escalado por volatilidad

**Barroso y Santa-Clara (2015).** Paquete: `internal/models/volscaling`.

No elige acciones: ajusta cuánto invertir en la estrategia. Si la cartera
estuvo muy volátil, reduce la exposición. Capa de protección contra
colapsos, combinable con cualquiera de los anteriores.

**Implementación:** se calcula la volatilidad realizada anualizada de una
cartera equal-weight con todas las mega-caps del universo, sobre los
últimos 126 días (`vol_scaling_lookback_days`), y se compara contra un
objetivo anual (`vol_scaling_target_vol_annual`, 12% por defecto). El
factor de exposición = objetivo / realizada, **topeado en 100%**.

**Simplificación deliberada:** el paper original permite apalancar (más de
100%) cuando la volatilidad cae por debajo del objetivo. Acá se limita a
reducir exposición como máximo, nunca a aumentarla, para mantener el
modelo conservador y no implicar operar con apalancamiento.



## 6. Momentum Acelerado / Doble Velocidad

**ap Gwilym, Clare, O'Sullivan y Walmsley (2010).** Paquete:
`internal/models/accelmomentum`.

Compara el retorno reciente (corto plazo) contra el retorno "esperado" para
ese mismo plazo si la tendencia de los últimos 12 meses se hubiera
mantenido constante. Un valor positivo indica que el momentum se está
acelerando (el tramo reciente rinde más que el promedio del tramo largo);
uno negativo indica desaceleración, aun si el retorno total todavía es
positivo.

**Implementación:** `R_corto − R_largo × (meses_corto / meses_largo)`, con
`meses_corto = 3` (`accel_short_months`) y `meses_largo = 12`
(`lookback_months_long`). Por ejemplo, si el retorno de 12 meses fue 40%,
el retorno "esperado" de un tramo de 3 meses al mismo ritmo es 10%; si el
retorno real de los últimos 3 meses fue 18%, el score es +8% (acelerando).



## 7. Momentum Absoluto / Time-Series Momentum (TSMOM)

**Moskowitz, Ooi y Pedersen (2012).** Paquete:
`internal/models/absolutemomentum`.

A diferencia del momentum 12-1 clásico (que es *relativo*: compara una
acción contra las demás del universo), esta familia de modelos es
*absoluta*: compara el retorno propio de la acción contra un umbral, no
contra sus pares.

**Implementación:** se usa como umbral la tasa libre de riesgo compuesta en
la misma ventana de 12 meses (factor RF de Fama-French, ya descargado para
el modelo 4): `score = R12 meses − RF compuesta de 12 meses`.

**Simplificación:** el paper original arma una cartera long/short con el
signo del retorno pasado, escalada por volatilidad, por instrumento. Como
este proyecto produce un ranking cruzado de acciones (no una cartera
apalancada), se usa el exceso de retorno sobre la tasa libre de riesgo como
score: mide lo mismo que el signo/umbral del paper (¿la acción le ganó a
"no hacer nada"?) de forma comparable entre acciones.



## 8. Momentum de Fundamentos / Earnings Momentum

**Chan, Jegadeesh y Lakonishok (1996).** Paquete:
`internal/models/earningsmomentum`.

Ordena por la sorpresa de ganancias (earnings): las acciones cuyas
ganancias reportadas vienen sorprendiendo al alza a los analistas tienden a
seguir haciéndolo por un tiempo (post-earnings-announcement drift).

**De dónde salen los datos:** el endpoint de precios que usa el resto del
proyecto (`v8/finance/chart`) no tiene earnings ni estimados. El que sí los
tiene es `v10/finance/quoteSummary` (módulo `earningsHistory`), pero a
diferencia de los precios, **exige una sesión autenticada**: primero hay
que pedir una cookie de sesión a `fc.yahoo.com`, y con esa cookie pedir un
token "crumb" a `query1.finance.yahoo.com/v1/test/getcrumb`, que después se
manda como parámetro en cada request a `quoteSummary`. Es el mismo
mecanismo que usa `yfinance` (Python) por debajo. Se verificó a mano con
`curl` antes de implementarlo (el endpoint devuelve
`{"error":{"code":"Unauthorized","description":"Invalid Crumb"}}` si se lo
llama directo, sin el paso de la cookie+crumb primero) — está implementado
en `internal/yahoo/earnings.go`, con el mismo cliente HTTP (y su cookie
jar) que usa `internal/yahoo/client.go` para los precios.

**Importante — esto es más frágil que el resto del pipeline:** Yahoo no
documenta oficialmente este mecanismo, así que podría cambiarlo sin aviso
en cualquier momento (es la misma clase de riesgo que hizo que se
descartara `piquette/finance-go`, ver `CLAUDE.md`). Si en algún momento
este modelo empieza a fallar para todos los tickers con un error de
"crumb" o "cookie" inválido, revisar primero con `curl` si Yahoo cambió el
flujo, antes de asumir que es un bug del código.

**Implementación:** versión simplificada de las "Standardized Unexpected
Earnings" (SUE) académicas. En vez de la sorpresa en dólares de EPS
dividida por el precio o por la dispersión de estimados, se usa
directamente el `surprisePercent` que reporta Yahoo (sorpresa como % del
EPS estimado) en los últimos trimestres disponibles (típicamente 4): el
score es el promedio de esas sorpresas, dividido por su desviación
estándar cuando hay suficiente variación (mismo patrón que el modelo 4,
momentum residual). Los datos se guardan en la tabla `earnings` de SQLite
(símbolo + trimestre + EPS real/estimado/sorpresa), igual que los precios y
los factores de Fama-French — a diferencia de los precios, siempre se
refrescan en cada corrida sin `-skip-fetch` (son pocos requests, no hace
falta cachear por día).



## 9. Momentum Consecutivo / Frog-in-the-Pan (FITP)

**Da, Peng y Engelberg; Da, Gurun y Warachka (2014).** Paquete:
`internal/models/frogpan`.

La idea es que el mercado reacciona más lento (y el momentum dura más)
cuando la información llega de forma continua y gradual, en vez de en
pocos saltos grandes ("lumpy"). El nombre viene de la metáfora de la rana
que no nota que el agua se está calentando de a poco.

**Implementación:** "Information Discreteness" (ID) de
Da-Gurun-Warachka: `ID = signo(retorno) × (%días negativos − %días
positivos)` sobre la ventana de formación de 12 meses (la misma que usa el
modelo 1). El score reportado es `-ID`: más alto es mejor (tendencia más
pareja y continua, con momentum más persistente según el paper).



## 10. Momentum Secundario / Sectorial

**Moskowitz y Grinblatt (1999).** Paquete: `internal/models/sectormomentum`.

La idea original es que el momentum de una industria/sector es más fuerte
y persistente que el de una acción individual.

**Implementación:** en vez de armar un "sector" sintético promediando las
pocas mega-caps de este universo que comparten rubro (una muestra chica y
sesgada), se reusa el ETF sectorial de Vanguard correspondiente (ya
descargado para el análisis de ETFs) como proxy del sector completo, y se
le asigna a cada acción el momentum 12-1 de **su** sector. El mapeo
acción → ETF sectorial está en la columna `sector_etf` de
`configs/megacap_stocks.csv`. Dos acciones del mismo sector comparten el
mismo score: este modelo elige sectores, no acciones dentro de un sector
(se ve en el reporte como barras empatadas en el mini-gráfico).



## 11. Momentum con Liquidez

**Amihud (2002).** Paquete: `internal/models/liquidityadjusted`.

Amihud propuso un ratio de iliquidez simple: cuánto se mueve el precio por
cada dólar operado. Acciones más ilíquidas (difíciles de comprar o vender
sin mover el precio) tienden a cargar un "premio" de retorno, pero también
son más caras y riesgosas de operar en la práctica.

**Implementación:** `ILLIQ = promedio(|retorno diario| / volumen en
dólares)` sobre los últimos 252 días. Se calcula el momentum 12-1 y el
ILLIQ de cada acción, se ordenan las acciones por ILLIQ (de más líquida a
menos), y se arma un score de liquidez en `(0, 1]` (1 = la más líquida del
universo). El score final es `momentum × score_de_liquidez`: a igual
momentum, se prefiere la acción más fácil de operar.

**Simplificación:** el modelo también se referencia junto a Asness,
Frazzini y Pedersen (2013, "Quality Minus Junk"), pero QMJ necesita datos
fundamentales (rentabilidad, crecimiento, pagos de dividendos) que no están
disponibles vía la API gratuita de Yahoo usada en este proyecto. Por eso
el modelo implementado usa solo el componente de liquidez de Amihud, no el
de calidad de AFP.



## Construcción de cartera: simulación de Markowitz

**Markowitz (1952), "Portfolio Selection", Journal of Finance.** Paquete:
`internal/models/markowitz`.

A diferencia de los 11 modelos anteriores, esto no es un modelo de
momentum: no rankea acciones por ninguna señal de tendencia, construye una
cartera. Por eso se muestra en el reporte como una sección aparte
("Construcción de cartera"), no numerada junto a los modelos 1-11.

La idea central de Markowitz es que diversificar entre activos con
retornos poco correlacionados reduce el riesgo de la cartera sin
sacrificar retorno — el objetivo es encontrar la cartera que maximiza el
retorno por unidad de riesgo asumido (el Sharpe ratio).

**Implementación:** en vez de resolver el problema de optimización
cuadrática de forma analítica (invirtiendo la matriz de covarianza), se
simulan `models.markowitz_simulations` carteras aleatorias long-only (sin
apalancamiento, pesos en `[0,1]` que suman 1, muestreados uniformemente
sobre el símplex) sobre los retornos diarios de los últimos
`models.markowitz_lookback_days` días, y se evalúa cada una:

```
retorno anual = pesos · retorno medio anualizado de cada activo
volatilidad anual = sqrt(pesos · matriz de covarianza anualizada · pesos)
Sharpe = (retorno anual − RF anual) / volatilidad anual
```

La tasa libre de riesgo (RF) es la misma que usan los modelos 4 y 7
(factor RF de Fama-French), compuesta sobre los últimos 12 meses
disponibles. La cartera de mayor Sharpe ratio encontrada es el resultado;
sus pesos se muestran como un ranking (mayor peso primero) para reusar el
mismo formato de tabla que los modelos de momentum.

Se corren dos variantes, ambas sobre el universo mega-cap:

- **Top momentum:** sobre las `models.markowitz_top_momentum_n` acciones
  de mejor ranking según el modelo 1 (momentum 12-1 clásico) — conecta la
  construcción de cartera con el resto del pipeline en vez de ser una
  herramienta aislada.
- **Universo completo:** sobre todas las acciones mega-cap con historia
  suficiente, sin ningún filtro previo.

**Reproducibilidad:** al ser una simulación aleatoria, el generador de
números aleatorios se siembra de forma determinística a partir del día de
la corrida (`AsOf`, no la hora exacta): reejecutar el pipeline el mismo
día encuentra siempre la misma cartera óptima, en vez de una distinta en
cada corrida — igual que el resto del pipeline, que garantiza resultados
estables al reejecutarse el mismo día (ver `CLAUDE.md`, sección
"Idempotencia").

**Simplificación:** el método clásico de Markowitz resuelve la frontera
eficiente de forma analítica (optimización cuadrática); acá se aproxima
por simulación Monte Carlo, el enfoque más común en la práctica para
explorar la frontera eficiente sin depender de un solver externo. Tampoco
se permite apalancamiento ni posiciones en corto (long-only), siguiendo la
misma postura conservadora que el modelo 5 (escalado por volatilidad).
