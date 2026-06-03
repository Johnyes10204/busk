package processor

import (
	"testing"
	"time"
)

func TestParseBolivarFechaInclusion_MDY_Enero20(t *testing.T) {
	layouts := defaultDateLayouts()
	p := parseBolivarFechaInclusion("01-20-22", layouts)
	if p.Fecha.Month() != time.January || p.Fecha.Day() != 20 || p.Fecha.Year() != 2022 {
		t.Fatalf("archivo MICRO_BANCO: got %v", p.Fecha)
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

func TestBolivarValidarFechasAmbiguaSinInforme(t *testing.T) {
	// Fechas ambiguas en Bolívar se resuelven silenciosamente con MDY; no generan informe de ruido.
	values := map[string]string{
		"loan_due_date_current": "03-06-26",
		"monthly_premium":       "1000",
	}
	hard, soft, parsed := bolivarValidarFechasCreditoInclusion(values, defaultDateLayouts())
	if len(parsed) == 0 {
		t.Fatal("expected parsed entry")
	}
	if len(hard) > 0 {
		t.Fatalf("ambiguo resuelto con MDY no debe ser incidencia: %v", hard)
	}
	if len(soft) != 0 {
		t.Fatalf("fecha ambigua resuelta no debe generar informe: %v", soft)
	}
}

func TestBolivarValidarFechas_AdjConActivationNoDuplica(t *testing.T) {
	// loan_award_date y activation_date apuntan a la misma columna: no debe generar mensajes duplicados ni de ambigüedad.
	values := map[string]string{
		"loan_award_date": "05-11-23",
		"activation_date": "05-11-23",
	}
	hard, soft, _ := bolivarValidarFechasCreditoInclusion(values, defaultDateLayouts())
	if len(hard) > 0 {
		t.Fatalf("no debe generar incidencia: %v", hard)
	}
	if len(soft) != 0 {
		t.Fatalf("fecha ambigua resuelta no debe generar informe: %v", soft)
	}
}

func TestBolivarVigenciaFecha_MismaQueDiagrama(t *testing.T) {
	layouts := defaultDateLayouts()
	raw := "05-15-26"
	got := bolivarVigenciaFecha(raw, layouts)
	if got.Format("2006-01-02") != "2026-05-15" {
		t.Fatalf("bolivarVigenciaFecha=%v", got)
	}
	p := parseBolivarFechaInclusion(raw, layouts)
	if !sameCalendarDay(got, p.Fecha) {
		t.Fatalf("edad y diagrama deben usar la misma fecha: vig=%v parse=%v", got, p.Fecha)
	}
}
