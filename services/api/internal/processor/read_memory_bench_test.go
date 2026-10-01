package processor

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/buskseguros-design/services/api/internal/model"
	"github.com/buskseguros-design/services/api/internal/store"
	"github.com/xuri/excelize/v2"
)

// Medición del coste de memoria de la fase de lectura de un workbook real.
// Se ejecuta en procesos separados (una variable de entorno por modo) porque el pico de RSS
// no se puede atribuir a un subtest si todos comparten proceso.
//
// Uso: BENCH_XLSX=/ruta/archivo.xlsx BENCH_MODE=all|one|headers go test -run TestMedirMemoriaLectura
//
// all     = comportamiento anterior: GetRows de cada hoja y todas vivas a la vez.
// one     = fase 2 actual: GetRows sólo de la hoja ganadora.
// headers = fase 1 actual: iterador que se detiene tras la fila de encabezado.
func TestMedirMemoriaLectura(t *testing.T) {
	path := os.Getenv("BENCH_XLSX")
	if path == "" {
		t.Skip("BENCH_XLSX no definido")
	}
	mode := os.Getenv("BENCH_MODE")
	if mode == "" {
		mode = "one"
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	f, err := excelize.OpenFile(path, excelize.Options{ShortDatePattern: "dd/mm/yyyy"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	var totalRows int

	switch mode {
	case "all":
		for _, name := range sheets {
			rows, err := f.GetRows(name, excelize.Options{ShortDatePattern: "dd/mm/yyyy", RawCellValue: false})
			if err != nil {
				t.Fatalf("getrows %q: %v", name, err)
			}
			totalRows += len(rows)
			_ = rows // se mantiene viva: es el punto de la medición
		}
	case "one":
		rows, err := f.GetRows(sheets[0], excelize.Options{ShortDatePattern: "dd/mm/yyyy", RawCellValue: false})
		if err != nil {
			t.Fatalf("getrows: %v", err)
		}
		totalRows = len(rows)
		_ = rows
	case "headers":
		for _, name := range sheets {
			rows, err := firstRowsWithExcelize(f, name, 20)
			if err != nil {
				continue
			}
			totalRows += len(rows)
		}
	default:
		t.Fatalf("BENCH_MODE desconocido: %q", mode)
	}

	runtime.GC()
	runtime.ReadMemStats(&after)
	liveMB := float64(after.HeapAlloc) / (1024 * 1024)
	fmt.Printf("MODO=%-8s hojas=%d filas_cargadas=%-8d heap_vivo=%7.1f MB total_asignado=%7.1f MB\n",
		mode, len(sheets), totalRows, liveMB, float64(after.TotalAlloc)/(1024*1024))
}

// Medición del reporte de validación sobre 167k filas. Es la fase sospechosa: producción
// murió con 1.85 GB cuando leer el libro entero apenas cuesta 539 MB.
//
// Modo "skipped_ligero" = lo que hace el código actual (las filas omitidas no llevan
// raw_data_json, commit 07a8e52).
// Modo "skipped_pesado" = lo que hacía antes (cada fila con el JSON crudo completo).
func TestMedirMemoriaReporte(t *testing.T) {
	n := 167473
	v, ok := os.LookupEnv("BENCH_ROWS")
	if !ok {
		t.Skip("BENCH_ROWS no definido: medición opt-in de memoria")
	}
	fmt.Sscanf(v, "%d", &n)
	mode := os.Getenv("BENCH_REPORT_MODE")
	if mode == "" {
		mode = "skipped_ligero"
	}

	policies := make([]model.PolicyRecord, 0, n)
	for i := 0; i < n; i++ {
		p := model.PolicyRecord{
			RowNumber:      i + 2,
			DocumentNumber: fmt.Sprintf("CC%09d", 100000000+i),
			CreditNumber:   fmt.Sprintf("%d", 1000000+i),
			PolicyStatus:   model.PolicyStatusSkippedHistoricalDup,
			ValidationJSON: `["crédito ya cargado en corrida previa; fila omitida sin reinsertar"]`,
		}
		if mode == "skipped_pesado" {
			p.FileID = "file_1790805024238311839"
			p.ProductID = "BOLIVAR_INCLUSION_DEUDORES_BANCO"
			p.FileName = "MICRO_BANCO.xlsx"
			p.RawDataJSON = `{"document_number":"CC100000000","credit_number":"1000000","edad":"40","prima":"50000","plan":"A","nombre":"APELLIDO NOMBRE","direccion":"CALLE 123","ciudad":"BOGOTA","telefono":"3001234567","email":"a@b.com","fecha_nacimiento":"15/01/1980","ocupacion":"EMPLEADO","ingreso":"2000000"}`
			p.CreatedAt = time.Now().UTC()
		}
		policies = append(policies, p)
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	report := store.BuildFileValidationReportFromPolicies(
		"file_1790805024238311839", "MICRO_BANCO.xlsx", "BOLIVAR_INCLUSION_DEUDORES_BANCO",
		string(model.FileStatusProcessed), "", "2026-09-30T00:00:00Z", policies)

	b, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	runtime.GC()
	runtime.ReadMemStats(&after)
	fmt.Printf("REPORTE modo=%-15s filas=%-7d json=%.1f MB heap_vivo=%7.1f MB\n",
		mode, n, float64(len(b))/(1024*1024), float64(after.HeapAlloc)/(1024*1024))
}

// Última fase medible sin base de datos: generar el XLSX espejo del reporte.
// saveValidationReportArchive pide los bytes completos a ValidationReportClientXLSX y los
// escribe con un solo os.WriteFile, así que el libro entero vive en memoria dos veces.
func TestMedirMemoriaXLSXReporte(t *testing.T) {
	n := 167473
	v, ok := os.LookupEnv("BENCH_ROWS")
	if !ok {
		t.Skip("BENCH_ROWS no definido: medición opt-in de memoria")
	}
	fmt.Sscanf(v, "%d", &n)
	mode := os.Getenv("BENCH_REPORT_MODE")
	if mode == "" {
		mode = "skipped_ligero"
	}

	policies := make([]model.PolicyRecord, 0, n)
	for i := 0; i < n; i++ {
		p := model.PolicyRecord{
			RowNumber:      i + 2,
			DocumentNumber: fmt.Sprintf("CC%09d", 100000000+i),
			CreditNumber:   fmt.Sprintf("%d", 1000000+i),
			PolicyStatus:   model.PolicyStatusSkippedHistoricalDup,
			ValidationJSON: `["crédito ya cargado en corrida previa; fila omitida sin reinsertar"]`,
		}
		if mode == "skipped_pesado" {
			p.FileID = "file_1790805024238311839"
			p.ProductID = "BOLIVAR_INCLUSION_DEUDORES_BANCO"
			p.FileName = "MICRO_BANCO.xlsx"
			p.RawDataJSON = `{"document_number":"CC100000000","credit_number":"1000000","edad":"40","prima":"50000","plan":"A","nombre":"APELLIDO NOMBRE","direccion":"CALLE 123","ciudad":"BOGOTA","telefono":"3001234567","email":"a@b.com","fecha_nacimiento":"15/01/1980","ocupacion":"EMPLEADO","ingreso":"2000000"}`
			p.CreatedAt = time.Now().UTC()
		}
		policies = append(policies, p)
	}

	report := store.BuildFileValidationReportFromPolicies(
		"file_1790805024238311839", "MICRO_BANCO.xlsx", "BOLIVAR_INCLUSION_DEUDORES_BANCO",
		string(model.FileStatusProcessed), "", "2026-09-30T00:00:00Z", policies)
	_ = policies

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	data, err := store.ValidationReportClientXLSX(report)
	if err != nil {
		t.Fatalf("xlsx: %v", err)
	}

	runtime.GC()
	runtime.ReadMemStats(&after)
	fmt.Printf("XLSX   modo=%-15s filas=%-7d xlsx=%.1f MB heap_vivo=%7.1f MB\n",
		mode, n, float64(len(data))/(1024*1024), float64(after.HeapAlloc)/(1024*1024))
}
