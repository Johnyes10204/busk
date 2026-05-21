package processor

import (
	"strings"
	"testing"
)

func TestMensajePrimaCalculadaDifiere_IncluyeColumnas(t *testing.T) {
	values := map[string]string{
		"_hdr_monthly_premium":    "PRIMA MENSUAL",
		"monthly_premium":         "25000",
		"_hdr_initial_debt_amount": "DEUDA INICIAL",
		"initial_debt_amount":   "25000000",
		"_hdr_rate_percent":       "%",
		"rate_percent":            "0.001",
	}
	msg := mensajePrimaCalculadaDifiere(values, 25000, 30000, 25_000_000, 0.001, "0.001")
	if !strings.Contains(msg, "columna «PRIMA MENSUAL»") {
		t.Fatalf("falta header prima: %s", msg)
	}
	if !strings.Contains(msg, "columna «DEUDA INICIAL»") {
		t.Fatalf("falta header deuda: %s", msg)
	}
	if !strings.Contains(msg, "referencia") {
		t.Fatalf("falta referencia cálculo: %s", msg)
	}
}
