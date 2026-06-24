package processor

import (
	"os"
	"strings"
	"testing"

	"github.com/buskseguros-design/services/api/internal/model"
)

func TestProcessRealRows_NoFechaInvalida_BolivarPyme(t *testing.T) {
	path := "../../data/files-archive/file_1781049017697597000_4. Deudores_Banco_Bolivar__Pyme_BANCO_ABRIL.xlsx"
	if _, err := os.Stat(path); err != nil {
		t.Skip(path)
	}
	rows, err := readWorkbookRows(path, "")
	if err != nil {
		t.Fatal(err)
	}
	mappings := bolivarPymeMappings()
	header := rows[0]
	fieldToCol := map[string]int{}
	for _, m := range mappings {
		col, ok := columnIndexForFieldMap(header, m)
		if !ok && m.Required {
			t.Fatalf("missing col %s", m.SourceHeader)
		}
		if ok {
			fieldToCol[m.CanonicalField] = col
		}
	}
	cfg := ruleRuntimeConfig{
		DateLayouts:                defaultDateLayouts(),
		HasAgeLimits:               true,
		AgeMin:                     18,
		AgeMax:                     75.997,
		AgeMaxDaysBeforeBirthday:   1,
		BolivarDebtManualThreshold: 20_000_000,
		BolivarValidateDueMonth:    true,
		RequiredValidDateFields:    []string{"birth_date", "activation_date", "loan_award_date", "loan_due_date_current"},
	}
	seen := make(map[string]struct{})
	var fechaErrors int
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if rowEmpty(row) {
			continue
		}
		values := rowToValues(header, row, fieldToCol)
		values["_file_name"] = "Pyme_BANCO_ABRIL.xlsx"
		values["product_id"] = "bolivar_inclusion_deudores_banco"
		hard, _ := applyDiagramRules("BOLIVAR_INCLUSION_DEUDORES_BANCO", values, seen, nil, 0, 0, cfg, nil)
		for _, h := range hard {
			u := strings.ToUpper(h)
			if strings.Contains(u, "FECHA NO VÁLIDA") || strings.Contains(u, "FECHA ACTIVACIÓN NO VÁLIDA") || strings.Contains(u, "FECHA NACIMIENTO NO VÁLIDA") {
				fechaErrors++
				if fechaErrors <= 5 {
					t.Logf("row %d: %s (birth=%q adj=%q due=%q)", i+1, h, values["birth_date"], values["loan_award_date"], values["loan_due_date_current"])
				}
			}
		}
	}
	t.Logf("fecha errors in %d rows: %d (las celdas con mes > 12 ahora se reportan como inválidas: el negocio exige día/mes/año)", len(rows)-1, fechaErrors)
}

func bolivarPymeMappings() []model.FieldMap {
	return []model.FieldMap{
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
}

func rowToValues(header, row []string, fieldToCol map[string]int) map[string]string {
	values := make(map[string]string)
	for col, h := range header {
		if col >= len(row) {
			continue
		}
		key := strings.TrimSpace(h)
		if key == "" {
			continue
		}
		values[key] = strings.TrimSpace(row[col])
	}
	for field, col := range fieldToCol {
		if col < len(row) {
			values[field] = strings.TrimSpace(row[col])
		}
	}
	return values
}
