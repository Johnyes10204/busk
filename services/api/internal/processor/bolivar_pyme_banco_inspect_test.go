package processor

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/buskseguros-design/services/api/internal/model"
	"github.com/buskseguros-design/services/api/internal/store"
)

func TestProductFreezesOnZeroPremiumBolivarSinReglaBD(t *testing.T) {
	rules := []model.RuleConfig{
		{Type: "number_gte", Field: "monthly_premium", Params: map[string]float64{"min": 0}},
	}
	if !productFreezesOnZeroPremium("BOLIVAR_INCLUSION_DEUDORES_BANCO", rules) {
		t.Fatal("Bolívar debe congelar prima 0 aunque falte freeze_on_zero_premium en rules_json")
	}
	values := map[string]string{"monthly_premium": "0"}
	frozen, violations := runRules(values, rules)
	if frozen {
		t.Fatal("runRules sin regla freeze no debe congelar")
	}
	if !frozen && productFreezesOnZeroPremium("BOLIVAR_INCLUSION_DEUDORES_BANCO", rules) {
		if prem, _ := parseFlexibleNumber(values["monthly_premium"]); prem == 0 {
			frozen = true
		}
	}
	if !frozen {
		t.Fatal("fallback Bolívar debe congelar prima 0")
	}
	if len(violations) > 0 {
		t.Fatalf("prima 0 no debe generar incidencias: %v", violations)
	}
}

func TestPymeBancoAbrilArchivo_PrimaCeroCongelada(t *testing.T) {
	paths := []string{
		"/Users/johnnelsonflorez/Documents/ipsilon/busk/tools/sftpconnect/downloads/4. Deudores_Banco_Bolivar__Pyme_BANCO_ABRIL.xlsx",
	}
	var path string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			path = p
			break
		}
	}
	if path == "" {
		t.Skip("archivo Pyme BANCO ABRIL no disponible")
	}

	rows, err := readWorkbookRows(path, "CASTIGOS ABRIL")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatal("sin filas en primera hoja")
	}
	header := rows[0]
	mappings := []model.FieldMap{
		{CanonicalField: "monthly_premium", SourceHeader: "PRIMA MENSUAL", Required: true},
		{CanonicalField: "credit_number", SourceHeader: "OP BT", Required: true},
	}
	fieldToCol := make(map[string]int)
	for _, m := range mappings {
		col, ok := columnIndexForFieldMap(header, m)
		if !ok {
			t.Fatalf("falta columna %s", m.SourceHeader)
		}
		fieldToCol[m.CanonicalField] = col
	}
	rules := []model.RuleConfig{
		{Type: "number_gte", Field: "monthly_premium", Params: map[string]float64{"min": 0}},
	}
	code := "BOLIVAR_INCLUSION_DEUDORES_BANCO"

	var total, primaZero, frozenOK int
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if rowEmpty(row) {
			continue
		}
		total++
		values := map[string]string{}
		for field, col := range fieldToCol {
			if col < len(row) {
				values[field] = strings.TrimSpace(row[col])
			}
		}
		prem, _ := parseFlexibleNumber(values["monthly_premium"])
		if prem != 0 {
			continue
		}
		primaZero++
		frozen, _ := runRules(values, rules)
		if !frozen && productFreezesOnZeroPremium(code, rules) {
			frozen = true
		}
		if frozen {
			frozenOK++
		}
	}
	t.Logf("filas=%d prima_cero=%d congeladas=%d (primera hoja)", total, primaZero, frozenOK)
	if primaZero > 0 && frozenOK != primaZero {
		t.Fatalf("todas las prima cero deben congelarse: zero=%d frozen=%d", primaZero, frozenOK)
	}
}

func TestPymeBancoAbrilArchivo_InformeReportaNovedades(t *testing.T) {
	path := "/Users/johnnelsonflorez/Documents/ipsilon/busk/tools/sftpconnect/downloads/4. Deudores_Banco_Bolivar__Pyme_BANCO_ABRIL.xlsx"
	if _, err := os.Stat(path); err != nil {
		t.Skip("archivo no disponible")
	}
	rows, err := readWorkbookRows(path, "")
	if err != nil {
		t.Fatal(err)
	}
	header := rows[0]
	mappings := []model.FieldMap{
		{CanonicalField: "document_number", SourceHeader: "IDENTIFICACION", Required: true},
		{CanonicalField: "birth_date", SourceHeader: "FECHA DE NACIMIENTO", Required: true},
		{CanonicalField: "credit_number", SourceHeader: "OP BT", Required: true},
		{CanonicalField: "initial_debt_amount", SourceHeader: "DEUDA INICIAL", Required: true},
		{CanonicalField: "monthly_premium", SourceHeader: "PRIMA MENSUAL", Required: true},
		{CanonicalField: "rate_percent", SourceHeader: "%", Required: true},
		{CanonicalField: "activation_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
		{CanonicalField: "loan_award_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
		{CanonicalField: "loan_due_date_current", SourceHeader: "FECHA VENCIMIENTO ACTUAL", Required: true},
	}
	fieldToCol := make(map[string]int)
	for _, m := range mappings {
		col, ok := columnIndexForFieldMap(header, m)
		if !ok && m.Required {
			t.Fatalf("falta columna %s", m.SourceHeader)
		}
		if ok {
			fieldToCol[m.CanonicalField] = col
		}
	}
	rules := []model.RuleConfig{
		{Type: "number_gte", Field: "monthly_premium", Params: map[string]float64{"min": 0}},
		{Type: "freeze_on_zero_premium", Field: "monthly_premium"},
	}
	code := "BOLIVAR_INCLUSION_DEUDORES_BANCO"
	cfg := ruleRuntimeConfig{
		DateLayouts:             defaultDateLayouts(),
		BolivarValidateDueMonth: true,
	}

	policies := make([]model.PolicyRecord, 0)
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if rowEmpty(row) {
			continue
		}
		values := map[string]string{"_file_name": "4. Deudores_Banco_Bolivar__Pyme_BANCO_ABRIL.xlsx"}
		for field, col := range fieldToCol {
			if col < len(row) {
				values[field] = strings.TrimSpace(row[col])
			}
		}
		frozen, _ := runRules(values, rules)
		if !frozen && productFreezesOnZeroPremium(code, rules) {
			if prem, _ := parseFlexibleNumber(values["monthly_premium"]); prem == 0 {
				frozen = true
			}
		}
		status := "ACTIVE"
		notes := []string{}
		if frozen {
			status = "FROZEN"
			notes = append(notes, noteInformativo(mensajePolizaCongeladaPrimaCero()))
		}
		_, soft := applyBolivarDiagramRules(values, cfg)
		for _, msg := range soft {
			notes = append(notes, noteInformativo(msg))
		}
		raw, _ := json.Marshal(values)
		noteJSON, _ := json.Marshal(notes)
		policies = append(policies, model.PolicyRecord{
			RowNumber:      i + 1,
			DocumentNumber: values["document_number"],
			CreditNumber:   values["credit_number"],
			PolicyStatus:   status,
			RawDataJSON:    string(raw),
			ValidationJSON: string(noteJSON),
		})
	}

	report := store.BuildFileValidationReportFromPolicies(
		"file_test_pyme",
		"4. Deudores_Banco_Bolivar__Pyme_BANCO_ABRIL.xlsx",
		"bolivar_inclusion_deudores_banco",
		"PROCESSED",
		"",
		time.Now().UTC().Format(time.RFC3339Nano),
		policies,
	)
	t.Logf("informes=%d incidencias=%d email_rows=%d",
		report.TotalInformativeValidations, report.TotalPendingValidations, len(report.EmailExportedRows))
	if report.TotalInformativeValidations == 0 {
		t.Fatal("debe haber informes (congeladas y/o vencimiento)")
	}
	t.Setenv("REPORTS_ARCHIVE_DIR", t.TempDir())
	jsonReport, archivePath := validationReportFromPolicies(
		"file_test_pyme",
		"4. Deudores_Banco_Bolivar__Pyme_BANCO_ABRIL.xlsx",
		"bolivar_inclusion_deudores_banco",
		"PROCESSED",
		"",
		time.Now().UTC(),
		policies,
	)
	if jsonReport == "" {
		t.Fatal("validationReportFromPolicies no debe quedar vacío")
	}
	if archivePath == "" {
		t.Fatal("se esperaba ruta de XLSX de auditoría")
	}
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("XLSX de auditoría no encontrado en %s: %v", archivePath, err)
	}
}

func TestReadRowsWithExcelize_SiemprePrimeraHoja(t *testing.T) {
	path := "/Users/johnnelsonflorez/Documents/ipsilon/busk/tools/sftpconnect/downloads/4. Deudores_Banco_Bolivar__Pyme_BANCO_ABRIL.xlsx"
	if _, err := os.Stat(path); err != nil {
		t.Skip("archivo no disponible")
	}
	rows, err := readWorkbookRows(path, "CASTIGOS ABRIL")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 100 {
		t.Fatalf("debe leer FACTURACION (194 filas), no CASTIGOS (7): got %d filas", len(rows))
	}
}
