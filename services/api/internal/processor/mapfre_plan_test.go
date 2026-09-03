package processor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/buskseguros-design/services/api/internal/model"
	"github.com/buskseguros-design/services/api/internal/store"
)

func TestValidarPlanMapfre_PorNombrePlan(t *testing.T) {
	values := map[string]string{
		"plan_name":       "PLAN 1",
		"monthly_premium": "8600",
		"insured_amount":  "5000000",
	}
	if v := validarPlanMapfre("MAPFRE_VIDA", values); len(v) != 0 {
		t.Fatalf("plan 1 + prima 8600 + valor asegurado 5M debe pasar: %v", v)
	}
}

func TestValidarPlanMapfre_ValorAseguradoNoCoincide(t *testing.T) {
	values := map[string]string{
		"plan_name":       "PLAN 1",
		"monthly_premium": "8600",
		"insured_amount":  "10000000",
	}
	v := validarPlanMapfre("MAPFRE_VIDA", values)
	if len(v) != 1 {
		t.Fatalf("want 1 violation got %v", v)
	}
	if !strings.Contains(strings.ToLower(v[0]), "plan no válido") {
		t.Fatalf("mensaje: %s", v[0])
	}
}

func TestValidarPlanMapfre_ValorAseguradoObligatorio(t *testing.T) {
	values := map[string]string{
		"plan_name":       "PLAN 2",
		"monthly_premium": "17100",
	}
	v := validarPlanMapfre("MAPFRE_VIDA", values)
	if len(v) != 1 || !strings.Contains(strings.ToLower(v[0]), "valor asegurado") {
		t.Fatalf("sin valor asegurado debe fallar: %v", v)
	}
}

func TestValidarPlanMapfre_PrimaNoCoincidePlan(t *testing.T) {
	values := map[string]string{
		"plan_name":       "PLAN 1",
		"monthly_premium": "17100",
		"insured_amount":  "5000000",
	}
	v := validarPlanMapfre("MAPFRE_VIDA", values)
	if len(v) != 1 {
		t.Fatalf("want 1 violation got %v", v)
	}
	if !strings.Contains(strings.ToLower(v[0]), "no coincide") {
		t.Fatalf("mensaje: %s", v[0])
	}
}

func TestValidarPlanMapfre_IgnoraPlanCode(t *testing.T) {
	values := map[string]string{
		"plan_name":       "PLAN 1",
		"plan_code":       "99999",
		"monthly_premium": "8600",
		"insured_amount":  "5.000.000",
	}
	if v := validarPlanMapfre("MAPFRE_VIDA", values); len(v) != 0 {
		t.Fatalf("plan_code no debe usarse en validación: %v", v)
	}
}

func TestValidarPlanMapfre_SinNombrePlan(t *testing.T) {
	values := map[string]string{"monthly_premium": "8600"}
	v := validarPlanMapfre("MAPFRE_VIDA", values)
	if len(v) == 0 {
		t.Fatal("falta plan_name debe fallar")
	}
}

func TestValidarPlanMapfre_AccPlan1DosPrimas(t *testing.T) {
	for _, prem := range []string{"7800", "7410"} {
		values := map[string]string{
			"plan_name":       "PLAN 1",
			"monthly_premium": prem,
			"insured_amount":  "5000000",
		}
		if v := validarPlanMapfre("MAPFRE_ACC_MEN", values); len(v) != 0 {
			t.Fatalf("ACC plan 1 prima %s: %v", prem, v)
		}
	}
}

func TestInformeIncluyeValorAseguradoNoCoincide(t *testing.T) {
	values := map[string]string{
		"plan_name":         "PLAN 1",
		"monthly_premium":   "8600",
		"VALOR ASEGURADO":   "10000000",
	}
	msgs := validarPlanMapfre("MAPFRE_VIDA", values)
	if len(msgs) != 1 {
		t.Fatalf("want 1 violation got %v", msgs)
	}
	notes := make([]string, 0, len(msgs))
	for _, m := range msgs {
		notes = append(notes, noteIncidencia(m))
	}
	if !noteIsBlocking(notes[0]) {
		t.Fatalf("debe ser incidencia bloqueante: %s", notes[0])
	}
	notesJSON, _ := json.Marshal(notes)
	report := store.BuildFileValidationReportFromPolicies(
		"file_test",
		"INCLUSION.xlsx",
		"mapfre_vida",
		string(model.FileStatusError),
		"carga omitida",
		"2026-01-01T00:00:00Z",
		[]model.PolicyRecord{{
			RowNumber:      5,
			DocumentNumber: "123",
			CreditNumber:   "OP1",
			PolicyStatus:   "MANUAL_REVIEW",
			ValidationJSON: string(notesJSON),
		}},
	)
	if report.TotalPendingValidations != 1 {
		t.Fatalf("pending=%d", report.TotalPendingValidations)
	}
	csv, err := store.ValidationReportClientCSV(report)
	if err != nil {
		t.Fatal(err)
	}
	body := string(csv)
	if !strings.Contains(strings.ToLower(body), "valor asegurado") {
		t.Fatalf("informe CSV debe mencionar valor asegurado: %s", body)
	}
	if !strings.Contains(strings.ToLower(body), "plan no válido") {
		t.Fatalf("informe CSV debe incluir mensaje de plan: %s", body)
	}
}

func TestValidarPlanMapfre_PrimaTotalDivididaPorPlazo(t *testing.T) {
	// Archivo trae prima total del contrato; prima/plazo = mensual del tarifario (8600 × 12 = 103200).
	values := map[string]string{
		"plan_name":            "PLAN 1",
		"monthly_premium":      "103200",
		"initial_term_months":  "12",
		"insured_amount":       "5000000",
	}
	if v := validarPlanMapfre("MAPFRE_VIDA", values); len(v) != 0 {
		t.Fatalf("103200/12 debe equivaler a prima mensual 8600: %v", v)
	}
}

func TestValidarPlanMapfre_PrimaTotalSinPlazoNoPasa(t *testing.T) {
	values := map[string]string{
		"plan_name":       "PLAN 1",
		"monthly_premium": "103200",
		"insured_amount":  "5000000",
	}
	v := validarPlanMapfre("MAPFRE_VIDA", values)
	if len(v) != 1 {
		t.Fatalf("sin plazo no debe aceptar prima total: %v", v)
	}
}

func TestMapfrePrimaCoincideTarifa(t *testing.T) {
	if !mapfrePrimaCoincideTarifa(103200, 8600, 12) {
		t.Fatal("total/plazo debe coincidir")
	}
	if !mapfrePrimaCoincideTarifa(8600, 8600, 12) {
		t.Fatal("mensual directa debe coincidir")
	}
	if mapfrePrimaCoincideTarifa(103200, 8600, 0) {
		t.Fatal("sin plazo no debe dividir")
	}
}

func TestValidarPlanMapfre_CancerPlan2Prima13000(t *testing.T) {
	values := map[string]string{
		"plan_name":       "PLAN 2",
		"monthly_premium": "13000",
		"insured_amount":  "10000000",
	}
	if v := validarPlanMapfre("MAPFRE_CANCER", values); len(v) != 0 {
		t.Fatalf("cancer plan 2 prima 13000 + 10M: %v", v)
	}
}

// TestValidarPlanMapfre_AccMenPrimaTotalArchivoJunioVF: fila real de
// «ACC MEN RM-INCLUSION JUNIO 2026 VF»: prima total 374400 ÷ plazo 48 = 7800
// (Plan 1 base, valor asegurado 5M). Debe pasar sin novedad.
func TestValidarPlanMapfre_AccMenPrimaTotalArchivoJunioVF(t *testing.T) {
	values := map[string]string{
		"plan_name":           "Plan 1",
		"monthly_premium":     "374400",
		"initial_term_months": "48",
		"insured_amount":      "5000000",
	}
	if v := validarPlanMapfre("MAPFRE_ACC_MEN", values); len(v) != 0 {
		t.Fatalf("AP menores 374400/48=7800 debe pasar limpio: %v", v)
	}
}

// TestValidarPlanMapfre_CancerPrimaTotalArchivoJunioVF: fila real de
// «CANCER RM-INCLUSION JUNIO 2026 VF»: prima total 145350 ÷ plazo 18 = 8075
// (Plan 1 con %, valor asegurado 7M). Debe pasar sin novedad.
func TestValidarPlanMapfre_CancerPrimaTotalArchivoJunioVF(t *testing.T) {
	values := map[string]string{
		"plan_name":           "Plan 1",
		"monthly_premium":     "145350",
		"initial_term_months": "18",
		"insured_amount":      "7000000",
	}
	if v := validarPlanMapfre("MAPFRE_CANCER", values); len(v) != 0 {
		t.Fatalf("Cáncer 145350/18=8075 debe pasar limpio: %v", v)
	}
}

// TestValidarPlanMapfre_VidaPrimaTotalArchivoJunioVF: fila real de
// «VOL RM-INCLUSION JUNIO 2026 VF»: prima total 206400 ÷ plazo 24 = 8600
// (Plan 1, valor asegurado 5M). Debe pasar sin novedad.
func TestValidarPlanMapfre_VidaPrimaTotalArchivoJunioVF(t *testing.T) {
	values := map[string]string{
		"plan_name":           "Plan 1",
		"monthly_premium":     "206400",
		"initial_term_months": "24",
		"insured_amount":      "5000000",
	}
	if v := validarPlanMapfre("MAPFRE_VIDA", values); len(v) != 0 {
		t.Fatalf("Vida 206400/24=8600 debe pasar limpio: %v", v)
	}
}
