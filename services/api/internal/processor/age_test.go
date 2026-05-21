package processor

import (
	"testing"
	"time"
)

func TestCompletedYearsBetween(t *testing.T) {
	ref := time.Date(2025, 5, 20, 0, 0, 0, 0, time.UTC)
	birth := time.Date(2007, 6, 1, 0, 0, 0, 0, time.UTC)
	if got := completedYearsBetween(birth, ref); got != 17 {
		t.Fatalf("antes del cumpleaños en 2025: got %d want 17", got)
	}
	ref2 := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	if got := completedYearsBetween(birth, ref2); got != 18 {
		t.Fatalf("después del cumpleaños: got %d want 18", got)
	}
}

func TestEdadCumpleRango_DMYThenMDY(t *testing.T) {
	layouts := defaultDateLayouts()
	ref := time.Date(2025, 5, 20, 0, 0, 0, 0, time.UTC)
	// Ambiguo: DMY 17, MDY 18 → acepta si MDY cumple mínimo.
	if ok, _ := edadCumpleRango("01/06/2007", layouts, ref, 18, 75.997); !ok {
		t.Fatal("debe aceptar cuando al menos una interpretación cumple")
	}
	if ok, edad := edadCumpleRango("01/06/2007", layouts, ref, 19, 75); ok || edad != 18 {
		t.Fatalf("fuera de rango mínimo: ok=%v edad=%d (MDY da 18)", ok, edad)
	}
	refOld := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if ok, _ := edadCumpleRango("01/01/2010", layouts, refOld, 18, 75); ok {
		t.Fatal("menor de 18 en cualquier orden debe fallar")
	}
}

func TestAgeLimitIntsFromDB(t *testing.T) {
	if ageLimitMinInt(18) != 18 || ageLimitMaxBirthdayYear(75.997) != 76 || ageLimitMaxBirthdayYear(65.997) != 66 {
		t.Fatal("límites BD: min 18, tope cumpleaños 76 / 66")
	}
}

func TestEdad18Inclusive(t *testing.T) {
	layouts := defaultDateLayouts()
	ref := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	birth := "01/06/2007"
	if ok, edad := edadCumpleRango(birth, layouts, ref, 18, 75.997); !ok || edad != 18 {
		t.Fatalf("18 años cumplidos debe ser válido: ok=%v edad=%d", ok, edad)
	}
	refMenor := time.Date(2025, 5, 20, 0, 0, 0, 0, time.UTC)
	if ok, _ := edadCumpleRango("2008-06-01", layouts, refMenor, 18, 75); ok {
		t.Fatal("menor de 18 no debe cumplir")
	}
}

func TestEdadMax75Anos364Dias(t *testing.T) {
	layouts := defaultDateLayouts()
	// Nació 1-jun-1950 → cumpleaños 76 el 1-jun-2026; negocio: válido hasta 31-may-2026 (día anterior).
	birthRaw := "01/06/1950"
	dias := 1
	refOK := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	if ok, age := edadCumpleRangoEstricto(birthRaw, layouts, refOK, 18, 75.997, dias, false); !ok {
		t.Fatalf("31-may-2026 debe ser válido (1 día antes del cumpleaños 76): edad=%d", age)
	}
	ref76 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if ok, age := edadCumpleRangoEstricto(birthRaw, layouts, ref76, 18, 75.997, dias, false); ok {
		t.Fatalf("cumpleaños 76 debe rechazarse: edad=%d", age)
	}
	refCumplidos76 := time.Date(2027, 6, 2, 0, 0, 0, 0, time.UTC)
	if ok, _ := edadCumpleRangoEstricto(birthRaw, layouts, refCumplidos76, 18, 75.997, dias, false); ok {
		t.Fatal("no debe pasar con 76 años cumplidos")
	}
}

func TestEdadUnDiaAntesCumpleanos76_Jun1950(t *testing.T) {
	layouts := defaultDateLayouts()
	birthRaw := "06-19-50"
	ref := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC) // 1 día antes del 76 (19-jun-2026)
	if ok, age := edadCumpleRangoEstricto(birthRaw, layouts, ref, 18, 75.997, 1, true); !ok || age != 75 {
		t.Fatalf("activación 18-jun-2026 debe ser válida (75 cumplidos): ok=%v edad=%d", ok, age)
	}
}

func TestEdadMapfreInicioVigenciaMDY(t *testing.T) {
	layouts := defaultDateLayouts()
	values := map[string]string{
		"birth_date":          "06-19-50",
		"coverage_start_date": "04-08-26",
	}
	cfg := ruleRuntimeConfig{
		HasAgeLimits:            true,
		AgeMin:                  18,
		AgeMax:                  75.997,
		MapfreDateToleranceDays: 2,
		DateLayouts:             layouts,
	}
	det := evaluarEdadDetalle(values, cfg, true, "MAPFRE_VIDA")
	if !det.refValid {
		t.Fatal("fecha de activación debe parsear")
	}
	if det.ref.Year() != 2026 || det.ref.Month() != time.April || det.ref.Day() != 8 {
		t.Fatalf("activación debe ser 2026-04-08 (MDY), got %s", FormatDateCanonical(det.ref))
	}
	if !det.cumple {
		t.Fatalf("debe cumplir a 8-abr-2026 (75 años en activación, cumple 76 el 19-jun): edad=%d", det.edadReportada)
	}
	if det.edadReportada != 75 {
		t.Fatalf("edad reportada %d, want 75", det.edadReportada)
	}
}

func TestEdadRequiereFechaActivacion(t *testing.T) {
	layouts := defaultDateLayouts()
	cfg := ruleRuntimeConfig{
		HasAgeLimits: true,
		AgeMin:       18,
		AgeMax:       75.997,
		DateLayouts:  layouts,
	}
	det := evaluarEdadDetalle(map[string]string{"birth_date": "01/06/1950"}, cfg, true, "MAPFRE_VIDA")
	if det.activacionRaw != "" || det.refValid {
		t.Fatal("sin fecha de activación no debe haber referencia válida")
	}
}

func TestPlanNombrePermitido(t *testing.T) {
	if !planNombreCoincideLinea("Voluntario PLAN 1", "PLAN 1") {
		t.Fatal("debe aceptar PLAN 1 en el nombre")
	}
	if planNombreCoincideLinea("PLAN 3", "PLAN 1") {
		t.Fatal("PLAN 3 no permitido")
	}
}
