package processor

import (
	"strconv"
	"strings"
	"time"
)

// DateCanonicalLayout formato interno único (ISO 8601 fecha) tras leer cualquier celda Excel.
const DateCanonicalLayout = "2006-01-02"

type dateYearContext int

const (
	dateYearContextBirth     dateYearContext = iota // nacimiento: solo años 2 dígitos se expanden
	dateYearContextVigencia                       // vigencia: años 2 dígitos → 20xx
)

// FormatDateCanonical devuelve la fecha en calendario estándar YYYY-MM-DD.
func FormatDateCanonical(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(DateCanonicalLayout)
}

// resolveYear: si el año ya viene completo (≥100, p. ej. 1963 o 2026), no se modifica.
func resolveYear(y int, ctx dateYearContext) int {
	if y >= 100 {
		return y
	}
	switch ctx {
	case dateYearContextVigencia:
		return 2000 + y
	default:
		if y > 30 {
			return 1900 + y
		}
		return 2000 + y
	}
}

func parseDateNonNumeric(raw string, layouts []string) time.Time {
	s := normalizeDateRaw(raw)
	if s == "" {
		return time.Time{}
	}
	for _, l := range layouts {
		if isYearFirstDateLayout(l) {
			if t, err := time.ParseInLocation(l, s, time.UTC); err == nil {
				return t.UTC()
			}
		}
	}
	if t, ok := parseExcelSerialDateString(s); ok {
		return t
	}
	return time.Time{}
}

// parseDateFieldOrder interpreta una fecha numérica con el orden indicado (DMY o MDY).
func parseDateFieldOrder(raw string, layouts []string, order dateFieldOrder, ctx dateYearContext) time.Time {
	if t := parseDateNonNumeric(raw, layouts); !t.IsZero() {
		return t
	}
	s := normalizeDateRaw(raw)
	if t, ok := parseNumericDateWithContext(s, order, ctx); ok {
		return t
	}
	return time.Time{}
}

// parseDateField interpreta fechas numéricas únicamente como día/mes/año.
// Las celdas con mes > 12 (p. ej. 05-14-26) se consideran inválidas: no se infiere mes/día/año.
func parseDateField(raw string, layouts []string, ctx dateYearContext) time.Time {
	return parseDateFieldOrder(raw, layouts, dateOrderDMY, ctx)
}

func parseNumericDateWithContext(s string, order dateFieldOrder, ctx dateYearContext) (time.Time, bool) {
	m := numericDatePattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return time.Time{}, false
	}
	a, err1 := strconv.Atoi(m[1])
	b, err2 := strconv.Atoi(m[2])
	y, err3 := strconv.Atoi(m[3])
	if err1 != nil || err2 != nil || err3 != nil {
		return time.Time{}, false
	}
	y = resolveYear(y, ctx)
	switch order {
	case dateOrderDMY:
		t := calendarDateUTC(b, a, y)
		return t, !t.IsZero()
	default:
		t := calendarDateUTC(a, b, y)
		return t, !t.IsZero()
	}
}

func dateFieldParseable(raw string, layouts []string, ctx dateYearContext) bool {
	return !parseDateField(raw, layouts, ctx).IsZero()
}

// parseBirthDate interpreta la fecha de nacimiento (regla de edad).
func parseBirthDate(raw string, layouts []string, mapfreSheet bool) time.Time {
	_ = mapfreSheet
	return parseDateField(raw, layouts, dateYearContextBirth)
}

// parseBirthDateOrders devuelve ambas lecturas posibles para informes (DMY y MDY).
func parseBirthDateOrders(raw string, layouts []string, mapfreSheet bool) (dmy, mdy time.Time) {
	ctx := dateYearContextBirth
	_ = mapfreSheet
	dmy = parseDateFieldOrder(raw, layouts, dateOrderDMY, ctx)
	mdy = parseDateFieldOrder(raw, layouts, dateOrderMDY, ctx)
	return dmy, mdy
}

// parseAgeReferenceDate parsea la fecha de activación (edad de ingreso = edad en ese día).
func parseAgeReferenceDate(raw string, layouts []string, mapfreSheet bool, productCode string) time.Time {
	_ = mapfreSheet
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(productCode)), "BOLIVAR") {
		return bolivarVigenciaFecha(raw, layouts)
	}
	return parseDateField(raw, layouts, dateYearContextVigencia)
}

// parseMapfreVigenciaDate: inicio/fin de vigencia y plazo.
func parseMapfreVigenciaDate(raw string, layouts []string) time.Time {
	return parseDateField(raw, layouts, dateYearContextVigencia)
}
