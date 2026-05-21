package processor

import (
	"os"
	"strings"
	"testing"

	"github.com/buskseguros-design/services/api/internal/model"
)

func TestMicroBancoAbrilArchivo_MapeoTasaYPrima(t *testing.T) {
	path := "/Users/johnnelsonflorez/Documents/ipsilon/busk/tools/sftpconnect/downloads/MICRO_BANCO_ABRIL_VF_Pruebas.xlsx"
	if _, err := os.Stat(path); err != nil {
		t.Skip("archivo de prueba no disponible")
	}
	rows, err := readWorkbookRows(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatal("sin filas de datos")
	}
	header := rows[0]
	mappings := []model.FieldMap{
		{CanonicalField: "initial_debt_amount", SourceHeader: "DEUDA INICIAL", Required: true},
		{CanonicalField: "monthly_premium", SourceHeader: "PRIMA MENSUAL", Required: true},
		{CanonicalField: "rate_percent", SourceHeader: "%", Required: true},
		{CanonicalField: "loan_award_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
		{CanonicalField: "loan_due_date_current", SourceHeader: "FECHA VENCIMIENTO ACTUAL", Required: true},
	}
	fieldToCol := make(map[string]int)
	for _, m := range mappings {
		col, ok := columnIndexForFieldMap(header, m)
		if !ok {
			t.Fatalf("falta columna %s", m.SourceHeader)
		}
		fieldToCol[m.CanonicalField] = col
	}
	values := make(map[string]string)
	row := rows[1]
	for field, col := range fieldToCol {
		if col < len(row) {
			values[field] = strings.TrimSpace(row[col])
		}
	}
	for col, h := range header {
		if col < len(row) {
			values[strings.TrimSpace(h)] = strings.TrimSpace(row[col])
		}
	}
	if values["rate_percent"] == "0.23" || strings.Contains(values["rate_percent"], "23") && !strings.Contains(strings.ToLower(values["rate_percent"]), "e-") {
		t.Fatalf("rate_percent debe ser 1E-3/0.001, no %% con espacio (0.23): %q", values["rate_percent"])
	}
	primaCalc, _ := bolivarPrimaEsperada(25_000_000, values["rate_percent"])
	if primaCalc < 24_999 || primaCalc > 25_001 {
		t.Fatalf("prima esperada 25000, got %v (tasa %q)", primaCalc, values["rate_percent"])
	}
	cfg := ruleRuntimeConfig{
		DateLayouts:               defaultDateLayouts(),
		BolivarPrimaCalcTolerance: 1,
		BolivarPlazoDiasTolerance: 31,
	}
	hard, soft := applyBolivarDiagramRules(values, cfg)
	for _, h := range hard {
		if strings.Contains(strings.ToLower(h), "prima mensual") && strings.Contains(strings.ToLower(h), "no coincide") {
			t.Fatalf("fila 1 no debe fallar prima/tasa: %v soft=%v", hard, soft)
		}
	}
}

func TestMicroBancoEdadEnFechaAdjudicacion(t *testing.T) {
	values := map[string]string{
		"birth_date":      "32461",
		"activation_date": "44581",
		"loan_award_date": "44581",
	}
	cfg := ruleRuntimeConfig{
		DateLayouts:              defaultDateLayouts(),
		HasAgeLimits:             true,
		AgeMin:                   18,
		AgeMax:                   75.997,
		AgeMaxDaysBeforeBirthday: 1,
	}
	det := evaluarEdadDetalle(values, cfg, false, "BOLIVAR_BANCO")
	if !det.refValid {
		t.Fatalf("fecha adjudicación debe parsear: raw=%q", det.activacionRaw)
	}
	if !det.cumple {
		t.Fatalf("edad en activación debe cumplir 18-75: edad=%d ref=%s", det.edadReportada, formatFechaCalendario(det.ref))
	}
	if det.edadReportada < 32 || det.edadReportada > 34 {
		t.Fatalf("edad esperada ~33 años, got %d", det.edadReportada)
	}
}
