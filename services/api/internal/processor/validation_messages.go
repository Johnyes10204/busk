package processor

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/buskseguros-design/services/api/internal/model"
	"github.com/buskseguros-design/services/api/internal/validationnotes"
)

func noteIsBlocking(n string) bool {
	return validationnotes.IsBlocking(n)
}

func noteIncidencia(msg string) string {
	return validationnotes.Incidencia(msg)
}

func noteInformativo(msg string) string {
	return validationnotes.Informativo(msg)
}

// etiquetaCampoCanónico traduce campos técnicos a nombres de negocio en español.
func etiquetaCampoCanónico(field string) string {
	switch strings.TrimSpace(field) {
	case "document_number":
		return "Identificación del afiliado"
	case "birth_date":
		return "Fecha de nacimiento"
	case "monthly_premium":
		return "Prima mensual"
	case "credit_number":
		return "Número de crédito u operación (OP BT)"
	case "activation_date":
		return "Fecha de activación"
	case "coverage_start_date":
		return "Fecha de inicio de vigencia"
	case "coverage_end_date":
		return "Fecha de fin de vigencia"
	case "initial_term_months":
		return "Plazo inicial (meses)"
	case "loan_award_date":
		return "Fecha de adjudicación del crédito"
	case "loan_due_date_current":
		return "Fecha de vencimiento actual del crédito"
	case "initial_debt_amount":
		return "Deuda inicial"
	case "plan_name":
		return "Nombre del plan"
	case "plan_code":
		return "Código del plan"
	case "insured_amount":
		return "Valor asegurado"
	default:
		if field == "" {
			return "campo no identificado"
		}
		return field
	}
}

func etiquetaEstadoPoliza(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ACTIVE":
		return "Activa"
	case "FROZEN":
		return "Congelada"
	case "MANUAL_REVIEW":
		return "Revisión manual"
	case "CANCELLED":
		return "Cancelada"
	default:
		if strings.TrimSpace(status) == "" {
			return ""
		}
		return status
	}
}

func mensajeReglaFormato(r model.RuleConfig, values map[string]string, frozen bool) []string {
	var out []string
	c := campoDesdeValues(values, r.Field)
	tipo := strings.ToLower(strings.TrimSpace(r.Type))

	switch tipo {
	case "required_not_empty":
		if c.Valor == "" {
			out = append(out, mensajeConContexto(
				fmt.Sprintf("El dato «%s» es obligatorio y la celda viene vacía.", etiquetaCampoCanónico(r.Field)),
				lineaColumnaValor(c),
			))
		}
	case "number_gt":
		n, err := parseFlexibleNumber(c.Valor)
		if err != nil {
			out = append(out, mensajeConContexto(
				fmt.Sprintf("El dato «%s» debe ser numérico.", etiquetaCampoCanónico(r.Field)),
				lineaColumnaValor(c),
			))
			break
		}
		if n <= r.Params["min"] {
			out = append(out, mensajeConContexto(
				fmt.Sprintf("El dato «%s» debe ser mayor que %v.", etiquetaCampoCanónico(r.Field), r.Params["min"]),
				lineaColumnaValor(c),
				lineaReferencia(fmt.Sprintf("mínimo %v", r.Params["min"])),
			))
		}
	case "number_gte":
		n, err := parseFlexibleNumber(c.Valor)
		if err != nil {
			out = append(out, mensajeConContexto(
				fmt.Sprintf("El dato «%s» debe ser numérico.", etiquetaCampoCanónico(r.Field)),
				lineaColumnaValor(c),
			))
			break
		}
		if n < r.Params["min"] {
			out = append(out, mensajeConContexto(
				fmt.Sprintf("El dato «%s» debe ser mayor o igual a %v.", etiquetaCampoCanónico(r.Field), r.Params["min"]),
				lineaColumnaValor(c),
				lineaReferencia(fmt.Sprintf("mínimo %v", r.Params["min"])),
			))
		}
	case "number_between":
		n, err := parseFlexibleNumber(c.Valor)
		if err != nil {
			out = append(out, mensajeConContexto(
				fmt.Sprintf("El dato «%s» debe ser numérico.", etiquetaCampoCanónico(r.Field)),
				lineaColumnaValor(c),
			))
			break
		}
		min, max := r.Params["min"], r.Params["max"]
		if n < min || n > max {
			out = append(out, mensajeConContexto(
				fmt.Sprintf("El dato «%s» debe estar entre %v y %v.", etiquetaCampoCanónico(r.Field), min, max),
				lineaColumnaValor(c),
				lineaReferencia(fmt.Sprintf("rango %v–%v", min, max)),
			))
		}
	case "freeze_on_zero_premium":
		// No genera incidencia; solo congela la póliza (se evalúa en runRules).
		_ = frozen
	case "number_in_allowed":
		n, err := parseFlexibleNumber(c.Valor)
		if err != nil {
			out = append(out, mensajeConContexto(
				fmt.Sprintf("El dato «%s» debe ser numérico.", etiquetaCampoCanónico(r.Field)),
				lineaColumnaValor(c),
			))
			break
		}
		if n == 0 {
			break
		}
		allowed := make([]float64, 0)
		for k, val := range r.Params {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(k)), "allowed_") {
				allowed = append(allowed, val)
			}
		}
		if len(allowed) == 0 {
			out = append(out, mensajeConContexto(
				fmt.Sprintf("La regla de catálogo para «%s» no está bien configurada.", etiquetaCampoCanónico(r.Field)),
				lineaColumnaValor(c),
			))
			break
		}
		if !numberInAllowed(c.Valor, allowed) {
			out = append(out, mensajeConContexto(
				fmt.Sprintf("El valor no está en el catálogo permitido para «%s».", etiquetaCampoCanónico(r.Field)),
				lineaColumnaValor(c),
				lineaReferencia(fmt.Sprintf("permitidos %v", allowed)),
			))
		}
	default:
		if tipo != "" {
			out = append(out, fmt.Sprintf("Regla de validación no reconocida (%s) sobre «%s».", tipo, etiquetaCampoCanónico(r.Field)))
		}
	}
	return out
}

func mensajesFechasRequeridas(values map[string]string, cfg ruleRuntimeConfig) []string {
	var out []string
	for _, field := range cfg.RequiredValidDateFields {
		f := strings.TrimSpace(field)
		if f == "" {
			continue
		}
		etiqueta := etiquetaCampoCanónico(f)
		raw := strings.TrimSpace(values[f])
		if raw == "" {
			out = append(out, fmt.Sprintf("La «%s» es obligatoria y viene vacía.", etiqueta))
			continue
		}
		if !dateParsedWithOrder(raw, cfg.DateLayouts, dateOrderDMY) && !dateParsedWithOrder(raw, cfg.DateLayouts, dateOrderMDY) {
			out = append(out, mensajeFechaNoValida(etiqueta, raw))
		}
	}
	return out
}

func mensajeFechaNoValida(etiqueta, raw string) string {
	msg := fmt.Sprintf(
		"La «%s» no tiene un formato de fecha válido como día/mes/año ni como mes/día/año (valor en archivo: %s).",
		etiqueta, strings.TrimSpace(raw),
	)
	if norm := normalizeDateRaw(raw); norm != "" && norm != strings.TrimSpace(raw) {
		msg += fmt.Sprintf(" Tras normalizar el texto queda «%s»; revise espacios, hora o formato de celda en Excel.", norm)
	}
	return msg
}

func mensajeFechaNacimientoObligatoriaParaEdad() string {
	return "La fecha de nacimiento es obligatoria para validar la edad de ingreso."
}

func mensajeFechaActivacionObligatoriaParaEdad() string {
	return "La fecha de activación es obligatoria para validar la edad de ingreso (columna FECHAACTIVACION o FECHA INICIO DE VIGENCIA en el archivo)."
}

func mensajeFechaActivacionObligatoriaBolivar() string {
	return "La fecha de adjudicación/activación del crédito es obligatoria para validar la edad (columna FECHA ADJUDICACION en el archivo)."
}

func mensajeFechaActivacionNoCalculable(raw string, layouts []string, mapfreSheet bool, productCode string) string {
	msg := fmt.Sprintf(
		"No se pudo validar la edad: la fecha de activación «%s» no es válida.",
		strings.TrimSpace(raw),
	)
	if mapfreSheet {
		msg += " En archivos MAPFRE suele ir como mes/día/año (ej. 04-08-26 = 8 de abril de 2026)."
	} else if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(productCode)), "BOLIVAR") {
		msg += " En Bolívar use FECHA ADJUDICACION (fecha de activación del crédito); puede venir como serial Excel."
	}
	if norm := normalizeDateRaw(raw); norm != "" && norm != strings.TrimSpace(raw) {
		msg += fmt.Sprintf(" Texto normalizado: «%s».", norm)
	}
	if t := parseAgeReferenceDate(raw, layouts, mapfreSheet, productCode); !t.IsZero() {
		msg += fmt.Sprintf(" Interpretación: %s.", formatFechaCalendario(t))
	}
	return msg
}

func mensajeEdadNoCalculable(birthRaw string) string {
	msg := fmt.Sprintf(
		"No se pudo calcular la edad: la fecha de nacimiento «%s» no es válida como día/mes/año ni como mes/día/año.",
		strings.TrimSpace(birthRaw),
	)
	if norm := normalizeDateRaw(birthRaw); norm != "" && norm != strings.TrimSpace(birthRaw) {
		msg += fmt.Sprintf(" Texto normalizado: «%s».", norm)
	}
	if t := fechaInterpretadaParaMensaje(birthRaw); !t.IsZero() {
		msg += fmt.Sprintf(" Como mes/día/año se interpreta: %s.", formatFechaCalendario(t))
	}
	return msg
}

// fechaInterpretadaParaMensaje devuelve la fecha si al menos un orden (DMY o MDY) parsea.
func fechaInterpretadaParaMensaje(raw string) time.Time {
	layouts := defaultDateLayouts()
	if t := parseDateWithLayoutsOrder(raw, layouts, dateOrderMDY); !t.IsZero() {
		return t
	}
	return parseDateWithLayoutsOrder(raw, layouts, dateOrderDMY)
}

func formatAgeMaxYears(ageMax float64) string {
	if ageMax <= 0 {
		return "0"
	}
	return strconv.Itoa(int(math.Floor(ageMax + 1e-9)))
}

func mensajeEdadFueraDeRango(code string, d edadValidacionDetalle, ageMin, ageMax float64, _ int) string {
	var b strings.Builder
	maxYear := ageLimitMaxBirthdayYear(ageMax)
	fmt.Fprintf(&b,
		"La edad (%d años cumplidos) está fuera del rango permitido para el producto %s (mínimo %d años cumplidos; máximo hasta el día anterior al cumpleaños %d, equivalente a %s años 364 días).",
		d.edadReportada, strings.TrimSpace(code), ageLimitMinInt(ageMin), maxYear, formatAgeMaxYears(ageMax),
	)
	if raw := strings.TrimSpace(d.birthRaw); raw != "" {
		fmt.Fprintf(&b, " Fecha de nacimiento en archivo: «%s».", raw)
	}
	refCal := formatFechaCalendario(d.ref)
	fmt.Fprintf(&b, " Edad al momento de la activación")
	if refCal != "" {
		fmt.Fprintf(&b, " (%s)", refCal)
	}
	if v := strings.TrimSpace(d.refValorArchivo); v != "" {
		fmt.Fprintf(&b, "; valor en archivo: «%s»", v)
	}
	b.WriteString(".")
	appendEdadInterpretacion(&b, "día/mes/año", d.nacimientoDMY, d.edadDMY, d.fechaLimiteMaxDMY)
	if !d.nacimientoMDY.IsZero() && (d.nacimientoDMY != d.nacimientoDMY || d.edadMDY != d.edadDMY) {
		appendEdadInterpretacion(&b, "mes/día/año", d.nacimientoMDY, d.edadMDY, d.fechaLimiteMaxMDY)
	}
	return b.String()
}

func appendEdadInterpretacion(b *strings.Builder, orden string, nacimiento time.Time, edad int, fechaLimite time.Time) {
	if nacimiento.IsZero() || edad < 0 {
		return
	}
	fmt.Fprintf(b, " Con interpretación %s: nacimiento %s → %d años cumplidos",
		orden, formatFechaCalendario(nacimiento), edad)
	if !fechaLimite.IsZero() {
		fmt.Fprintf(b, " (última fecha permitida: %s)", formatFechaCalendario(fechaLimite))
	}
	b.WriteString(".")
}

func mensajeCreditoDuplicadoEnArchivo(credit string, repeats, dupCredits, dupRows int) string {
	return fmt.Sprintf(
		"El número de crédito u operación «%s» se repite en este mismo archivo (%d veces en total; %d créditos con duplicado; %d filas duplicadas). Revise las filas indicadas en el informe.",
		credit, repeats, dupCredits, dupRows,
	)
}

func mensajeCreditoDuplicadoHistorico() string {
	return "El número de crédito u operación ya existe en pólizas procesadas anteriormente (duplicado histórico)."
}

func mensajeCreditoDuplicadoEnArchivoPorFila(credit, filas string) string {
	return fmt.Sprintf(
		"El número de crédito u operación «%s» aparece más de una vez en este archivo. Filas del archivo donde consta: %s.",
		credit, filas,
	)
}

func mensajeVencimientoMesPasado(mesMinimoAnio, mesMinimo int) string {
	return fmt.Sprintf(
		"La fecha de vencimiento actual del crédito es anterior al mes de facturación cargado (mes vencido %02d/%d); el vencimiento debe ser de ese mes o de un mes posterior (no se valida por día de procesamiento).",
		mesMinimo, mesMinimoAnio,
	)
}

func mensajeBolivarRevisarPrimaVencimientoInferior(
	values map[string]string,
	due time.Time,
	minYear int,
	minMonth time.Month,
	prima float64,
) string {
	dueC := campoDesdeValues(values, "loan_due_date_current")
	primaC := campoDesdeValues(values, "monthly_premium")
	return mensajeConContexto(
		"La fecha de vencimiento (mes/año) es anterior al mes de facturación del archivo; revise la prima mensual. No se usa el mes calendario en que se carga el lote (ej. mayo).",
		lineaColumnaValor(dueC),
		lineaValorUsado(FormatDateCanonical(due)),
		lineaColumnaValor(primaC),
		lineaReferencia("mes mínimo del archivo "+mesAnioRef(minYear, time.Month(minMonth))),
		archivoFacturacionRef(values),
	)
}

func mensajeBolivarPrimaCeroVencimientoInferior(mesFactAnio, mesFact int) string {
	return fmt.Sprintf(
		"Vencimiento anterior al mes de facturación (%02d/%d) con prima mensual en cero: la póliza se marca como cancelada (sin incidencia por fecha de vencimiento).",
		mesFact, mesFactAnio,
	)
}

func mensajeFechaBolivarAmbiguaMDY(campo, raw string, dmy, mdy time.Time) string {
	return fmt.Sprintf(
		"%s: «%s» admite dos lecturas (%s día/mes/año o %s mes/día/año); se aplicó mes/día/año (convención inclusiones Bolívar). Corrija en origen si la lectura no es la esperada.",
		etiquetaCampoCanónico(campo),
		raw,
		FormatDateCanonical(dmy),
		FormatDateCanonical(mdy),
	)
}

func mensajeFechaBolivarInvalida(values map[string]string, canon string) string {
	c := campoDesdeValues(values, canon)
	return mensajeConContexto(
		fmt.Sprintf("%s: no se pudo interpretar la fecha en inclusiones Bolívar (use mes/día/año MM-DD-AAAA o serial Excel).", etiquetaCampoCanónico(canon)),
		lineaColumnaValor(c),
	)
}

func mensajeFechaBolivarNoInterpretable(values map[string]string, canon string) string {
	c := campoDesdeValues(values, canon)
	return mensajeConContexto(
		fmt.Sprintf("%s: fecha ambigua (día y mes ≤ 12); indique formato mes/día/año.", etiquetaCampoCanónico(canon)),
		lineaColumnaValor(c),
		lineaReferencia("convención inclusiones: mes/día/año"),
	)
}

func mensajeVencimientoMenorAdjudicacion(values map[string]string, adj, due time.Time) string {
	adjC := campoDesdeValues(values, "loan_award_date")
	if adjC.Valor == "" {
		adjC = campoDesdeValues(values, "activation_date")
	}
	dueC := campoDesdeValues(values, "loan_due_date_current")
	return mensajeConContexto(
		"La fecha de vencimiento es anterior a la fecha de adjudicación del crédito.",
		lineaColumnaValor(dueC),
		lineaValorUsado(FormatDateCanonical(due)),
		lineaColumnaValor(adjC),
		lineaReferencia("adjudicación "+FormatDateCanonical(adj)),
	)
}

func mensajePrimaCalculadaDifiere(values map[string]string, primaMensual, primaCalc, deuda, tasaFactor float64, tasaRaw string) string {
	primaC := campoDesdeValues(values, "monthly_premium")
	deudaC := campoDesdeValues(values, "initial_debt_amount")
	tasaC := campoDesdeValues(values, "rate_percent")
	if tasaC.Valor == "" {
		tasaC.Valor = strings.TrimSpace(tasaRaw)
	}
	return mensajeConContexto(
		fmt.Sprintf(
			"La prima mensual no coincide con DEUDA INICIAL × %% (esperada %s; diferencia %s).",
			formatoMontoNegocio(primaCalc),
			formatoMontoNegocio(math.Abs(primaCalc-primaMensual)),
		),
		lineaColumnaValor(primaC),
		lineaColumnaValor(deudaC),
		lineaColumnaValor(tasaC),
		lineaReferencia(fmt.Sprintf("cálculo %s × %s = %s", formatoMontoNegocio(deuda), formatoTasaBolivarFactor(tasaFactor), formatoMontoNegocio(primaCalc))),
	)
}

func formatoTasaBolivarFactor(factor float64) string {
	if factor <= 0 {
		return "0"
	}
	return formatoMontoNegocio(factor)
}

func mensajePrimaCalculadaDifiereJustificada(values map[string]string, primaMensual, primaCalc float64, obs string) string {
	primaC := campoDesdeValues(values, "monthly_premium")
	return mensajeConContexto(
		fmt.Sprintf(
			"La prima mensual difiere de la calculada (%s vs %s); justificada con observación.",
			formatoMontoNegocio(primaMensual),
			formatoMontoNegocio(primaCalc),
		),
		lineaColumnaValor(primaC),
		lineaReferencia("observación «"+strings.TrimSpace(obs)+"»"),
	)
}

func mensajeDeudaAltaRevisionManual(umbral float64) string {
	return fmt.Sprintf(
		"La deuda inicial supera el umbral de revisión manual (más de %s pesos); requiere validación antes de emitir.",
		formatoMontoNegocio(umbral),
	)
}

func mensajePlazoCalculadoDifiere(plazoArchivo float64, plazoCalc int, adj, due time.Time) string {
	return fmt.Sprintf(
		"El plazo calculado del archivo (%s meses) no coincide con el calculado por fechas (%d meses entre %s y %s). Registre observación si aplica.",
		formatoMontoNegocio(plazoArchivo),
		plazoCalc,
		formatFechaCalendario(adj),
		formatFechaCalendario(due),
	)
}

func mensajePlazoCalculadoDifiereJustificado(plazoArchivo float64, plazoCalc int, obs string) string {
	return fmt.Sprintf(
		"El plazo del archivo (%s) difiere del calculado (%d meses); justificado: «%s».",
		formatoMontoNegocio(plazoArchivo), plazoCalc, strings.TrimSpace(obs),
	)
}

func mensajePlazoFinVigenciaIncoherente(values map[string]string, diffDias, plazoMeses int, adj, due time.Time) string {
	adjC := campoDesdeValues(values, "loan_award_date")
	if adjC.Valor == "" {
		adjC = campoDesdeValues(values, "activation_date")
	}
	dueC := campoDesdeValues(values, "loan_due_date_current")
	finEsp := adj.AddDate(0, plazoMeses, 0)
	return mensajeConContexto(
		fmt.Sprintf(
			"El vencimiento no cuadra con adjudicación + plazo %d meses (diferencia %d días; se espera ≈0).",
			plazoMeses,
			diffDias,
		),
		lineaColumnaValor(adjC),
		lineaValorUsado(FormatDateCanonical(adj)),
		lineaColumnaValor(dueC),
		lineaValorUsado(FormatDateCanonical(due)),
		lineaReferencia("fin esperado "+FormatDateCanonical(finEsp)),
	)
}

func mensajePlazoFinVigenciaJustificado(values map[string]string, diffDias int, obs string) string {
	dueC := campoDesdeValues(values, "loan_due_date_current")
	return mensajeConContexto(
		fmt.Sprintf("Diferencia de %d días entre vencimiento y plazo calculado; justificado.", diffDias),
		lineaColumnaValor(dueC),
		lineaReferencia("observación «"+strings.TrimSpace(obs)+"»"),
	)
}

func mensajePrimaNoCoincideValidacionExcel(primaArchivo, primaExcel float64, refRaw string) string {
	return fmt.Sprintf(
		"La prima mensual (%s) no coincide con la columna VALIDACION PRIMA MENSUAL del archivo (%s).",
		formatoMontoNegocio(primaArchivo), strings.TrimSpace(refRaw),
	)
}

func mensajeDifPrimaExcelDistinta(difRaw string) string {
	return fmt.Sprintf(
		"La columna Dif prima del archivo indica diferencia (%s); el control de tasa del Excel no está en cero.",
		strings.TrimSpace(difRaw),
	)
}

func mensajePlazoControlExcelDifiere(plazoCtrl float64, plazoCalc int, adj, due time.Time) string {
	return fmt.Sprintf(
		"La columna Control plazo (%s meses) no coincide con el plazo calculado por fechas (%d meses entre %s y %s).",
		formatoMontoNegocio(plazoCtrl), plazoCalc, formatFechaCalendario(adj), formatFechaCalendario(due),
	)
}

func mensajePrimaNoPermitida(code, valorRaw string, permitidos []float64) string {
	return fmt.Sprintf(
		"La prima mensual (%s) no está dentro de los valores permitidos para el producto %s (catálogo: %s).",
		valorRaw, strings.TrimSpace(code), formatoListaMontos(permitidos),
	)
}

func mensajeInicioVigenciaFueraMesTrabajo(valorRaw string) string {
	return fmt.Sprintf(
		"La fecha de inicio de vigencia (%s) no corresponde al mes de trabajo actual; debe ajustarse al periodo que se está procesando.",
		valorRaw,
	)
}

func mensajeFinVigenciaIncoherentePlazo(inicioRaw, plazoRaw, finRaw string, diffDays, tolerancia int, layouts []string) string {
	direccion := "posterior"
	absDiff := diffDays
	if diffDays < 0 {
		direccion = "anterior"
		absDiff = -diffDays
	}
	msg := fmt.Sprintf(
		"La fecha de fin de vigencia (%s) no coincide con el plazo inicial: desde el inicio (%s) con %s meses de plazo, la fecha fin esperada difiere en %d días (%s a la esperada). Tolerancia permitida: ±%d días.",
		finRaw, inicioRaw, strings.TrimSpace(plazoRaw), absDiff, direccion, tolerancia,
	)
	if det := detalleVigenciaPlazoInterpretada(inicioRaw, plazoRaw, finRaw, layouts); det != "" {
		msg += " " + det
	}
	return msg
}

// detalleVigenciaPlazoInterpretada explica cómo se leyeron inicio y fin (mes/día/año en vigencias).
func detalleVigenciaPlazoInterpretada(inicioRaw, plazoRaw, finRaw string, layouts []string) string {
	term, _ := parseFlexibleNumber(plazoRaw)
	if term <= 0 {
		return ""
	}
	var b strings.Builder
	for _, orden := range []struct {
		label string
		order dateFieldOrder
	}{
		{"día/mes/año", dateOrderDMY},
		{"mes/día/año", dateOrderMDY},
	} {
		start := parseVigenciaDateWithLayoutsOrder(inicioRaw, layouts, orden.order)
		end := parseVigenciaDateWithLayoutsOrder(finRaw, layouts, orden.order)
		if start.IsZero() || end.IsZero() {
			continue
		}
		expected := start.AddDate(0, int(term), 0)
		diff := int(end.Sub(expected).Hours() / 24)
		fmt.Fprintf(&b, "Con %s: inicio %s, fin %s, fin esperado %s (diferencia %d días).",
			orden.label,
			formatFechaCalendario(start),
			formatFechaCalendario(end),
			formatFechaCalendario(expected),
			diff,
		)
	}
	return strings.TrimSpace(b.String())
}

func mensajeFlujoCancelacionesMapfre() string {
	return "El nombre del archivo sugiere un flujo de cancelaciones MAPFRE; confirme que la operación corresponde a cancelación y no a inclusión."
}

func mensajePrimaCeroSinCongelamiento() string {
	return "La prima mensual es cero y el producto no tiene política de congelamiento; la póliza se marca como cancelada."
}

func mensajeResumenCreditoDuplicadoArchivo(cred string, count int, filasTodas, filasDup string) string {
	return fmt.Sprintf(
		"Resumen: el crédito u operación «%s» aparece %d veces en el archivo. Filas: %s. Filas repetidas (después de la primera aparición): %s.",
		cred, count, filasTodas, filasDup,
	)
}

func formatoMontoNegocio(n float64) string {
	if n == float64(int64(n)) {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatFloat(n, 'f', 2, 64)
}

func formatoListaMontos(vals []float64) string {
	if len(vals) == 0 {
		return "(sin catálogo)"
	}
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = formatoMontoNegocio(v)
	}
	return strings.Join(parts, ", ")
}
