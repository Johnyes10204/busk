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
