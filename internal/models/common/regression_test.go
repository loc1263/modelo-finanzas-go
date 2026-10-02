package common

import "testing"

// TestOLS_RecoversExactLinearRelation construye y = 2 + 3*x1 - 1*x2 sin
// ruido: una regresion correcta debe recuperar esos coeficientes casi
// exactos y dejar residuos ~0.
func TestOLS_RecoversExactLinearRelation(t *testing.T) {
	x := [][]float64{
		{1, 1}, {2, 1}, {3, 2}, {4, 2}, {5, 3},
		{6, 1}, {7, 4}, {8, 2}, {9, 5}, {10, 3},
	}
	y := make([]float64, len(x))
	for i, row := range x {
		y[i] = 2 + 3*row[0] - 1*row[1]
	}

	result, err := OLS(y, x)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(result.Coefficients) != 3 {
		t.Fatalf("len(Coefficients) = %d, want 3", len(result.Coefficients))
	}

	wantCoef := []float64{2, 3, -1}
	for i, want := range wantCoef {
		if got := result.Coefficients[i]; !almostEqualTol(got, want, 1e-6) {
			t.Errorf("Coefficients[%d] = %v, want %v", i, got, want)
		}
	}
	for i, r := range result.Residuals {
		if !almostEqualTol(r, 0, 1e-6) {
			t.Errorf("Residuals[%d] = %v, want ~0", i, r)
		}
	}
}

func TestOLS_DimensionErrors(t *testing.T) {
	if _, err := OLS(nil, nil); err == nil {
		t.Error("esperaba error con y vacio")
	}
	if _, err := OLS([]float64{1, 2}, [][]float64{{1}}); err == nil {
		t.Error("esperaba error con dimensiones de x distintas a y")
	}
	// Menos observaciones que parametros a estimar (intercepto + 2 regresores = 3).
	if _, err := OLS([]float64{1, 2}, [][]float64{{1, 1}, {2, 1}}); err == nil {
		t.Error("esperaba error por pocas observaciones")
	}
}

func TestOLS_CollinearRegressorsFails(t *testing.T) {
	// x2 = 2*x1 siempre: la matriz queda singular.
	x := [][]float64{{1, 2}, {2, 4}, {3, 6}, {4, 8}}
	y := []float64{1, 2, 3, 4}
	if _, err := OLS(y, x); err == nil {
		t.Error("esperaba error por matriz singular (regresores colineales)")
	}
}

func almostEqualTol(a, b, tol float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < tol
}
