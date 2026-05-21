package processor

import (
	"testing"
	"time"
)

func TestParseBolivarFechaInclusion_MDYJulio12(t *testing.T) {
	layouts := defaultDateLayouts()
	p := parseBolivarFechaInclusion("07-12-26", layouts)
	if !p.Ambiguo {
		t.Fatal("07-12-26 es ambiguo (7-dic vs 12-jul)")
	}
	if p.Fecha.Month() != time.July || p.Fecha.Day() != 12 || p.Fecha.Year() != 2026 {
		t.Fatalf("convención MDY: got %v", p.Fecha)
	}
}

func TestParseBolivarFechaInclusion_Ambiguo030626UsaMDY(t *testing.T) {
	layouts := defaultDateLayouts()
	p := parseBolivarFechaInclusion("03-06-26", layouts)
	if !p.Ambiguo {
		t.Fatalf("03-06-26 es ambiguo: dmy=%v mdy=%v", p.DMY, p.MDY)
	}
	if p.Fecha.Month() != time.March || p.Fecha.Day() != 6 {
		t.Fatalf("MDY marzo 6: got %v (DMY sería junio 3)", p.Fecha)
	}
}

func TestParseBolivarFechaInclusion_SoloDMY15Marzo(t *testing.T) {
	layouts := defaultDateLayouts()
	p := parseBolivarFechaInclusion("15-03-26", layouts)
	if p.Ambiguo {
		t.Fatal("15-03-26 solo válido DMY")
	}
	if p.Fecha.Month() != time.March || p.Fecha.Day() != 15 {
		t.Fatalf("got %v", p.Fecha)
	}
}

func TestBolivarValidarFechasAmbiguaSinNotaEnInforme(t *testing.T) {
	values := map[string]string{
		"loan_due_date_current": "03-06-26",
		"monthly_premium":       "1000",
	}
	hard, parsed := bolivarValidarFechasCreditoInclusion(values, defaultDateLayouts())
	if len(parsed) == 0 {
		t.Fatal("expected parsed entry")
	}
	if len(hard) > 0 {
		t.Fatalf("ambiguo resuelto con MDY no debe reportarse en archivo: %v", hard)
	}
}
