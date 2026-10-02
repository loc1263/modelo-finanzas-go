# Proyecto.



> Estado: implementado. Para como correr el pipeline y el detalle del
> reporte, ver [`README.md`](README.md). Para la descripcion de cada
> modelo, ver [`Modelos_de_Momentum.md`](Modelos_de_Momentum.md). Para
> convenciones de desarrollo y arquitectura, ver [`CLAUDE.md`](CLAUDE.md).



## Análisis ETFs de Vanguard.



Bajar la data necesaria para aplicar los modelos 1 y 2 con todos los ETFs de Vanguard.



## Analisis acciones Mega-Cap



Bajar la data necesaria para aplicar los modelos 1 al 11 con todas las acciones mega-cap de EEUU.



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

Ver [`Modelos_de_Momentum.md`](Modelos_de_Momentum.md) para el detalle de cada uno (que mide, formula, y como esta implementado).



## Desarrollo



1. Implementado 100% en Go (sin Python, sin pandas): el pipeline, la descarga de datos, el cálculo de los modelos y la generación del reporte son todo código Go. La persistencia es SQLite, con un driver 100% Go (`modernc.org/sqlite`, sin cgo).
2. Separado en paquetes por responsabilidad (datos de Yahoo, datos de Fama-French, storage, un paquete por modelo, orquestación del pipeline, reporte HTML) para que sea fácil de entender, mantener y escalar.
3. El pipeline corre como un único binario Go (`cmd/pipeline`), sin un orquestador externo en otro lenguaje.
4. El resultado final es un reporte en HTML (no se genera PDF — decisión explícita, para no cargar con una librería extra).
5. Los precios se descargan con un cliente HTTP propio contra el endpoint público de Yahoo Finance (el mismo que usa `yfinance` de Python por debajo), en vez de depender de una librería de terceros para Go (la que existía, `piquette/finance-go`, estaba rota).
6. El pipeline es reejecutable: correrlo varias veces el mismo día no duplica precios ni resultados (upsert por fecha), y hay una opción para forzar una recarga limpia del día en curso si hace falta.
7. Los datos de earnings (modelo 8) también se bajan de Yahoo, pero de un endpoint distinto al de precios que exige una sesión autenticada (cookie + "crumb") en vez de ser totalmente público. Es el mismo mecanismo que usa `yfinance` por debajo; no está documentado oficialmente por Yahoo, así que es más frágil que el resto del pipeline — ver `Modelos_de_Momentum.md` y `CLAUDE.md` para el detalle.



## Consideraciones



1. Se deben bajar todos los datos necesarios usando la api gratis de yahoo.
2. Se debe hacer pausas para no bloquear la API gratis de yahoo (configurable).
3. Se debe guardar un log de cada paso para revision en caso de caidas.
