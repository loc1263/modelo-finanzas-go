# Descripción del Proyecto

> Estado: implementado. Para cómo correr el pipeline y el detalle del
> reporte, ver [`README.md`](README.md). Para la descripción de cada
> modelo, ver [`Modelos_de_Momentum.md`](Modelos_de_Momentum.md). Para
> convenciones de desarrollo y arquitectura, ver [`CLAUDE.md`](CLAUDE.md).

## Análisis de ETFs de Vanguard

Bajar la data necesaria para aplicar los modelos 1 y 2 a todos los ETFs de
Vanguard.

## Análisis de acciones mega-cap

Bajar la data necesaria para aplicar los modelos 1 al 11 a todas las
acciones mega-cap de EE.UU.

## Los modelos

1. Momentum 12-1 clásico — Jegadeesh y Titman (1993)
2. Máximo de 52 semanas — George y Hwang (2004)
3. Momentum ajustado por volatilidad — método MSCI
4. Momentum residual — Blitz, Huij y Martens (2011)
5. Escalado por volatilidad — Barroso y Santa-Clara (2015)
6. Momentum Acelerado / Doble Velocidad — ap Gwilym, Clare, O'Sullivan y Walmsley (2010)
7. Momentum Absoluto / Time-Series Momentum (TSMOM) — Moskowitz, Ooi y Pedersen (2012)
8. Momentum de Fundamentos / Earnings Momentum — Chan, Jegadeesh y Lakonishok (1996)
9. Momentum Consecutivo / Frog-in-the-Pan (FITP) — Da, Peng y Engelberg / Da, Gurun y Warachka (2014)
10. Momentum Secundario / Sectorial — Moskowitz y Grinblatt (1999)
11. Momentum con Liquidez — Amihud (2002)

Ver [`Modelos_de_Momentum.md`](Modelos_de_Momentum.md) para el detalle de
cada uno (qué mide, fórmula, y cómo está implementado).

## Construcción de cartera

Además de los 11 modelos de momentum, el reporte incluye una simulación de
Markowitz (1952) sobre las acciones mega-cap: no rankea acciones, construye
una cartera. Se muestra en una sección aparte del reporte, no numerada
junto a los modelos de momentum — ver
[`Modelos_de_Momentum.md`](Modelos_de_Momentum.md), sección "Construcción
de cartera: simulación de Markowitz".

## Desarrollo

1. Implementado 100% en Go (sin Python, sin pandas): el pipeline, la descarga de datos, el cálculo de los modelos y la generación del reporte son todo código Go. La persistencia es SQLite, con un driver 100% Go (`modernc.org/sqlite`, sin cgo).
2. Separado en paquetes por responsabilidad (datos de Yahoo, datos de Fama-French, storage, un paquete por modelo, orquestación del pipeline, reporte HTML) para que sea fácil de entender, mantener y escalar.
3. El pipeline corre como un único binario Go (`cmd/pipeline`), sin un orquestador externo en otro lenguaje.
4. El resultado final es un reporte en HTML (no se genera PDF — decisión explícita, para no cargar con una librería extra).
5. Los precios se descargan con un cliente HTTP propio contra el endpoint público de Yahoo Finance (el mismo que usa `yfinance` de Python por debajo), en vez de depender de una librería de terceros para Go (la que existía, `piquette/finance-go`, estaba rota).
6. El pipeline es reejecutable: correrlo varias veces el mismo día no duplica precios ni resultados (upsert por fecha), y hay una opción para forzar una recarga limpia del día en curso si hace falta.
7. Los datos de earnings (modelo 8) también se bajan de Yahoo, pero de un endpoint distinto al de precios que exige una sesión autenticada (cookie + "crumb") en vez de ser totalmente público. Es el mismo mecanismo que usa `yfinance` por debajo; no está documentado oficialmente por Yahoo, así que es más frágil que el resto del pipeline — ver `Modelos_de_Momentum.md` y `CLAUDE.md` para el detalle.

## Requisitos del proyecto

1. Descargar todos los datos necesarios usando la API gratuita de Yahoo Finance.
2. Respetar una pausa configurable entre requests, para no exceder el límite de la API gratuita de Yahoo.
3. Guardar un log de cada corrida, para poder revisar fallas si el pipeline se cae a mitad de camino.
