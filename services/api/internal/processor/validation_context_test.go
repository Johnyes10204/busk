package processor

import (
	"strings"
	"testing"
)

func TestMensajePrimaCalculadaDifiere_Corto(t *testing.T) {
	values := map[string]string{
		"monthly_premium":       "25000",
		"initial_debt_amount":   "25000000",
		"rate_percent":          "0.001",
	}
	msg := mensajePrimaCalculadaDifiere(values, 25000, 30000, 25_000_000, 0.001, "0.001")
	if !strings.Contains(msg, "REVISAR PRIMA") {
		t.Fatalf("mensaje corto: %s", msg)
	}
	if strings.Contains(msg, "columna «") {
		t.Fatalf("no debe incluir contexto largo: %s", msg)
	}
}
