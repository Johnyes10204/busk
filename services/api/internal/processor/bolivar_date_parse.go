package processor

import (
	"fmt"
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

// parseBolivarFechaInclusion interpreta fechas de crédito/vigencia con deducción automática de orden.
func parseBolivarFechaInclusion(raw string, layouts []string) bolivarFechaParse {
	raw = strings.TrimSpace(raw)
	out := bolivarFechaParse{Raw: raw}
	if raw == "" {
		return out
	}
	out.DMY = parseDateFieldOrder(raw, layouts, dateOrderDMY, dateYearContextVigencia)
	out.MDY = parseDateFieldOrder(raw, layouts, dateOrderMDY, dateYearContextVigencia)
	if !out.DMY.IsZero() && !out.MDY.IsZero() && !sameCalendarDay(out.DMY, out.MDY) {
		out.Ambiguo = true
	}
	out.Fecha = parseDateField(raw, layouts, dateYearContextVigencia)
	return out
}

// bolivarVigenciaFecha misma lectura que E.2/E.10 (adjudicación, vencimiento, activación).
func bolivarVigenciaFecha(raw string, layouts []string) time.Time {
	return parseBolivarFechaInclusion(raw, layouts).Fecha
}

func bolivarValidarFechasCreditoInclusion(
	values map[string]string,
	layouts []string,
) (hard, soft []string, parsed map[string]bolivarFechaParse) {
	parsed = make(map[string]bolivarFechaParse)

	// activation_date es fallback de loan_award_date: solo se valida y reporta si loan_award_date está vacío.
	adjRaw := strings.TrimSpace(values["loan_award_date"])
	actRaw := strings.TrimSpace(values["activation_date"])

	type check struct {
		canon string
		raw   string
		store string
	}
	checks := []check{
		{"loan_due_date_current", strings.TrimSpace(values["loan_due_date_current"]), "loan_due_date_current"},
	}
	if adjRaw != "" {
		checks = append([]check{{"loan_award_date", adjRaw, "loan_award_date"}}, checks...)
		if actRaw != "" {
			parsed["activation_date"] = parseBolivarFechaInclusion(actRaw, layouts)
		}
	} else if actRaw != "" {
		checks = append([]check{{"loan_award_date", actRaw, "activation_date"}}, checks...)
	}

	for _, c := range checks {
		if c.raw == "" {
			hard = append(hard, fmt.Sprintf("FALTA %s", strings.ToUpper(etiquetaCampoCanónico(c.store))))
			continue
		}
		p := parseBolivarFechaInclusion(c.raw, layouts)
		parsed[c.store] = p
		if p.Fecha.IsZero() && bolivarFechaNumericaAmbigua(c.raw) {
			hard = append(hard, mensajeFechaBolivarNoInterpretable(c.raw, c.canon))
		} else if p.Fecha.IsZero() {
			hard = append(hard, mensajeFechaBolivarInvalida(c.raw, c.canon))
		}
	}
	return hard, soft, parsed
}

func bolivarFechaDesdeParseMap(parsed map[string]bolivarFechaParse, keys ...string) time.Time {
	for _, k := range keys {
		if p, ok := parsed[k]; ok && !p.Fecha.IsZero() {
			return p.Fecha
		}
	}
	return time.Time{}
}
