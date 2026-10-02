// Package liquidityadjusted implementa el modelo 11: Momentum con Liquidez
// (Amihud, 2002).
//
// Amihud propuso un ratio de iliquidez simple: cuanto se mueve el precio
// por cada dolar operado. Acciones mas iliquidas (dificiles de comprar o
// vender sin mover el precio) tienden a cargar un "premio" de retorno, pero
// tambien son mas caras y riesgosas de operar en la practica. Este modelo
// pondera el momentum 12-1 de cada accion por que tan liquida es
// relativo al resto del universo: a igual momentum, se prefiere la accion
// mas facil de operar.
//
// Simplificacion: el trabajo de Asness, Frazzini y Pedersen (2013) sobre
// "Quality Minus Junk" tambien se menciona como referencia de este modelo,
// pero requiere datos fundamentales (rentabilidad, crecimiento, pagos de
// dividendos) que no estan disponibles via la API gratuita de Yahoo usada
// en este proyecto. Por eso el modelo implementado usa solo el componente
// de liquidez de Amihud, no el de calidad de AFP.
package liquidityadjusted

import (
	"fmt"
	"math"

	"modelofinanzas/internal/storage/sqlite"
)

// Illiquidity calcula el ratio de iliquidez de Amihud: el promedio de
// |retorno diario| dividido por el volumen en dolares de ese dia, sobre la
// ventana dada. Valores mas altos significan que la accion es mas dificil
// de operar sin mover el precio.
func Illiquidity(bars []sqlite.PriceBar) (float64, error) {
	if len(bars) < 2 {
		return 0, fmt.Errorf("se necesitan al menos 2 dias de precios")
	}

	sum := 0.0
	n := 0
	for i := 1; i < len(bars); i++ {
		dollarVolume := bars[i].Close * float64(bars[i].Volume)
		if dollarVolume <= 0 || bars[i-1].AdjClose == 0 {
			continue
		}
		ret := bars[i].AdjClose/bars[i-1].AdjClose - 1
		sum += math.Abs(ret) / dollarVolume
		n++
	}

	if n == 0 {
		return 0, fmt.Errorf("sin datos validos de volumen en la ventana")
	}

	return sum / float64(n), nil
}

// CombineWithMomentum resta al momentum una penalidad proporcional a la
// iliquidez (1 - liquidityScore, donde liquidityScore esta en (0,1] y 1 es
// la accion mas liquida del universo comparado), escalada por penaltyScale.
//
// Se resta en vez de multiplicar a proposito: multiplicar por un numero en
// (0,1] acerca cualquier valor a cero, lo que MEJORA (acerca a cero) el
// score de una accion con momentum negativo a medida que es mas iliquida —
// exactamente al reves de lo que el modelo quiere decir. Restar una
// penalidad siempre empeora el score, sin importar el signo del momentum.
//
// penaltyScale fija cuanto pesa la iliquidez frente al momentum; se
// recomienda el desvio estandar del momentum del universo comparado (ver
// pipeline.computeLiquidityAdjustedMomentum), para que la penalidad sea de
// una magnitud comparable a la señal en vez de una constante arbitraria.
func CombineWithMomentum(momentum, liquidityScore, penaltyScale float64) float64 {
	illiquidityPenalty := (1 - liquidityScore) * penaltyScale
	return momentum - illiquidityPenalty
}
