# Proyecto.



> Estado: implementado. Para como correr el pipeline y el detalle del
> reporte, ver [`README.md`](README.md). Para convenciones de desarrollo y
> arquitectura, ver [`CLAUDE.md`](CLAUDE.md). Este documento describe el
> alcance conceptual del proyecto: que se analiza y con que modelos.



## Análisis ETFs de Vanguard.



Bajar la data necesaria para aplicar los modelos 1 y 2 con todos los ETFs de Vanguard.



## Analisis acciones Mega-Cap



Bajar la data necesaria para aplicar los modelos 1 al 5 con todas las acciones mega-cap de EEUU.



## Los modelos



1. Momentum 12-1 clásico (Jegadeesh y Titman, 1993)

Ordena por el retorno de los últimos 12 meses, sin contar el último. Es la referencia: sirve para medir si los modelos más complejos realmente aportan algo.



2. Máximo de 52 semanas (George y Hwang, 2004)

Ordena por precio actual ÷ máximo del último año. Lo usaría como filtro secundario, para descartar acciones que se alejaron mucho de su máximo y cuya tendencia podría estar rota.



3. Momentum ajustado por volatilidad (método MSCI)

Divide el retorno de 12-1 y de 6-1 meses por la volatilidad de cada acción, y promedia ambos. Modelo base recomendado: es simple, lo usa un índice real y evita que el ranking lo dominen las acciones más volátiles.



4. Momentum residual (Blitz, Huij y Martens, 2011)

Ordena por la parte del retorno que no explican los factores comunes del mercado. Mejora recomendada: tiene menos colapsos y menos concentración en un solo tema. Es el que usa los factores de French.



5. Escalado por volatilidad (Barroso y Santa-Clara, 2015)

No elige acciones: ajusta cuánto inviertes en la estrategia. Si la cartera estuvo muy volátil, reduces la exposición. Capa de protección contra colapsos, combinable con cualquiera de los anteriores.



## Desarrollo



1. Implementado 100% en Go (sin Python, sin pandas): el pipeline, la descarga de datos, el cálculo de los 5 modelos y la generación del reporte son todo código Go. La persistencia es SQLite, con un driver 100% Go (`modernc.org/sqlite`, sin cgo).
2. Separado en paquetes por responsabilidad (datos de Yahoo, datos de Fama-French, storage, un paquete por modelo, orquestación del pipeline, reporte HTML) para que sea fácil de entender, mantener y escalar.
3. El pipeline corre como un único binario Go (`cmd/pipeline`), sin un orquestador externo en otro lenguaje.
4. El resultado final es un reporte en HTML (no se genera PDF — decisión explícita, para no cargar con una librería extra).
5. Los precios se descargan con un cliente HTTP propio contra el endpoint público de Yahoo Finance (el mismo que usa `yfinance` de Python por debajo), en vez de depender de una librería de terceros para Go (la que existía, `piquette/finance-go`, estaba rota).
6. El pipeline es reejecutable: correrlo varias veces el mismo día no duplica precios ni resultados (upsert por fecha), y hay una opción para forzar una recarga limpia del día en curso si hace falta.



## Consideraciones



1. Se deben bajar todos los datos necesarios usando la api gratis de yahoo.
2. Se debe hacer pausas para no bloquear la API gratis de yahoo (configurable).
3. Se debe guardar un log de cada paso para revision en caso de caidas.



## Los factores de Fama-French



Modelo de 3 factores (1993):



Mkt-RF (mercado): retorno del mercado de EE.UU. menos la tasa libre de riesgo. Captura el movimiento general.

SMB (pequeñas menos grandes): retorno de las empresas pequeñas menos el de las grandes. Captura el "efecto tamaño".

HML (valor menos crecimiento): retorno de las empresas baratas, con alto valor libro sobre precio, menos el de las caras. Captura el "efecto valor".



Modelo de 5 factores (2015) agrega dos:



RMW (rentables menos débiles): empresas con alta rentabilidad operacional menos las de baja rentabilidad.

CMA (conservadoras menos agresivas): empresas que invierten poco en expandirse menos las que invierten mucho.



En su base de datos también está Mom, el factor de momentum: ganadores menos perdedores, basado en el trabajo de Carhart (1997).



Por qué se necesitan para el momentum residual



Para cada acción se hace una regresión de sus retornos mensuales de los últimos 36 meses:



rᵢ − rf = α + β·(Mkt−RF) + s·SMB + h·HML + ε



β, s, h dicen cuánto se expone la acción a cada factor. NVIDIA, por ejemplo, tendría β alto y h negativo, porque es una acción de crecimiento.

ε (el residuo) es lo que queda: el retorno que no explican ni el mercado, ni el tamaño, ni el estilo.



El momentum residual rankea por la suma de esos residuos en 12-1 meses, dividida por su desviación estándar. Así compara la fuerza propia de cada empresa, no la ola que la está empujando. Cuando la ola cambia de dirección, como en los colapsos de momentum, el ranking no se da vuelta de golpe.



Una aclaración: en mega-caps, el factor SMB aporta poco, porque todas son grandes. Muchos practicantes usan solo mercado y valor, o reemplazan los factores por el S\&P 500 y el ETF del sector de cada acción. En esta implementación se usa el modelo de 3 factores completo (Mkt-RF, SMB, HML), tal como está descrito arriba.



### Cómo se obtienen los datos



Están en la Kenneth R. French Data Library (sitio de Dartmouth), en archivos zip con un CSV de frecuencia mensual (entre otras). El pipeline (`internal/famafrench`) descarga el zip correspondiente, lo descomprime en memoria, y parsea el CSV quedándose solo con la sección mensual (hay otra sección anual más abajo en el mismo archivo, que se ignora). Los valores vienen en porcentaje y se dividen por 100 al guardarlos.

Datasets usados: `F-F_Research_Data_Factors` (3 factores, el que se usa para el momentum residual), `F-F_Research_Data_5_Factors_2x3` (5 factores) y `F-F_Momentum_Factor` (Mom/Carhart) — estos dos últimos se descargan y guardan para referencia, aunque el momentum residual solo usa el de 3 factores.
