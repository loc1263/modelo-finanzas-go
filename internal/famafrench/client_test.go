package famafrench

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// buildZip arma un zip en memoria con un archivo por cada (nombre, contenido)
// dado, para simular lo que descarga Dartmouth sin pegarle a la red real.
func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatalf("creando %s en zip: %v", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("escribiendo %s en zip: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("cerrando zip: %v", err)
	}
	return buf.Bytes()
}

const sampleCSV = ",Mkt-RF,SMB,HML,RF\n" +
	"202601,1.23,0.45,-0.67,0.01\n" +
	"202602,2.00,-1.00,0.50,0.01\n" +
	"\n" +
	"  Annual Factors: January-December  \n" +
	"2026,10.0,5.0,-3.0,0.1\n"

// TestGetMonthly_RetriesTransientErrors cubre MEJ-011: una falla transitoria
// (ej. 500 momentaneo) no deberia abortar la descarga en el primer intento.
func TestGetMonthly_RetriesTransientErrors(t *testing.T) {
	var attempts int32
	zipBytes := buildZip(t, map[string]string{"F-F_Research_Data_Factors.CSV": sampleCSV})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(zipBytes)
	}))
	defer server.Close()

	c := NewClient(5*time.Second, time.Millisecond, 3, nil)
	c.urlTemplate = server.URL + "/%s_CSV.zip"

	got, err := c.GetMonthly("F-F_Research_Data_Factors")
	if err != nil {
		t.Fatalf("GetMonthly: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (2 fallos + 1 exito)", attempts)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
}

// TestGetMonthly_ExhaustsRetriesAndFails confirma que, agotados los
// reintentos, se devuelve un error claro en vez de reintentar indefinidamente.
func TestGetMonthly_ExhaustsRetriesAndFails(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(5*time.Second, time.Millisecond, 2, nil)
	c.urlTemplate = server.URL + "/%s_CSV.zip"

	if _, err := c.GetMonthly("F-F_Research_Data_Factors"); err == nil {
		t.Error("esperaba error tras agotar los reintentos")
	}
	if attempts != 3 { // intento inicial + 2 reintentos
		t.Errorf("attempts = %d, want 3 (1 inicial + 2 reintentos)", attempts)
	}
}

// TestFindCSVFile_SelectsByExtension cubre MEJ-016: el archivo se busca por
// extension .csv, no por posicion en el zip.
func TestFindCSVFile_SelectsByExtension(t *testing.T) {
	zipBytes := buildZip(t, map[string]string{
		"readme.txt":                    "no soy el csv",
		"F-F_Research_Data_Factors.CSV": sampleCSV,
	})
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("abriendo zip de prueba: %v", err)
	}

	f, err := findCSVFile(zr)
	if err != nil {
		t.Fatalf("findCSVFile: %v", err)
	}
	if f.Name != "F-F_Research_Data_Factors.CSV" {
		t.Errorf("findCSVFile devolvio %q, want el archivo .CSV", f.Name)
	}
}

func TestFindCSVFile_ErrorsOnZeroMatches(t *testing.T) {
	zipBytes := buildZip(t, map[string]string{"readme.txt": "nada de csv aca"})
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("abriendo zip de prueba: %v", err)
	}
	if _, err := findCSVFile(zr); err == nil {
		t.Error("esperaba error: ningun archivo .csv en el zip")
	}
}

func TestFindCSVFile_ErrorsOnMultipleMatches(t *testing.T) {
	zipBytes := buildZip(t, map[string]string{
		"a.csv": sampleCSV,
		"b.csv": sampleCSV,
	})
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("abriendo zip de prueba: %v", err)
	}
	if _, err := findCSVFile(zr); err == nil {
		t.Error("esperaba error: mas de un archivo .csv en el zip")
	}
}

func TestParseMonthly_StopsAtAnnualSection(t *testing.T) {
	got, err := parseMonthly([]byte(sampleCSV))
	if err != nil {
		t.Fatalf("parseMonthly: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (la seccion anual no debe incluirse)", len(got))
	}
	if got[0].Values["Mkt-RF"] != 0.0123 {
		t.Errorf("Mkt-RF del primer mes = %v, want 0.0123 (dividido por 100)", got[0].Values["Mkt-RF"])
	}
	if !got[1].Date.Equal(mustParseMonth(t, "2026-02")) {
		t.Errorf("Date del segundo mes = %v, want 2026-02", got[1].Date)
	}
}

func mustParseMonth(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse("2006-01", s)
	if err != nil {
		t.Fatalf("parseando fecha de prueba: %v", err)
	}
	return tm
}
