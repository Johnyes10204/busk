package processor

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var bolivarAnioEnNombreArchivo = regexp.MustCompile(`(?:19|20)\d{2}`)

// parseBolivarRateNumber lee la columna % sin reglas de miles (0.001 no debe convertirse en 0.01).
func parseBolivarRateNumber(raw string) float64 {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, "%", "")
	s = strings.ReplaceAll(s, "\u00a0", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, ",", ".")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0
	}
	return f
}

// bolivarTasaFactorDesdeRaw convierte el valor de la columna % al factor del diagrama (E.1).
func bolivarTasaFactorDesdeRaw(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	hasPctSymbol := strings.Contains(raw, "%")
	n := parseBolivarRateNumber(raw)
	if n <= 0 {
		return 0
	}
	if hasPctSymbol {
		return n / 100
	}
	if n < 0.01 {
		return n
	}
	return n / 100
}

// bolivarPrimaEsperada aplica E.1: PRIMA MENSUAL = DEUDA INICIAL × %.
func bolivarPrimaEsperada(deuda float64, tasaRaw string) (prima float64, factor float64) {
	factor = bolivarTasaFactorDesdeRaw(tasaRaw)
	if deuda <= 0 || factor <= 0 {
		return 0, factor
	}
	return deuda * factor, factor
}

// observacionJustificaDiferenciaBolivar implementa E.4: la observación debe existir para justificar un desvío.
func observacionJustificaDiferenciaBolivar(obs string) bool {
	return strings.TrimSpace(obs) != ""
}

// bolivarPlazoCalculadoMeses — E.2: plazo calculado a partir de fechas de adjudicación y vencimiento.
func bolivarPlazoCalculadoMeses(inicio, fin time.Time) int {
	if inicio.IsZero() || fin.IsZero() || fin.Before(inicio) {
		return 0
	}
	days := fin.Sub(inicio).Hours() / 24
	if days < 0 {
		return 0
	}
	return int(math.Floor(days / 30))
}

func tasaPorcentajeDesdeValues(values map[string]string) string {
	if v := strings.TrimSpace(values["rate_percent"]); v != "" {
		return v
	}
	for _, key := range []string{"%", "% "} {
		if v := strings.TrimSpace(values[key]); v != "" {
			return v
		}
	}
	for key, val := range values {
		if strings.TrimSpace(key) == "%" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

func observacionDesdeValues(values map[string]string) string {
	if v := strings.TrimSpace(values["observacion"]); v != "" {
		return v
	}
	for key, val := range values {
		upper := strings.ToUpper(strings.TrimSpace(key))
		if strings.Contains(upper, "OBSERVACION") {
			if v := strings.TrimSpace(val); v != "" {
				return v
			}
		}
	}
	return ""
}

// bolivarMesMinimoVencimiento devuelve el año/mes mínimo permitido para FECHA VENCIMIENTO ACTUAL (solo mes, no día).
// offset -1: en mayo se carga abril (mes vencido); 0: mes calendario del procesamiento.
var bolivarMesesNombre = map[string]time.Month{
	"ENERO": time.January, "FEBRERO": time.February, "MARZO": time.March, "ABRIL": time.April,
	"MAYO": time.May, "JUNIO": time.June, "JULIO": time.July, "AGOSTO": time.August,
	"SEPTIEMBRE": time.September, "SETIEMBRE": time.September, "OCTUBRE": time.October,
	"NOVIEMBRE": time.November, "DICIEMBRE": time.December,
}

// bolivarMesFacturacionDesdeArchivo extrae mes/año de facturación del nombre (ej. MICRO_BANCO_ABRIL_VF → abril 2026).
func bolivarMesFacturacionDesdeArchivo(fileName string) (year int, month time.Month, ok bool) {
	upper := strings.ToUpper(strings.TrimSpace(fileName))
	var m time.Month
	for name, mo := range bolivarMesesNombre {
		if strings.Contains(upper, name) {
			m = mo
			break
		}
	}
	if m == 0 {
		return 0, 0, false
	}
	year = time.Now().UTC().Year()
	if match := bolivarAnioEnNombreArchivo.FindString(upper); match != "" {
		if y, err := strconv.Atoi(match); err == nil {
			year = y
		}
	}
	return year, m, true
}

func bolivarMesMinimoVencimiento(now time.Time, offsetMonths int) (year int, month time.Month) {
	ref := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if offsetMonths != 0 {
		ref = ref.AddDate(0, offsetMonths, 0)
	}
	return ref.Year(), ref.Month()
}

// bolivarMesReferenciaE10: mes de facturación del lote (ABRIL en el nombre del archivo), no el mes calendario de carga (MAYO).
// En mayo se sube el archivo de abril → referencia = abril. Solo si el nombre no trae mes, se usa mes vencido (now-1).
func bolivarMesReferenciaE10(values map[string]string, cfg ruleRuntimeConfig, now time.Time) (year int, month time.Month) {
	if fn := strings.TrimSpace(values["_file_name"]); fn != "" {
		if y, m, ok := bolivarMesFacturacionDesdeArchivo(fn); ok {
			return y, m
		}
	}
	return bolivarMesMinimoVencimiento(now, cfg.BolivarDueReferenceMonthOffset)
}

// bolivarAplicaIncidenciaEdadFueraRango: diagrama E.8 — novedad de edad solo si deuda > umbral (p. ej. 20M).
// Con deuda menor o igual al umbral, la fila puede continuar aunque la edad esté fuera del rango 18–75.
func bolivarAplicaIncidenciaEdadFueraRango(values map[string]string, cfg ruleRuntimeConfig) bool {
	umbral := cfg.BolivarDebtManualThreshold
	if umbral <= 0 {
		return true
	}
	deuda, _ := parseFlexibleNumber(values["initial_debt_amount"])
	return deuda > umbral
}

func bolivarVencimientoAntesMesMinimo(due time.Time, minYear int, minMonth time.Month) bool {
	if due.IsZero() {
		return false
	}
	if due.Year() < minYear {
		return true
	}
	return due.Year() == minYear && due.Month() < minMonth
}

// bolivarEvaluarVencimientoInferiorPrima: si el vencimiento es anterior al mes de facturación del archivo,
// prima > 0 → incidencia (revisar prima); prima = 0 → nota informativa (cancelación en processor, sin importar la fecha).
func bolivarEvaluarVencimientoInferiorPrima(
	values map[string]string,
	parsed map[string]bolivarFechaParse,
	minYear int,
	minMonth time.Month,
) (hardViolations []string, softNotes []string) {
	due := bolivarFechaDesdeParseMap(parsed, "loan_due_date_current")
	if due.IsZero() || !bolivarVencimientoAntesMesMinimo(due, minYear, minMonth) {
		return nil, nil
	}
	prima, _ := parseFlexibleNumber(values["monthly_premium"])
	if prima == 0 {
		// Cancelación por prima 0 la marca processor; no nota en informe solo por vencimiento.
		return nil, nil
	}
	// E.10 operativo: aviso en hoja «Informes» del Excel; no bloquea la importación de pólizas.
	softNotes = append(softNotes, mensajeBolivarRevisarPrimaVencimientoInferior(values, due, minYear, minMonth, prima))
	return nil, softNotes
}

// applyBolivarDiagramRules aplica E.1, E.4, E.5 y E.10 del diagrama PDF.
// E.3 (plazo calculado vs prima) no se implementa: el PDF no define la fórmula de esa columna.
// E.6–E.8 (edad) y E.9 (OP BT) se aplican en applyDiagramRules.
func applyBolivarDiagramRules(
	values map[string]string,
	cfg ruleRuntimeConfig,
) (hardViolations []string, softNotes []string) {
	layouts := cfg.DateLayouts

	fechaHards, parsed := bolivarValidarFechasCreditoInclusion(values, layouts)
	hardViolations = append(hardViolations, fechaHards...)

	now := time.Now().UTC()
	minYear, minMonth := bolivarMesReferenciaE10(values, cfg, now)

	// E.10 operativo: vencimiento anterior al mes de facturación → revisar prima; prima 0 → cancelación (processor), sin incidencia por fecha.
	if cfg.BolivarValidateDueMonth {
		vh, vs := bolivarEvaluarVencimientoInferiorPrima(values, parsed, minYear, minMonth)
		hardViolations = append(hardViolations, vh...)
		softNotes = append(softNotes, vs...)
	}

	adj := bolivarFechaDesdeParseMap(parsed, "loan_award_date", "activation_date")
	due := bolivarFechaDesdeParseMap(parsed, "loan_due_date_current")
	if !adj.IsZero() && !due.IsZero() && due.Before(adj) {
		hardViolations = append(hardViolations, mensajeVencimientoMenorAdjudicacion(values, adj, due))
	}

	obs := observacionDesdeValues(values)
	justifica := observacionJustificaDiferenciaBolivar(obs)

	// E.1 — PRIMA MENSUAL = DEUDA INICIAL × %.
	tasaRaw := tasaPorcentajeDesdeValues(values)
	deuda, _ := parseFlexibleNumber(values["initial_debt_amount"])
	prima, _ := parseFlexibleNumber(values["monthly_premium"])
	if deuda > 0 && strings.TrimSpace(tasaRaw) != "" && prima > 0 {
		primaCalc, factor := bolivarPrimaEsperada(deuda, tasaRaw)
		if math.Abs(primaCalc-prima) > cfg.BolivarPrimaCalcTolerance {
			if !justifica {
				hardViolations = append(hardViolations, mensajePrimaCalculadaDifiere(values, prima, primaCalc, deuda, factor, tasaRaw))
			} else {
				softNotes = append(softNotes, mensajePrimaCalculadaDifiereJustificada(values, prima, primaCalc, obs))
			}
		}
	}

	// E.2 + E.5 (Nota 2) — plazo calculado y diferencia en días respecto al fin de plazo esperado.
	if !adj.IsZero() && !due.IsZero() {
		plazoCalc := bolivarPlazoCalculadoMeses(adj, due)
		finEsperado := adj.AddDate(0, plazoCalc, 0)
		diffFinDias := int(due.Sub(finEsperado).Hours() / 24)
		if diffFinDias != 0 {
			tol := cfg.BolivarPlazoDiasTolerance
			if tol <= 0 {
				tol = 0
			}
			if math.Abs(float64(diffFinDias)) > float64(tol) {
				if !justifica {
					hardViolations = append(hardViolations, mensajePlazoFinVigenciaIncoherente(values, diffFinDias, plazoCalc, adj, due))
				} else {
					softNotes = append(softNotes, mensajePlazoFinVigenciaJustificado(values, diffFinDias, obs))
				}
			}
		}
	}

	return hardViolations, softNotes
}
