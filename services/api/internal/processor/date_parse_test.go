package processor

import (
	"strings"
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

// TestParseDateRechazaMDY: día/mes/año es el único orden admitido.
// Celdas con mes > 12 (típicas de Excel US: mes/día/año) se consideran inválidas.
func TestParseDateRechazaMDY(t *testing.T) {
	layouts := defaultDateLayouts()
	for _, raw := range []string{"08-14-76", "11-19-21", "10-13-22", "01-31-23", "05-14-26"} {
		if got := parseDateField(raw, layouts, dateYearContextBirth); !got.IsZero() {
			t.Fatalf("%q no debería parsear (mes > 12): got %v", raw, got)
		}
	}
}

func TestParseDateAceptaSoloDMY(t *testing.T) {
	layouts := defaultDateLayouts()
	for _, raw := range []string{"17-11-63", "15-03-26", "27-09-46"} {
		if parseDateField(raw, layouts, dateYearContextBirth).IsZero() {
			t.Fatalf("%q debe parsear como día/mes/año", raw)
		}
	}
}

func TestParseDateFourDigitYear(t *testing.T) {
	layouts := defaultDateLayouts()
	raw := "23-04-2026"
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

func TestParseDate171163DMY(t *testing.T) {
	layouts := defaultDateLayouts()
	raw := "17-11-63"
	if !dateParsedWithOrder(raw, layouts, dateOrderDMY) {
		t.Fatal("17-11-63 debe ser válido como día/mes/año (17 nov 1963)")
	}
	got := parseDateWithLayouts(raw, layouts)
	if got.Day() != 17 || got.Month() != time.November || got.Year() != 1963 {
		t.Fatalf("got %v", got)
	}
}

func TestParseDateUnicodeHyphen(t *testing.T) {
	layouts := defaultDateLayouts()
	raw := "17\u201311\u201363" // guiones Excel (en dash)
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
	if got := parseDateWithLayouts("17-11-63", layouts); got.IsZero() || got.Year() != 1963 || got.Month() != time.November || got.Day() != 17 {
		t.Fatalf("17-11-63: got %v want 17 nov 1963", got)
	}
}

func TestMapfreFinVigenciaPlazo_230426_60meses(t *testing.T) {
	layouts := defaultDateLayouts()
	diff, ok := mapfreFinVigenciaPlazoCoherente("23-04-26", "60", "23-04-31", layouts, 2)
	if !ok {
		t.Fatalf("23-04-26 + 60m debe cuadrar con 23-04-31 (día/mes/año abr 2026→2031): diff=%d", diff)
	}
}

func TestMapfreFinVigenciaPlazoCoherente_OmiteSinDatos(t *testing.T) {
	layouts := defaultDateLayouts()
	cases := []struct {
		name     string
		start    string
		term     string
		end      string
	}{
		{"sin plazo", "23-04-26", "", "23-04-31"},
		{"plazo cero", "23-04-26", "0", "23-04-31"},
		{"sin inicio", "", "60", "23-04-31"},
		{"sin fin", "23-04-26", "60", ""},
		{"fechas no interpretables", "xx", "60", "yy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diff, ok := mapfreFinVigenciaPlazoCoherente(tc.start, tc.term, tc.end, layouts, 2)
			if !ok || diff != 0 {
				t.Fatalf("debe omitir validación: diff=%d ok=%v", diff, ok)
			}
		})
	}
}

func TestMapfreFinVigenciaPlazoCoherente_SoloDiaMesAnio(t *testing.T) {
	layouts := defaultDateLayouts()
	// Día/mes/año: 7-abr-2026 + 10 meses = 7-feb-2027.
	if _, ok := mapfreFinVigenciaPlazoCoherente("07-04-26", "10", "07-02-27", layouts, 2); !ok {
		t.Fatal("debe aceptar plazo coherente (7-abr-2026 + 10m = 7-feb-2027)")
	}
	if _, ok := mapfreFinVigenciaPlazoCoherente("07-04-26", "10", "15-08-27", layouts, 2); ok {
		t.Fatal("debe rechazar cuando el plazo no cuadra")
	}
}

// TestMapfreFinVigenciaPlazoCoherente_ClampingFinDeMes: caso reportado por el cliente
// en «VOL RM-INCLUSION_AGOSTO.xlsx» — pólizas emitidas el 31-ago-2026 con plazo 6 meses
// cierran el 28-feb-2027 (último día de febrero en año no bisiesto). Con la regla vieja
// (Go time.AddDate hacía overflow: 31-ago+6m=3-mar-2027) fallaba con "REVISAR FIN
// VIGENCIA: DIFERENCIA 3 DÍAS". Con clamping fin-de-mes ahora pasa limpio.
func TestMapfreFinVigenciaPlazoCoherente_ClampingFinDeMes(t *testing.T) {
	layouts := defaultDateLayouts()
	cases := []struct {
		name  string
		start string
		term  string
		end   string
	}{
		{"31 ago 2026 + 6 m = 28 feb 2027 (feb no bisiesto)", "31/08/2026", "6", "28/02/2027"},
		{"31 ene 2026 + 1 m = 28 feb 2026", "31/01/2026", "1", "28/02/2026"},
		{"31 ene 2028 + 1 m = 29 feb 2028 (año bisiesto)", "31/01/2028", "1", "29/02/2028"},
		{"31 mar 2026 + 1 m = 30 abr 2026", "31/03/2026", "1", "30/04/2026"},
		{"31 may 2026 + 1 m = 30 jun 2026", "31/05/2026", "1", "30/06/2026"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diff, ok := mapfreFinVigenciaPlazoCoherente(tc.start, tc.term, tc.end, layouts, 10)
			if !ok {
				t.Fatalf("debe aceptar (clamping): diff=%d", diff)
			}
		})
	}
}

// TestMapfreFinVigenciaPlazoCoherente_AceptaOverflowTambien: si un archivo tiene el
// fin de vigencia calculado con overflow de Go (ej. 3-mar en vez de 28-feb) también
// debe pasar — no regresionamos archivos existentes que usan esa interpretación.
func TestMapfreFinVigenciaPlazoCoherente_AceptaOverflowTambien(t *testing.T) {
	layouts := defaultDateLayouts()
	// 31 ago 2026 + 6 m con Go = 31 feb 2027 → 3 mar 2027. Debe seguir pasando.
	if diff, ok := mapfreFinVigenciaPlazoCoherente("31/08/2026", "6", "03/03/2027", layouts, 10); !ok {
		t.Fatalf("debe aceptar overflow Go (compat hacia atrás): diff=%d", diff)
	}
}

// TestAddMonthsClamped: prueba directa del helper con casos de fin de mes.
func TestAddMonthsClamped(t *testing.T) {
	base := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	cases := []struct {
		name   string
		start  time.Time
		months int
		want   time.Time
	}{
		{"31 ago 2026 + 6 = 28 feb 2027", base(2026, time.August, 31), 6, base(2027, time.February, 28)},
		{"31 ene 2028 + 1 = 29 feb 2028 (bisiesto)", base(2028, time.January, 31), 1, base(2028, time.February, 29)},
		{"15 abr 2026 + 12 = 15 abr 2027 (día válido, no clampa)", base(2026, time.April, 15), 12, base(2027, time.April, 15)},
		{"31 dic 2026 + 2 = 28 feb 2027", base(2026, time.December, 31), 2, base(2027, time.February, 28)},
		{"31 may 2026 + 25 = 30 jun 2028 (2 años + 1 mes)", base(2026, time.May, 31), 25, base(2028, time.June, 30)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := addMonthsClamped(tc.start, tc.months)
			if !got.Equal(tc.want) {
				t.Fatalf("addMonthsClamped: got %v want %v", got, tc.want)
			}
		})
	}
}

func TestMesAnteriorAlCargue(t *testing.T) {
	cases := []struct {
		now       time.Time
		wantYear  int
		wantMonth time.Month
	}{
		{time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC), 2026, time.May},
		{time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), 2025, time.December}, // rollover de año
		{time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), 2026, time.November},
	}
	for _, tc := range cases {
		y, m := mesAnteriorAlCargue(tc.now)
		if y != tc.wantYear || m != tc.wantMonth {
			t.Fatalf("now=%v: got %d/%d want %d/%d", tc.now, y, m, tc.wantYear, tc.wantMonth)
		}
	}
}

func TestMensajeInicioVigenciaFueraMesTrabajo_ReportaMesAnterior(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	msg := mensajeInicioVigenciaFueraMesTrabajo("01-06-26", now)
	// Cargado en junio/2026 → mes esperado mayo/2026 → "05/2026".
	if !strings.Contains(msg, "05/2026") {
		t.Fatalf("mensaje debe pedir mes esperado 05/2026: got %q", msg)
	}
	if !strings.Contains(msg, "FUERA DEL MES ESPERADO") {
		t.Fatalf("mensaje con etiqueta nueva: got %q", msg)
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
