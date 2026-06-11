package processor

import (
	"testing"

	"github.com/buskseguros-design/services/api/internal/store"
)

type mockMapfreStockReader struct {
	match store.MapfreStockMatch
	found bool
}

func (m *mockMapfreStockReader) FindLatestMapfreStockPolicy(creditNumber, documentNumber string) (store.MapfreStockMatch, bool) {
	return m.match, m.found
}

func TestMapfreCancelacionViolaciones_okAbril2026(t *testing.T) {
	cfg := ruleRuntimeConfig{DateLayouts: defaultDateLayouts()}
	values := map[string]string{
		"_file_name":         "Anulacion masiva_ABRIL 2026.xlsx",
		"coverage_end_date":  "46124",
		"activation_date":    "45334",
		"observacion":        "ABRIL",
	}
	got := mapfreCancelacionViolacionesFechas(values, cfg)
	if len(got) != 0 {
		t.Fatalf("esperaba 0 violaciones, got %v", got)
	}
}

func TestMapfreCancelacionViolaciones_mismoDia(t *testing.T) {
	cfg := ruleRuntimeConfig{DateLayouts: defaultDateLayouts()}
	values := map[string]string{
		"_file_name":         "Anulacion masiva_ABRIL 2026.xlsx",
		"coverage_end_date":  "46124",
		"activation_date":    "45509",
		"observacion":        "ABRIL",
	}
	got := mapfreCancelacionViolacionesFechas(values, cfg)
	if len(got) != 1 {
		t.Fatalf("esperaba 1 violación, got %v", got)
	}
}

func TestMapfreCancelacionViolaciones_mesEtiqueta(t *testing.T) {
	cfg := ruleRuntimeConfig{DateLayouts: defaultDateLayouts()}
	values := map[string]string{
		"_file_name":         "Anulacion masiva_ABRIL 2026.xlsx",
		"coverage_end_date":  "46147",
		"activation_date":    "45509",
		"observacion":        "ABRIL",
	}
	got := mapfreCancelacionViolacionesFechas(values, cfg)
	if len(got) != 1 {
		t.Fatalf("esperaba 1 violación, got %v", got)
	}
}

func TestMapfreCancelacionViolaciones_faltanFechas(t *testing.T) {
	cfg := ruleRuntimeConfig{DateLayouts: defaultDateLayouts()}
	if got := mapfreCancelacionViolacionesFechas(map[string]string{}, cfg); len(got) != 1 || got[0] != mensajeCancelacionFaltaFinProyectado() {
		t.Fatalf("fin proyectado: got %v", got)
	}
	values := map[string]string{"coverage_end_date": "46124"}
	if got := mapfreCancelacionViolacionesFechas(values, cfg); len(got) != 1 || got[0] != mensajeCancelacionFaltaFechaActivacion() {
		t.Fatalf("activación: got %v", got)
	}
}

func TestMapfreMesEtiquetaCancelacion_obsYArchivo(t *testing.T) {
	y, m, ok := mapfreMesEtiquetaCancelacion(map[string]string{
		"observacion": "MAYO",
		"_file_name":  "Anulacion masiva_ABRIL 2026.xlsx",
	})
	if !ok || m != 5 || y != 2026 {
		t.Fatalf("obs+archivo: y=%d m=%v ok=%v", y, m, ok)
	}
	y, m, ok = mapfreMesEtiquetaCancelacion(map[string]string{
		"_file_name": "Anulacion masiva_ABRIL 2026.xlsx",
	})
	if !ok || m != 4 || y != 2026 {
		t.Fatalf("solo archivo: y=%d m=%v ok=%v", y, m, ok)
	}
}

func TestMapfreCancelacionViolacionesStock_noEnStock(t *testing.T) {
	cfg := ruleRuntimeConfig{DateLayouts: defaultDateLayouts()}
	values := map[string]string{
		"credit_number":   "1719123",
		"document_number": "6525556",
	}
	got := mapfreCancelacionViolacionesStock(values, cfg, &mockMapfreStockReader{found: false})
	if len(got) != 1 || got[0] != mensajeCancelacionPolizaNoEnStock("1719123") {
		t.Fatalf("got %v", got)
	}
}

func TestMapfreCancelacionViolacionesStock_ok(t *testing.T) {
	cfg := ruleRuntimeConfig{DateLayouts: defaultDateLayouts()}
	values := map[string]string{
		"credit_number":        "1719123",
		"document_number":      "6525556",
		"group_policy_number":  "5024424900108",
		"policy_number":        "5024424900108",
		"activation_date":      "45334",
	}
	mock := &mockMapfreStockReader{
		found: true,
		match: store.MapfreStockMatch{
			DocumentNumber: "6525556",
			GroupPolicy:    "5024424900108",
			ActivationDate: "45334",
		},
	}
	got := mapfreCancelacionViolacionesStock(values, cfg, mock)
	if len(got) != 0 {
		t.Fatalf("esperaba 0 violaciones stock, got %v", got)
	}
}

func TestMapfreCancelacionViolacionesStock_documentoDistinto(t *testing.T) {
	cfg := ruleRuntimeConfig{DateLayouts: defaultDateLayouts()}
	values := map[string]string{
		"credit_number":   "1719123",
		"document_number": "6525556",
	}
	mock := &mockMapfreStockReader{
		found: true,
		match: store.MapfreStockMatch{DocumentNumber: "999"},
	}
	got := mapfreCancelacionViolacionesStock(values, cfg, mock)
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
}

func TestMapfreCancelacionObservacionStock(t *testing.T) {
	cfg := ruleRuntimeConfig{DateLayouts: defaultDateLayouts()}
	obs := mapfreCancelacionObservacionStock(map[string]string{
		"coverage_end_date": "46124",
		"observacion":       "ABRIL",
		"_file_name":        "Anulacion masiva_ABRIL 2026.xlsx",
	}, cfg)
	if obs != "cancelar/04/2026" {
		t.Fatalf("obs=%q", obs)
	}
}
