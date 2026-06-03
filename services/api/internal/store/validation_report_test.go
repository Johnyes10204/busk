package store

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/buskseguros-design/services/api/internal/model"
	"github.com/buskseguros-design/services/api/internal/validationnotes"
	"github.com/xuri/excelize/v2"
)

func TestValidationReport_InformativeSeparateFromIncidencias(t *testing.T) {
	policies := []model.PolicyRecord{{
		RowNumber:      10,
		DocumentNumber: "123",
		CreditNumber:   "OP1",
		PolicyStatus:   "ACTIVE",
		ValidationJSON: `["` + validationnotes.Informativo("vencimiento anterior al mes de facturación") + `"]`,
	}}
	report := BuildFileValidationReportFromPolicies("f1", "MICRO_BANCO_ABRIL.xlsx", "bolivar_banco", "PROCESSED", "", "", policies)
	if report.TotalPendingValidations != 0 {
		t.Fatalf("avisos informativos no deben ir a incidencias: pending=%d", report.TotalPendingValidations)
	}
	if report.TotalInformativeValidations != 1 {
		t.Fatalf("informative=%d", report.TotalInformativeValidations)
	}
	b, err := ValidationReportClientXLSX(report)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	incRows, _ := f.GetRows("Incidencias")
	if len(incRows) != 1 {
		t.Fatalf("hoja Incidencias debe tener solo encabezado, got %d filas", len(incRows))
	}
	infoRows, _ := f.GetRows("Informes")
	if len(infoRows) < 2 {
		t.Fatalf("hoja Informes debe tener datos, got %d filas", len(infoRows))
	}
	if infoRows[1][0] != "Informe informativo" {
		t.Fatalf("tipo registro: %q", infoRows[1][0])
	}
}

func TestValidationReport_EmailXLSXUnicaHoja(t *testing.T) {
	colOrder, _ := json.Marshal([]string{"IDENTIFICACION", "PRIMA MENSUAL"})
	rawBloqueante, _ := json.Marshal(map[string]string{
		"_excel_column_order": string(colOrder),
		"IDENTIFICACION":      "111",
		"PRIMA MENSUAL":       "0",
	})
	rawInfo, _ := json.Marshal(map[string]string{
		"_excel_column_order": string(colOrder),
		"IDENTIFICACION":      "222",
		"PRIMA MENSUAL":       "8600",
	})
	policies := []model.PolicyRecord{
		{
			RowNumber:      3,
			PolicyStatus:   "MANUAL_REVIEW",
			ValidationJSON: `["` + validationnotes.Incidencia("prima no válida") + `"]`,
			RawDataJSON:    string(rawBloqueante),
		},
		{
			RowNumber:      5,
			PolicyStatus:   "ACTIVE",
			ValidationJSON: `["` + validationnotes.Informativo("vencimiento anterior al mes de facturación") + `"]`,
			RawDataJSON:    string(rawInfo),
		},
	}
	report := BuildFileValidationReportFromPolicies("f1", "MICRO.xlsx", "bolivar_banco", "ERROR", "", "", policies)
	b, err := ValidationReportEmailXLSX(report)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != "Reporte" {
		t.Fatalf("se espera una sola hoja «Reporte», got %v", sheets)
	}
	for _, sheet := range []string{"Incidencias", "Informes", "Datos archivo"} {
		if idx, _ := f.GetSheetIndex(sheet); idx >= 0 {
			t.Fatalf("correo no debe incluir hoja %s", sheet)
		}
	}
	rows, _ := f.GetRows("Reporte")
	if len(rows) != 3 {
		t.Fatalf("encabezado + 2 filas (bloqueante e informativa), got %d", len(rows))
	}
	header := rows[0]
	if header[0] != "fila_excel" || header[1] != "estado_poliza" || header[len(header)-1] != "novedades" {
		t.Fatalf("encabezado inesperado: %v", header)
	}
	if rows[1][0] != "3" {
		t.Fatalf("primera fila debe ser la 3 (bloqueante), got %v", rows[1])
	}
	if !strings.Contains(rows[1][len(rows[1])-1], "prima no válida") {
		t.Fatalf("novedades fila 3: %q", rows[1][len(rows[1])-1])
	}
	if rows[2][0] != "5" {
		t.Fatalf("segunda fila debe ser la 5 (informativa), got %v", rows[2])
	}
	if !strings.Contains(rows[2][len(rows[2])-1], "vencimiento") {
		t.Fatalf("novedades fila 5: %q", rows[2][len(rows[2])-1])
	}
}

func TestValidationReport_EmailXLSXIncluyeBloqueantesEInformativasMismaFila(t *testing.T) {
	colOrder, _ := json.Marshal([]string{"IDENTIFICACION"})
	raw, _ := json.Marshal(map[string]string{
		"_excel_column_order": string(colOrder),
		"IDENTIFICACION":      "999",
	})
	policies := []model.PolicyRecord{{
		RowNumber:    4,
		PolicyStatus: "MANUAL_REVIEW",
		ValidationJSON: `["` + validationnotes.Incidencia("prima inválida") + `","` +
			validationnotes.Informativo("aviso adicional") + `"]`,
		RawDataJSON: string(raw),
	}}
	report := BuildFileValidationReportFromPolicies("f1", "MICRO.xlsx", "bolivar_banco", "ERROR", "", "", policies)
	if len(report.EmailExportedRows) != 1 {
		t.Fatalf("una sola fila esperada, got %d", len(report.EmailExportedRows))
	}
	nov := report.EmailExportedRows[0].Novedades
	bloqIdx := strings.Index(nov, "prima inválida")
	infoIdx := strings.Index(nov, "aviso adicional")
	if bloqIdx < 0 || infoIdx < 0 {
		t.Fatalf("ambas notas deben aparecer: %q", nov)
	}
	if bloqIdx > infoIdx {
		t.Fatalf("bloqueante debe aparecer antes que informativa: %q", nov)
	}
}

func TestValidationReport_PolizaCongeladaEnInformes(t *testing.T) {
	policies := []model.PolicyRecord{{
		RowNumber:      7,
		DocumentNumber: "555",
		CreditNumber:   "OP-FRZ",
		PolicyStatus:   "FROZEN",
		ValidationJSON: `["` + validationnotes.Informativo("La prima mensual es cero; la póliza se registra como congelada (no bloquea la carga del archivo).") + `"]`,
	}}
	report := BuildFileValidationReportFromPolicies("f1", "MICRO_BANCO_ABRIL.xlsx", "bolivar_banco", "PROCESSED", "", "", policies)
	if report.TotalPendingValidations != 0 {
		t.Fatalf("congelada no va a incidencias: pending=%d", report.TotalPendingValidations)
	}
	if report.TotalInformativeValidations != 1 {
		t.Fatalf("congelada debe ir a informes: %d", report.TotalInformativeValidations)
	}
}

func TestValidationReport_MirrorSoloFilasConFallo(t *testing.T) {
	colOrder, _ := json.Marshal([]string{"IDENTIFICACION"})
	rawFrozen, _ := json.Marshal(map[string]string{
		"_excel_column_order": string(colOrder),
		"IDENTIFICACION":      "111",
	})
	rawInfo, _ := json.Marshal(map[string]string{
		"_excel_column_order": string(colOrder),
		"IDENTIFICACION":      "222",
	})
	policies := []model.PolicyRecord{
		{
			RowNumber:      5,
			PolicyStatus:   "FROZEN",
			ValidationJSON: `["` + validationnotes.Informativo("prima cero congelada") + `"]`,
			RawDataJSON:    string(rawFrozen),
		},
		{
			RowNumber:      8,
			PolicyStatus:   "ACTIVE",
			ValidationJSON: `["` + validationnotes.Informativo("vencimiento anterior al mes de facturación") + `"]`,
			RawDataJSON:    string(rawInfo),
		},
	}
	report := BuildFileValidationReportFromPolicies("f1", "MICRO_BANCO_ABRIL.xlsx", "bolivar_banco", "ERROR", "", "", policies)
	if len(report.ExportedRows) != 0 {
		t.Fatalf("solo informativas/congeladas no deben ir a Datos archivo: exported=%d", len(report.ExportedRows))
	}
}

func TestValidationReport_MirrorSheetConDatosArchivo(t *testing.T) {
	colOrder, _ := json.Marshal([]string{"IDENTIFICACION", "PRIMA MENSUAL"})
	raw, _ := json.Marshal(map[string]string{
		"_excel_column_order": string(colOrder),
		"IDENTIFICACION":      "999",
		"PRIMA MENSUAL":       "8600",
		"document_number":     "999",
	})
	policies := []model.PolicyRecord{{
		RowNumber:      3,
		PolicyStatus:   "MANUAL_REVIEW",
		ValidationJSON: `["Incidencia: prima no válida"]`,
		RawDataJSON:    string(raw),
	}}
	report := BuildFileValidationReportFromPolicies("f1", "INCLUSION.xlsx", "mapfre_vida", "ERROR", "", "", policies)
	if len(report.ExportedRows) != 1 {
		t.Fatalf("exported=%d", len(report.ExportedRows))
	}
	if len(report.SourceColumns) < 2 {
		t.Fatalf("columns=%v", report.SourceColumns)
	}
	b, err := ValidationReportClientXLSX(report)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	rows, _ := f.GetRows("Datos archivo")
	if len(rows) < 2 {
		t.Fatalf("mirror rows=%d", len(rows))
	}
	if rows[0][len(rows[0])-1] != "novedades" {
		t.Fatalf("última columna debe ser novedades: %v", rows[0])
	}
	if !strings.Contains(rows[1][len(rows[1])-1], "prima") {
		t.Fatalf("novedades: %s", rows[1][len(rows[1])-1])
	}
}
