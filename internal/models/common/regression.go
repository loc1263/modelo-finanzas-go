package common

import "fmt"

// OLSResult es el resultado de una regresion lineal multiple con intercepto.
type OLSResult struct {
	Coefficients []float64 // [intercepto, beta_1, ..., beta_k]
	Residuals    []float64 // mismo largo que y
}

// OLS resuelve y = X*beta + eps por minimos cuadrados, agregando
// automaticamente una columna de intercepto. x[i] tiene los k regresores de
// la observacion i (sin la columna de 1s).
func OLS(y []float64, x [][]float64) (*OLSResult, error) {
	n := len(y)
	if n == 0 || len(x) != n {
		return nil, fmt.Errorf("dimensiones invalidas: y=%d, x=%d", len(y), len(x))
	}
	k := len(x[0]) + 1 // regresores + intercepto
	if n < k {
		return nil, fmt.Errorf("se necesitan al menos %d observaciones, hay %d", k, n)
	}

	// Construye X con columna de intercepto.
	design := make([][]float64, n)
	for i := range x {
		if len(x[i]) != k-1 {
			return nil, fmt.Errorf("fila %d tiene %d regresores, se esperaban %d", i, len(x[i]), k-1)
		}
		row := make([]float64, k)
		row[0] = 1
		copy(row[1:], x[i])
		design[i] = row
	}

	// Normal equations: (X^T X) beta = X^T y
	xtx := make([][]float64, k)
	xty := make([]float64, k)
	for a := 0; a < k; a++ {
		xtx[a] = make([]float64, k)
		for b := 0; b < k; b++ {
			sum := 0.0
			for i := 0; i < n; i++ {
				sum += design[i][a] * design[i][b]
			}
			xtx[a][b] = sum
		}
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += design[i][a] * y[i]
		}
		xty[a] = sum
	}

	beta, err := solveLinearSystem(xtx, xty)
	if err != nil {
		return nil, fmt.Errorf("resolviendo OLS: %w", err)
	}

	residuals := make([]float64, n)
	for i := 0; i < n; i++ {
		pred := 0.0
		for a := 0; a < k; a++ {
			pred += design[i][a] * beta[a]
		}
		residuals[i] = y[i] - pred
	}

	return &OLSResult{Coefficients: beta, Residuals: residuals}, nil
}

// solveLinearSystem resuelve A*x = b con eliminacion gaussiana y pivoteo parcial.
func solveLinearSystem(a [][]float64, b []float64) ([]float64, error) {
	n := len(b)

	// Copia para no mutar el input.
	m := make([][]float64, n)
	for i := range a {
		m[i] = append([]float64{}, a[i]...)
	}
	rhs := append([]float64{}, b...)

	for col := 0; col < n; col++ {
		pivot := col
		for row := col + 1; row < n; row++ {
			if abs(m[row][col]) > abs(m[pivot][col]) {
				pivot = row
			}
		}
		if abs(m[pivot][col]) < 1e-12 {
			return nil, fmt.Errorf("matriz singular (regresores colineales o datos insuficientes)")
		}
		m[col], m[pivot] = m[pivot], m[col]
		rhs[col], rhs[pivot] = rhs[pivot], rhs[col]

		for row := col + 1; row < n; row++ {
			factor := m[row][col] / m[col][col]
			for c := col; c < n; c++ {
				m[row][c] -= factor * m[col][c]
			}
			rhs[row] -= factor * rhs[col]
		}
	}

	x := make([]float64, n)
	for row := n - 1; row >= 0; row-- {
		sum := rhs[row]
		for c := row + 1; c < n; c++ {
			sum -= m[row][c] * x[c]
		}
		x[row] = sum / m[row][row]
	}

	return x, nil
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
