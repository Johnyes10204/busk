package processor

import (
	"strconv"
	"strings"
	"time"
)

// bolivarFechaParse resultado al leer fechas de crédito en inclusiones (adjudicación, vencimiento).
type bolivarFechaParse struct {
	Fecha   time.Time
	Ambiguo bool
	DMY     time.Time
	MDY     time.Time
	Raw     string
}

// bolivarFechaNumericaAmbigua: ambas partes ≤ 12 → válido como día/mes y mes/día (ej. 03-06-26, 07-12-26).
func bolivarFechaNumericaAmbigua(raw string) bool {
	s := normalizeDateRaw(raw)
	m := numericDatePattern.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	a, err1 := strconv.Atoi(m[1])
	b, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil {
		return false
	}
	return a <= 12 && b <= 12
}

// parseBolivarFechaInclusion interpreta fechas de inclusiones Bolívar (archivos tipo MICRO_BANCO).
// Convención: mes/día/año (MDY) como en Excel US; si día/mes y mes/día difieren se usa MDY sin nota en el informe.
func parseBolivarFechaInclusion(raw string, layouts []string) bolivarFechaParse {
	raw = strings.TrimSpace(raw)
	out := bolivarFechaParse{Raw: raw}
	if raw == "" {
		return out
	}
	norm := normalizeDateRaw(raw)
	if t, ok := parseExcelSerialDateString(norm); ok {
		out.Fecha = t
		return out
	}
	for _, l := range layouts {
		if isYearFirstDateLayout(l) {
			if t, err := time.ParseInLocation(l, norm, time.UTC); err == nil {
				out.Fecha = t.UTC()
				return out
			}
		}
	}
	out.DMY = parseDateFieldOrder(raw, layouts, dateOrderDMY, dateYearContextVigencia)
	out.MDY = parseDateFieldOrder(raw, layouts, dateOrderMDY, dateYearContextVigencia)
	if !out.DMY.IsZero() && !out.MDY.IsZero() && !sameCalendarDay(out.DMY, out.MDY) {
		out.Ambiguo = true
		// Inclusiones Bolívar (MICRO_BANCO): convención mes/día/año; se registra nota con ambas lecturas.
		if !out.MDY.IsZero() {
			out.Fecha = out.MDY
		}
		return out
	}
	if !out.MDY.IsZero() {
		out.Fecha = out.MDY
		return out
	}
	if !out.DMY.IsZero() {
		out.Fecha = out.DMY
	}
	return out
}

func bolivarValidarFechasCreditoInclusion(
	values map[string]string,
	layouts []string,
) (hard []string, parsed map[string]bolivarFechaParse) {
	parsed = make(map[string]bolivarFechaParse)
	checks := []struct {
		canon string
		raw   string
		store string
	}{
		{"loan_award_date", strings.TrimSpace(values["loan_award_date"]), "loan_award_date"},
		{"loan_award_date", strings.TrimSpace(values["activation_date"]), "activation_date"},
		{"loan_due_date_current", strings.TrimSpace(values["loan_due_date_current"]), "loan_due_date_current"},
	}
	for _, c := range checks {
		if c.raw == "" {
			continue
		}
		p := parseBolivarFechaInclusion(c.raw, layouts)
		parsed[c.store] = p
		if p.Fecha.IsZero() && bolivarFechaNumericaAmbigua(c.raw) {
			hard = append(hard, mensajeFechaBolivarNoInterpretable(values, c.canon))
		} else if p.Fecha.IsZero() {
			hard = append(hard, mensajeFechaBolivarInvalida(values, c.canon))
		}
	}
	return hard, parsed
}

func bolivarFechaDesdeParseMap(parsed map[string]bolivarFechaParse, keys ...string) time.Time {
	for _, k := range keys {
		if p, ok := parsed[k]; ok && !p.Fecha.IsZero() {
			return p.Fecha
		}
	}
	return time.Time{}
}
