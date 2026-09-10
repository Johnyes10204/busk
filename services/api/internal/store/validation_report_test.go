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

func TestValidationReport_ClientXLSXUnicaHojaDatosArchivo(t *testing.T) {
	policies := []model.PolicyRecord{{
		RowNumber:      10,
		DocumentNumber: "123",
		CreditNumber:   "OP1",
		PolicyStatus:   "ACTIVE",
		ValidationJSON: `["` + validationnotes.Informativo("vencimiento anterior al mes de facturación") + `"]`,
	}}
	report := BuildFileValidationReportFromPolicies("f1", "MICRO_BANCO_ABRIL.xlsx", "bolivar_inclusion_deudores_banco", "PROCESSED", "", "", policies)
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
	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != "Datos archivo" {
		t.Fatalf("se espera una sola hoja «Datos archivo», got %v", sheets)
	}
	rows, _ := f.GetRows("Datos archivo")
	if len(rows) < 2 {
		t.Fatalf("Datos archivo debe tener encabezado + fila informativa, got %d", len(rows))
	}
	if !strings.Contains(rows[1][len(rows[1])-1], "vencimiento") {
		t.Fatalf("novedades debe contener la informativa: %q", rows[1][len(rows[1])-1])
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
	report := BuildFileValidationReportFromPolicies("f1", "MICRO.xlsx", "bolivar_inclusion_deudores_banco", "ERROR", "", "", policies)
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
	if header[0] != "IDENTIFICACION" || header[1] != "PRIMA MENSUAL" || header[len(header)-2] != "observaciones" || header[len(header)-1] != "novedades" {
		t.Fatalf("encabezado inesperado: %v", header)
	}
	if rows[1][0] != "111" {
		t.Fatalf("primera fila (bloqueante) debe llevar los datos originales, got %v", rows[1])
	}
	if !strings.Contains(rows[1][len(rows[1])-1], "prima no válida") {
		t.Fatalf("novedades fila bloqueante: %q", rows[1][len(rows[1])-1])
	}
	if rows[1][len(rows[1])-2] == "" {
		t.Fatalf("observaciones fila bloqueante vacías")
	}
	if rows[2][0] != "222" {
		t.Fatalf("segunda fila (informativa) debe llevar los datos originales, got %v", rows[2])
	}
	if !strings.Contains(rows[2][len(rows[2])-1], "vencimiento") {
		t.Fatalf("novedades fila informativa: %q", rows[2][len(rows[2])-1])
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
	report := BuildFileValidationReportFromPolicies("f1", "MICRO.xlsx", "bolivar_inclusion_deudores_banco", "ERROR", "", "", policies)
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
	report := BuildFileValidationReportFromPolicies("f1", "MICRO_BANCO_ABRIL.xlsx", "bolivar_inclusion_deudores_banco", "PROCESSED", "", "", policies)
	if report.TotalPendingValidations != 0 {
		t.Fatalf("congelada no va a incidencias: pending=%d", report.TotalPendingValidations)
	}
	if report.TotalInformativeValidations != 1 {
		t.Fatalf("congelada debe ir a informes: %d", report.TotalInformativeValidations)
	}
}

func TestValidationReport_MirrorIncluyeTodasLasFilas(t *testing.T) {
	colOrder, _ := json.Marshal([]string{"IDENTIFICACION"})
	rawFrozen, _ := json.Marshal(map[string]string{
		"_excel_column_order": string(colOrder),
		"IDENTIFICACION":      "111",
	})
	rawInfo, _ := json.Marshal(map[string]string{
		"_excel_column_order": string(colOrder),
		"IDENTIFICACION":      "222",
	})
	rawLimpia, _ := json.Marshal(map[string]string{
		"_excel_column_order": string(colOrder),
		"IDENTIFICACION":      "333",
	})
	rawCancelada, _ := json.Marshal(map[string]string{
		"_excel_column_order": string(colOrder),
		"IDENTIFICACION":      "444",
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
		{
			RowNumber:    10,
			PolicyStatus: "ACTIVE",
			RawDataJSON:  string(rawLimpia),
		},
		{
			RowNumber:    12,
			PolicyStatus: "CANCELLED",
			RawDataJSON:  string(rawCancelada),
		},
	}
	report := BuildFileValidationReportFromPolicies("f1", "MICRO_BANCO_ABRIL.xlsx", "bolivar_inclusion_deudores_banco", "PROCESSED", "", "", policies)
	if len(report.ExportedRows) != 4 {
		t.Fatalf("espejo debe incluir las 4 filas del archivo: exported=%d", len(report.ExportedRows))
	}
	if len(report.EmailExportedRows) != 4 {
		t.Fatalf("adjunto de correo debe incluir las 4 filas: email_exported=%d", len(report.EmailExportedRows))
	}
	byRow := map[int]FileExportedRow{}
	for _, r := range report.ExportedRows {
		byRow[r.RowNumber] = r
	}
	if obs := byRow[10].Observaciones; obs != "Sin novedad" {
		t.Fatalf("fila limpia debe tener 'Sin novedad', got %q", obs)
	}
	if nov := byRow[10].Novedades; nov != "" {
		t.Fatalf("fila limpia no debe tener novedades, got %q", nov)
	}
	if obs := byRow[12].Observaciones; obs != "Póliza cancelada" {
		t.Fatalf("fila cancelada debe tener 'Póliza cancelada', got %q", obs)
	}
	if nov := byRow[12].Novedades; nov != "" {
		t.Fatalf("fila cancelada no debe tener novedades, got %q", nov)
	}
	if byRow[5].Observaciones == "" || byRow[5].Novedades == "" {
		t.Fatalf("fila congelada debe conservar observaciones y novedades: %+v", byRow[5])
	}
	if byRow[8].Observaciones == "" || byRow[8].Novedades == "" {
		t.Fatalf("fila con aviso debe conservar observaciones y novedades: %+v", byRow[8])
	}
}

func TestValidationReport_MirrorIncluyeFilasDuplicadoHistorico(t *testing.T) {
	// Regresión: cuando todas las filas del archivo son duplicados históricos, el mirror
	// no debe quedar en solo encabezado. Cada fila SKIPPED_HISTORICAL_DUP se pinta con
	// observación informativa y las columnas del archivo original quedan preservadas.
	colOrder, _ := json.Marshal([]string{"NUMERO_CREDITO", "IDENTIFICACION"})
	rawA, _ := json.Marshal(map[string]string{
		"_excel_column_order": string(colOrder),
		"NUMERO_CREDITO":      "OP-1",
		"IDENTIFICACION":      "111",
	})
	rawB, _ := json.Marshal(map[string]string{
		"_excel_column_order": string(colOrder),
		"NUMERO_CREDITO":      "OP-2",
		"IDENTIFICACION":      "222",
	})
	policies := []model.PolicyRecord{
		{
			RowNumber:      5,
			CreditNumber:   "OP-1",
			DocumentNumber: "111",
			PolicyStatus:   model.PolicyStatusSkippedHistoricalDup,
			ValidationJSON: `["` + validationnotes.Informativo("crédito ya cargado en corrida previa; fila omitida sin reinsertar") + `"]`,
			RawDataJSON:    string(rawA),
			SkipInsert:     true,
		},
		{
			RowNumber:      6,
			CreditNumber:   "OP-2",
			DocumentNumber: "222",
			PolicyStatus:   model.PolicyStatusSkippedHistoricalDup,
			ValidationJSON: `["` + validationnotes.Informativo("crédito ya cargado en corrida previa; fila omitida sin reinsertar") + `"]`,
			RawDataJSON:    string(rawB),
			SkipInsert:     true,
		},
	}
	report := BuildFileValidationReportFromPolicies("f1", "MICRO_ESAL_AGOSTO.xlsx", "bolivar_inclusion_deudores_esal_micro", "PROCESSED", "", "", policies)
	if len(report.ExportedRows) != 2 {
		t.Fatalf("espejo debe incluir las 2 filas SKIPPED: got=%d", len(report.ExportedRows))
	}
	if got := report.TotalInformativeValidations; got != 2 {
		t.Fatalf("informativos debe contar las 2 filas SKIPPED: got=%d", got)
	}
	if got := report.TotalPendingValidations; got != 0 {
		t.Fatalf("pendings no debe contarlas: got=%d", got)
	}
	for _, ex := range report.ExportedRows {
		if ex.Observaciones == "" || !strings.Contains(strings.ToLower(ex.Observaciones), "omitida") {
			t.Fatalf("observación esperada 'omitida ...', got=%q (fila %d)", ex.Observaciones, ex.RowNumber)
		}
		if ex.Novedades == "" {
			t.Fatalf("fila SKIPPED debe tener novedad informativa (fila %d)", ex.RowNumber)
		}
		if ex.Data["NUMERO_CREDITO"] == "" {
			t.Fatalf("fila SKIPPED debe conservar columnas del archivo original (fila %d)", ex.RowNumber)
		}
	}

	xlsx, err := ValidationReportClientXLSX(report)
	if err != nil {
		t.Fatalf("XLSX: %v", err)
	}
	if len(xlsx) == 0 {
		t.Fatalf("XLSX vacío")
	}
	f, err := excelize.OpenReader(bytes.NewReader(xlsx))
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	defer f.Close()
	rows, err := f.GetRows("Datos archivo")
	if err != nil {
		t.Fatalf("get rows: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("XLSX espejo debe tener 1 header + 2 filas: got=%d", len(rows))
	}
	if rows[0][0] != "NUMERO_CREDITO" || rows[0][1] != "IDENTIFICACION" {
		t.Fatalf("encabezado espejo debe conservar columnas originales: got=%v", rows[0])
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
	report := BuildFileValidationReportFromPolicies("f1", "INCLUSION.xlsx", "mapfre_inclusion_vida_voluntario", "ERROR", "", "", policies)
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
	if rows[0][len(rows[0])-2] != "observaciones" || rows[0][len(rows[0])-1] != "novedades" {
		t.Fatalf("columnas finales: %v", rows[0])
	}
	if !strings.Contains(rows[1][len(rows[1])-1], "prima") {
		t.Fatalf("novedades: %s", rows[1][len(rows[1])-1])
	}
	if !strings.Contains(rows[1][len(rows[1])-2], "REVISAR") {
		t.Fatalf("observaciones: %s", rows[1][len(rows[1])-2])
	}
}
