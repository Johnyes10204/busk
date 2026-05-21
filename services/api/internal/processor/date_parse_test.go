package processor

import (
	"testing"
	"time"
)

func TestParseNumericDateWithOrder(t *testing.T) {
	cases := []struct {
		in       string
		order    dateFieldOrder
		wantDay  int
		wantMon  time.Month
		wantYear int
	}{
		{"31/12/2024", dateOrderDMY, 31, time.December, 2024},
		{"12/31/2024", dateOrderMDY, 31, time.December, 2024},
		{"05-06-2024", dateOrderDMY, 5, time.June, 2024},
		{"05-06-2024", dateOrderMDY, 6, time.May, 2024},
		{"15/03/2024", dateOrderDMY, 15, time.March, 2024},
		{"17/11/1963", dateOrderDMY, 17, time.November, 1963},
		{"17/11/63", dateOrderDMY, 17, time.November, 1963},
	}
	for _, tc := range cases {
		got, ok := parseNumericDateWithContext(tc.in, tc.order, dateYearContextBirth)
		if !ok {
			t.Fatalf("parse %q order=%v: ok=false", tc.in, tc.order)
		}
		if got.Day() != tc.wantDay || got.Month() != tc.wantMon || got.Year() != tc.wantYear {
			t.Fatalf("parse %q: got %v want %d-%d-%d", tc.in, got, tc.wantYear, tc.wantMon, tc.wantDay)
		}
	}
}

func TestParseDate17Nov1963(t *testing.T) {
	layouts := defaultDateLayouts()
	for _, raw := range []string{
		"17/11/1963",
		"17 / 11 / 1963",
		"17-11-1963",
		"17/11/1963 0:00:00",
		"'17/11/1963",
		"17/11/63",
	} {
		if !dateParsedWithOrder(raw, layouts, dateOrderDMY) {
			t.Fatalf("%q debe ser válido como DMY (17 nov 1963)", raw)
		}
		got := parseDateWithLayouts(raw, layouts)
		if got.IsZero() || got.Day() != 17 || got.Month() != time.November || got.Year() != 1963 {
			t.Fatalf("parseDateWithLayouts %q: got %v", raw, got)
		}
	}
}

func TestResolveYearFourDigitsUnchanged(t *testing.T) {
	if resolveYear(1963, dateYearContextBirth) != 1963 {
		t.Fatal("año 1963 no debe cambiar")
	}
	if resolveYear(2026, dateYearContextVigencia) != 2026 {
		t.Fatal("año 2026 no debe cambiar")
	}
	if resolveYear(2031, dateYearContextVigencia) != 2031 {
		t.Fatal("año 2031 no debe cambiar")
	}
}

func TestParseDateFourDigitYear(t *testing.T) {
	layouts := defaultDateLayouts()
	raw := "04-23-2026"
	tm := parseDateField(raw, layouts, dateYearContextVigencia)
	if tm.Year() != 2026 || tm.Month() != time.April || tm.Day() != 23 {
		t.Fatalf("got %v", tm)
	}
	if FormatDateCanonical(tm) != "2026-04-23" {
		t.Fatalf("canonical %s", FormatDateCanonical(tm))
	}
}

func TestNormalizeDateRaw(t *testing.T) {
	if got := normalizeDateRaw(" 17 / 11 / 1963 "); got != "17/11/1963" {
		t.Fatalf("got %q", got)
	}
}

func TestParseDate111763MDY(t *testing.T) {
	layouts := defaultDateLayouts()
	raw := "11-17-63"
	if !dateParsedWithOrder(raw, layouts, dateOrderMDY) {
		t.Fatal("11-17-63 debe ser válido como mes/día/año (17 nov 1963)")
	}
	got := parseDateWithLayouts(raw, layouts)
	if got.Day() != 17 || got.Month() != time.November || got.Year() != 1963 {
		t.Fatalf("got %v", got)
	}
}

func TestParseDateUnicodeHyphen(t *testing.T) {
	layouts := defaultDateLayouts()
	raw := "11\u201317\u201363" // guiones Excel (en dash)
	if !fechaNacimientoParseable(raw, layouts, false) {
		t.Fatalf("no parsea con guion unicode, norm=%q", normalizeDateRaw(raw))
	}
}

func TestParseDateWithLayouts_ISOAndSlash(t *testing.T) {
	layouts := defaultDateLayouts()
	if got := parseDateWithLayouts("2024-03-15", layouts); got.IsZero() || got.Day() != 15 || got.Month() != time.March {
		t.Fatalf("ISO: %v", got)
	}
	if got := parseDateWithLayouts("15/03/2024", layouts); got.IsZero() || got.Day() != 15 {
		t.Fatalf("DMY slash: %v", got)
	}
	if got := parseDateWithLayouts("03/15/2024", layouts); got.IsZero() || got.Month() != time.March || got.Day() != 15 {
		t.Fatalf("MDY slash: %v", got)
	}
	if got := parseDateWithLayouts("11-17-63", layouts); got.IsZero() || got.Year() != 1963 || got.Month() != time.November || got.Day() != 17 {
		t.Fatalf("11-17-63: got %v want 17 nov 1963", got)
	}
}

func TestMapfreFinVigenciaPlazo_042326_60meses(t *testing.T) {
	layouts := defaultDateLayouts()
	diff, ok := mapfreFinVigenciaPlazoCoherente("04-23-26", "60", "04-23-31", layouts, 2)
	if !ok {
		t.Fatalf("04-23-26 + 60m debe cuadrar con 04-23-31 (MDY abr 2026→2031): diff=%d", diff)
	}
}

func TestMapfreFinVigenciaPlazoCoherente_DMYThenMDY(t *testing.T) {
	layouts := defaultDateLayouts()
	if _, ok := mapfreFinVigenciaPlazoCoherente("04-07-26", "10", "02-07-27", layouts, 2); !ok {
		t.Fatal("debe aceptar interpretación mes/día/año cuando día/mes/año no cuadra")
	}
	if _, ok := mapfreFinVigenciaPlazoCoherente("04-07-26", "10", "15-08-27", layouts, 2); ok {
		t.Fatal("debe rechazar cuando ningún orden cumple el plazo")
	}
}

func TestDateParsedWithOrder(t *testing.T) {
	layouts := defaultDateLayouts()
	if !dateParsedWithOrder("15/03/2024", layouts, dateOrderDMY) {
		t.Fatal("15/03/2024 válido como DMY")
	}
	if !dateParsedWithOrder("03/15/2024", layouts, dateOrderMDY) {
		t.Fatal("03/15/2024 válido como MDY")
	}
	if dateParsedWithOrder("32/13/2024", layouts, dateOrderDMY) && dateParsedWithOrder("32/13/2024", layouts, dateOrderMDY) {
		t.Fatal("fecha inválida no debería parsear en ningún orden")
	}
}
