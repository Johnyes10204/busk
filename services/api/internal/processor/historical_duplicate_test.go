package processor

import (
	"testing"

	"github.com/buskseguros-design/services/api/internal/model"
)

func policyWithNotes(notes ...string) model.PolicyRecord {
	b := "["
	for i, n := range notes {
		if i > 0 {
			b += ","
		}
		b += `"` + n + `"`
	}
	b += "]"
	return model.PolicyRecord{ValidationJSON: b, PolicyStatus: "ACTIVE"}
}

func TestPoliciesAllBlockingAreHistoricalDuplicate_TodasYaProcesadas(t *testing.T) {
	dup := noteIncidencia(mensajeCreditoDuplicadoHistorico())
	policies := []model.PolicyRecord{
		policyWithNotes(dup),
		policyWithNotes(dup, noteInformativo("aviso menor")),
	}
	if !policiesAllBlockingAreHistoricalDuplicate(policies) {
		t.Fatalf("todas las filas bloquean solo por YA PROCESADO: se esperaba true")
	}
}

func TestPoliciesAllBlockingAreHistoricalDuplicate_MezclaFilaLimpia(t *testing.T) {
	dup := noteIncidencia(mensajeCreditoDuplicadoHistorico())
	policies := []model.PolicyRecord{
		policyWithNotes(dup),
		{PolicyStatus: "ACTIVE"}, // fila nueva sin bloqueo — el file-level gate la rechazaría
	}
	if policiesAllBlockingAreHistoricalDuplicate(policies) {
		t.Fatalf("hay contenido nuevo mezclado: no debe silenciarse")
	}
}

func TestPoliciesAllBlockingAreHistoricalDuplicate_MezclaOtraIncidencia(t *testing.T) {
	dup := noteIncidencia(mensajeCreditoDuplicadoHistorico())
	otro := noteIncidencia("REVISAR PRIMA (PLAN)")
	policies := []model.PolicyRecord{
		policyWithNotes(dup),
		policyWithNotes(otro),
	}
	if policiesAllBlockingAreHistoricalDuplicate(policies) {
		t.Fatalf("si hay otra incidencia bloqueante debe devolver false")
	}
}

func TestPoliciesAllBlockingAreHistoricalDuplicate_SinBloqueos(t *testing.T) {
	policies := []model.PolicyRecord{
		{PolicyStatus: "ACTIVE"},
		policyWithNotes(noteInformativo("solo aviso")),
	}
	if policiesAllBlockingAreHistoricalDuplicate(policies) {
		t.Fatalf("sin filas bloqueantes debe devolver false (no aplica el corto-circuito)")
	}
}

func TestPolicyRowBlockingIsOnlyHistoricalDuplicate_ManualReviewNoCuenta(t *testing.T) {
	dup := noteIncidencia(mensajeCreditoDuplicadoHistorico())
	p := policyWithNotes(dup)
	p.PolicyStatus = "MANUAL_REVIEW"
	if policyRowBlockingIsOnlyHistoricalDuplicate(&p) {
		t.Fatalf("MANUAL_REVIEW es un bloqueo estructural: no puede ser 'solo YA PROCESADO'")
	}
}
