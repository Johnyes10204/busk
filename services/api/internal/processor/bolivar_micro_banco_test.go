package processor

import (
	"os"
	"strings"
	"testing"
	"time"

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

func TestMicroBancoAbrilArchivo_TodasLasFilas(t *testing.T) {
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
		{CanonicalField: "document_number", SourceHeader: "IDENTIFICACION", Required: true},
		{CanonicalField: "birth_date", SourceHeader: "FECHA DE NACIMIENTO", Required: true},
		{CanonicalField: "credit_number", SourceHeader: "OP BT", Required: true},
		{CanonicalField: "initial_debt_amount", SourceHeader: "DEUDA INICIAL", Required: true},
		{CanonicalField: "monthly_premium", SourceHeader: "PRIMA MENSUAL", Required: true},
		{CanonicalField: "rate_percent", SourceHeader: "%", Required: true},
		{CanonicalField: "activation_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
		{CanonicalField: "loan_award_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
		{CanonicalField: "loan_due_date_current", SourceHeader: "FECHA VENCIMIENTO ACTUAL", Required: true},
		{CanonicalField: "calculated_term", SourceHeader: "PLAZO CRÉDITO", Required: false},
		{CanonicalField: "observacion", SourceHeader: "OBSERVACIONES ABRIL 2026", Required: false},
	}
	fieldToCol := make(map[string]int)
	for _, m := range mappings {
		col, ok := columnIndexForFieldMap(header, m)
		if !ok {
			if m.Required {
				t.Logf("ADVERTENCIA: columna requerida no encontrada: %s", m.SourceHeader)
			}
			continue
		}
		fieldToCol[m.CanonicalField] = col
	}
	cfg := ruleRuntimeConfig{
		DateLayouts:               defaultDateLayouts(),
		BolivarPrimaCalcTolerance: 1,
		BolivarPlazoDiasTolerance: 31,
		BolivarValidateDueMonth:   true,
		HasAgeLimits:              true,
		AgeMin:                    18,
		AgeMax:                    75.997,
		AgeMaxDaysBeforeBirthday:  1,
		RequiredValidDateFields:   []string{"birth_date", "activation_date", "loan_award_date", "loan_due_date_current"},
	}
	const fileName = "MICRO_BANCO_ABRIL_VF_Pruebas.xlsx"

	now := time.Now().UTC()
	cargueYear, cargueMonth, _ := bolivarMesFacturacionDesdeArchivo(fileName)
	if cargueMonth == 0 {
		cargueYear, cargueMonth = bolivarMesMinimoVencimiento(now, -1)
	}

	var (
		totalFilas, congeladas, vencPasadoTotal, vencPasadoConPrima int
		totalHard, totalSoft                                         int
	)
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if rowEmpty(row) {
			continue
		}
		totalFilas++
		values := map[string]string{"_file_name": fileName, "product_code": "BOLIVAR_INCLUSION_DEUDORES_BANCO"}
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

		prima, _ := parseFlexibleNumber(values["monthly_premium"])
		if prima == 0 {
			congeladas++
		}

		dueRaw := strings.TrimSpace(values["loan_due_date_current"])
		if dueRaw != "" {
			due := bolivarVigenciaFecha(dueRaw, cfg.DateLayouts)
			if bolivarVencimientoAntesMesCargue(due, cargueYear, cargueMonth) {
				vencPasadoTotal++
				if prima > 0 {
					vencPasadoConPrima++
				}
			}
		}

		hard, soft := applyBolivarDiagramRules(values, cfg)
		hard = append(hard, mensajesFechasRequeridas(values, cfg, "BOLIVAR_MICRO_BANCO")...)
		totalHard += len(hard)
		totalSoft += len(soft)
	}
	t.Logf("=== RESUMEN ===")
	t.Logf("  Filas procesadas   : %d", totalFilas)
	t.Logf("  Congeladas (prima=0): %d  (esperado cliente: 2365)", congeladas)
	t.Logf("  Venc. pasado total : %d  (esperado cliente: 1027)", vencPasadoTotal)
	t.Logf("  Venc. pasado+prima : %d  (E.10 reportadas en informe)", vencPasadoConPrima)
	t.Logf("  Incidencias duras  : %d", totalHard)
	t.Logf("  Informes suaves    : %d", totalSoft)
	t.Logf("  Mes de cargue      : %02d/%d", cargueMonth, cargueYear)
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
