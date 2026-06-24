package processor

import (
	"testing"
	"time"
)

func TestParseBolivarFechaInclusion_DMY_Enero20(t *testing.T) {
	layouts := defaultDateLayouts()
	p := parseBolivarFechaInclusion("20-01-22", layouts)
	if p.Fecha.Month() != time.January || p.Fecha.Day() != 20 || p.Fecha.Year() != 2022 {
		t.Fatalf("día/mes/año: got %v", p.Fecha)
	}
}

func TestParseBolivarFechaInclusion_030626EsDMY(t *testing.T) {
	layouts := defaultDateLayouts()
	// Día/mes/año es el único orden admitido: 03-06-26 = 3 de junio de 2026.
	p := parseBolivarFechaInclusion("03-06-26", layouts)
	if p.Fecha.Month() != time.June || p.Fecha.Day() != 3 || p.Fecha.Year() != 2026 {
		t.Fatalf("día/mes/año junio 3: got %v", p.Fecha)
	}
}

func TestParseBolivarFechaInclusion_SoloDMY15Marzo(t *testing.T) {
	layouts := defaultDateLayouts()
	p := parseBolivarFechaInclusion("15-03-26", layouts)
	if p.Fecha.Month() != time.March || p.Fecha.Day() != 15 {
		t.Fatalf("got %v", p.Fecha)
	}
}

func TestBolivarValidarFechasAmbiguaSinInforme(t *testing.T) {
	// Fechas ambiguas se resuelven con día/mes/año; no generan informe de ruido.
	values := map[string]string{
		"loan_due_date_current": "03-06-26",
		"monthly_premium":       "1000",
	}
	hard, soft, parsed := bolivarValidarFechasCreditoInclusion(values, defaultDateLayouts())
	if len(parsed) == 0 {
		t.Fatal("expected parsed entry")
	}
	if len(hard) > 0 {
		t.Fatalf("ambiguo resuelto con DMY no debe ser incidencia: %v", hard)
	}
	if len(soft) != 0 {
		t.Fatalf("fecha ambigua resuelta no debe generar informe: %v", soft)
	}
}

func TestBolivarValidarFechas_AdjConActivationNoDuplica(t *testing.T) {
	// loan_award_date y activation_date apuntan a la misma columna: no debe generar mensajes duplicados ni de ambigüedad.
	values := map[string]string{
		"loan_award_date":       "05-11-23",
		"activation_date":       "05-11-23",
		"loan_due_date_current": "05-11-28",
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
	raw := "15-05-26"
	got := bolivarVigenciaFecha(raw, layouts)
	if got.Format("2006-01-02") != "2026-05-15" {
		t.Fatalf("bolivarVigenciaFecha=%v", got)
	}
	p := parseBolivarFechaInclusion(raw, layouts)
	if !sameCalendarDay(got, p.Fecha) {
		t.Fatalf("edad y diagrama deben usar la misma fecha: vig=%v parse=%v", got, p.Fecha)
	}
}
