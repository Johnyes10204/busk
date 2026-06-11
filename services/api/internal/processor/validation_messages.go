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

// etiquetaCortaCampo nombres breves para novedades en informe/Excel.
func etiquetaCortaCampo(canon string) string {
	switch strings.TrimSpace(canon) {
	case "loan_due_date_current":
		return "VENCIMIENTO"
	case "loan_award_date", "activation_date":
		return "ADJUDICACIÓN"
	case "birth_date":
		return "NACIMIENTO"
	case "monthly_premium":
		return "PRIMA"
	case "initial_debt_amount":
		return "DEUDA"
	case "rate_percent":
		return "TASA"
	default:
		return strings.ToUpper(etiquetaCampoCanónico(canon))
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
			out = append(out, fmt.Sprintf("FALTA %s", strings.ToUpper(etiqueta)))
			continue
		}
		if !dateParsedWithOrder(raw, cfg.DateLayouts, dateOrderDMY) && !dateParsedWithOrder(raw, cfg.DateLayouts, dateOrderMDY) {
			out = append(out, mensajeFechaNoValida(etiqueta, raw))
		}
	}
	return out
}

func mensajeFechaNoValida(etiqueta, raw string) string {
	_ = raw
	return fmt.Sprintf("FECHA NO VÁLIDA: %s", strings.ToUpper(etiqueta))
}

func mensajeFechaNacimientoObligatoriaParaEdad() string {
	return "FALTA FECHA DE NACIMIENTO"
}

func mensajeFechaActivacionObligatoriaParaEdad() string {
	return "FALTA FECHA DE ACTIVACIÓN"
}

func mensajeFechaActivacionObligatoriaBolivar() string {
	return "FALTA FECHA DE ADJUDICACIÓN"
}

func mensajeFechaActivacionNoCalculable(raw string, layouts []string, mapfreSheet bool, productCode string) string {
	_ = layouts
	_ = mapfreSheet
	_ = productCode
	return fmt.Sprintf("FECHA ACTIVACIÓN NO VÁLIDA: %s", strings.TrimSpace(raw))
}

func mensajeEdadNoCalculable(birthRaw string) string {
	return fmt.Sprintf("FECHA NACIMIENTO NO VÁLIDA: %s", strings.TrimSpace(birthRaw))
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
	_ = code
	return fmt.Sprintf(
		"REVISAR EDAD: %d AÑOS (PERMITIDO %d-%s)",
		d.edadReportada,
		ageLimitMinInt(ageMin),
		formatAgeMaxYears(ageMax),
	)
}


func mensajeCreditoDuplicadoEnArchivo(credit string, repeats, dupCredits, dupRows int) string {
	_ = dupCredits
	_ = dupRows
	return fmt.Sprintf("OP BT DUPLICADO EN ARCHIVO: %s (%d VECES)", credit, repeats)
}

func mensajeCreditoDuplicadoHistorico() string {
	return "OP BT DUPLICADO: YA PROCESADO"
}

func mensajeCreditoDuplicadoEnArchivoPorFila(credit, filas string) string {
	return fmt.Sprintf("OP BT DUPLICADO: %s (FILAS %s)", credit, filas)
}

func mensajeVencimientoMesPasado(mesMinimoAnio, mesMinimo int) string {
	return fmt.Sprintf("FECHA NO SE ADMITE: ANTERIOR AL MES %02d/%d", mesMinimo, mesMinimoAnio)
}

func mensajeBolivarVencimientoMesAnteriorAlCargue(
	values map[string]string,
	due time.Time,
	cargueYear int,
	cargueMonth time.Month,
	prima float64,
) string {
	_ = values
	_ = due
	if prima == 0 {
		return "VENCIMIENTO: ANTERIOR AL MES DE CARGUE (PRIMA CERO)"
	}
	return fmt.Sprintf(
		"FECHA NO SE ADMITE: ANTERIOR AL MES DE CARGUE (%s)",
		mesAnioRef(cargueYear, cargueMonth),
	)
}

// mensajeBolivarRevisarPrimaVencimientoInferior conserva el nombre histórico en tests/docs.
func mensajeBolivarRevisarPrimaVencimientoInferior(
	values map[string]string,
	due time.Time,
	minYear int,
	minMonth time.Month,
	prima float64,
) string {
	return mensajeBolivarVencimientoMesAnteriorAlCargue(values, due, minYear, minMonth, prima)
}

func mensajeBolivarPrimaCeroVencimientoInferior(mesFactAnio, mesFact int) string {
	return fmt.Sprintf("VENCIMIENTO: ANTERIOR AL MES %02d/%d (PRIMA CERO)", mesFact, mesFactAnio)
}

func mensajeFechaBolivarAmbiguaMDY(campo, raw string, dmy, mdy time.Time) string {
	_ = raw
	_ = dmy
	return fmt.Sprintf(
		"REVISAR FECHA %s: AMBIGUA (USADO %s)",
		etiquetaCortaCampo(campo),
		FormatDateCanonical(mdy),
	)
}

func mensajeFechaBolivarInvalida(values map[string]string, canon string) string {
	_ = values
	return fmt.Sprintf("FECHA NO VÁLIDA: %s", etiquetaCortaCampo(canon))
}

func mensajeFechaBolivarNoInterpretable(values map[string]string, canon string) string {
	_ = values
	return fmt.Sprintf("FECHA AMBIGUA: %s", etiquetaCortaCampo(canon))
}

func mensajeVencimientoMenorAdjudicacion(values map[string]string, adj, due time.Time) string {
	_ = values
	_ = adj
	_ = due
	return "VENCIMIENTO: ANTERIOR A ADJUDICACIÓN"
}

func mensajePrimaCalculadaDifiere(values map[string]string, primaMensual, primaCalc, deuda, tasaFactor float64, tasaRaw string) string {
	_ = values
	_ = primaMensual
	_ = deuda
	_ = tasaFactor
	_ = tasaRaw
	return fmt.Sprintf("REVISAR PRIMA: ESPERADA %s", formatoMontoNegocio(primaCalc))
}

func formatoTasaBolivarFactor(factor float64) string {
	if factor <= 0 {
		return "0"
	}
	return formatoMontoNegocio(factor)
}

func mensajePrimaCalculadaDifiereJustificada(values map[string]string, primaMensual, primaCalc float64, obs string) string {
	_ = values
	_ = primaMensual
	_ = obs
	return fmt.Sprintf("REVISAR PRIMA: ESPERADA %s (CON OBSERVACIÓN)", formatoMontoNegocio(primaCalc))
}

func mensajeDeudaAltaRevisionManual(umbral float64) string {
	return fmt.Sprintf("REVISAR DEUDA: SUPERA %s", formatoMontoNegocio(umbral))
}

func mensajePlazoCalculadoDifiere(plazoArchivo float64, plazoCalc int, adj, due time.Time) string {
	_ = adj
	_ = due
	return fmt.Sprintf("REVISAR PLAZO: ARCHIVO %s MESES, CALCULADO %d", formatoMontoNegocio(plazoArchivo), plazoCalc)
}

func mensajePlazoCalculadoDifiereJustificado(plazoArchivo float64, plazoCalc int, obs string) string {
	_ = obs
	return fmt.Sprintf("REVISAR PLAZO: ARCHIVO %s, CALCULADO %d (CON OBSERVACIÓN)", formatoMontoNegocio(plazoArchivo), plazoCalc)
}

func mensajePlazoFinVigenciaIncoherente(values map[string]string, diffDias, plazoMeses int, adj, due time.Time) string {
	_ = values
	_ = adj
	_ = due
	return fmt.Sprintf("REVISAR PLAZO: DIFERENCIA %d DÍAS (PLAZO %d MESES)", diffDias, plazoMeses)
}

func mensajePlazoFinVigenciaJustificado(values map[string]string, diffDias int, obs string) string {
	_ = values
	_ = obs
	return fmt.Sprintf("REVISAR PLAZO: DIFERENCIA %d DÍAS (CON OBSERVACIÓN)", diffDias)
}

func mensajePrimaNoCoincideValidacionExcel(primaArchivo, primaExcel float64, refRaw string) string {
	_ = primaExcel
	_ = refRaw
	return fmt.Sprintf("REVISAR PRIMA: NO COINCIDE VALIDACIÓN EXCEL (%s)", formatoMontoNegocio(primaArchivo))
}

func mensajeDifPrimaExcelDistinta(difRaw string) string {
	return fmt.Sprintf("REVISAR PRIMA: DIFERENCIA EXCEL %s", strings.TrimSpace(difRaw))
}

func mensajePlazoControlExcelDifiere(plazoCtrl float64, plazoCalc int, adj, due time.Time) string {
	_ = adj
	_ = due
	return fmt.Sprintf("REVISAR PLAZO: CONTROL %s, CALCULADO %d MESES", formatoMontoNegocio(plazoCtrl), plazoCalc)
}

func mensajePrimaNoPermitida(code, valorRaw string, permitidos []float64) string {
	_ = code
	_ = permitidos
	return fmt.Sprintf("REVISAR PRIMA: VALOR %s NO PERMITIDO", valorRaw)
}

func mensajeInicioVigenciaFueraMesTrabajo(valorRaw string) string {
	return fmt.Sprintf("FECHA INICIO: FUERA DEL MES DE TRABAJO (%s)", valorRaw)
}

func mensajeFinVigenciaIncoherentePlazo(inicioRaw, plazoRaw, finRaw string, diffDays, tolerancia int, layouts []string) string {
	_ = inicioRaw
	_ = plazoRaw
	_ = finRaw
	_ = layouts
	_ = tolerancia
	absDiff := diffDays
	if absDiff < 0 {
		absDiff = -absDiff
	}
	return fmt.Sprintf("REVISAR FIN VIGENCIA: DIFERENCIA %d DÍAS", absDiff)
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
	return "ARCHIVO: POSIBLE CANCELACIÓN MAPFRE"
}

func mensajeCancelacionFaltaFinProyectado() string {
	return "CANCELACIÓN: FALTA FECHA PROYECTADA FIN DE VIGENCIA"
}

func mensajeCancelacionFaltaFechaActivacion() string {
	return "CANCELACIÓN: FALTA FECHA DE ACTIVACIÓN"
}

func mensajeCancelacionFechasNoValidas(finRaw, activRaw string) string {
	return fmt.Sprintf(
		"CANCELACIÓN: FECHAS NO VÁLIDAS (FIN=%s, ACTIVACIÓN=%s)",
		strings.TrimSpace(finRaw),
		strings.TrimSpace(activRaw),
	)
}

func mensajeCancelacionFinActivMismoDia(fin, activ time.Time) string {
	return fmt.Sprintf(
		"CANCELACIÓN: FIN DE VIGENCIA DEBE COINCIDIR EN DÍA CON ACTIVACIÓN (FIN=%s, ACTIVACIÓN=%s)",
		formatFechaCalendario(fin),
		formatFechaCalendario(activ),
	)
}

func mensajeCancelacionMesEtiquetaIndeterminado() string {
	return "CANCELACIÓN: NO SE PUDO DETERMINAR MES DE ETIQUETADO (OBS O NOMBRE DE ARCHIVO)"
}

func mensajeCancelacionFinFueraMesEtiqueta(fin time.Time, labelYear int, labelMonth time.Month) string {
	return fmt.Sprintf(
		"CANCELACIÓN: FIN DE VIGENCIA FUERA DEL MES ETIQUETADO (FIN=%s, ETIQUETA=%02d/%d)",
		formatFechaCalendario(fin),
		int(labelMonth),
		labelYear,
	)
}

func mensajeCancelacionPolizaNoEnStock(credit string) string {
	return fmt.Sprintf("CANCELACIÓN: CRÉDITO NO ENCONTRADO EN STOCK MAPFRE (OP BT=%s)", strings.TrimSpace(credit))
}

func mensajeCancelacionDocumentoNoCoincide(anulDoc, stockDoc string) string {
	return fmt.Sprintf(
		"CANCELACIÓN: DOCUMENTO NO COINCIDE CON STOCK (ARCHIVO=%s, STOCK=%s)",
		strings.TrimSpace(anulDoc),
		strings.TrimSpace(stockDoc),
	)
}

func mensajeCancelacionGrupoPolizaInconsistente(grupo, actual string) string {
	return fmt.Sprintf(
		"CANCELACIÓN: PÓLIZA GRUPO Y ACTUAL INCONSISTENTES (GRUPO=%s, ACTUAL=%s)",
		strings.TrimSpace(grupo),
		strings.TrimSpace(actual),
	)
}

func mensajeCancelacionGrupoPolizaNoCoincideStock(anulGrupo, stockGrupo string) string {
	return fmt.Sprintf(
		"CANCELACIÓN: PÓLIZA GRUPO NO COINCIDE CON STOCK (ARCHIVO=%s, STOCK=%s)",
		strings.TrimSpace(anulGrupo),
		strings.TrimSpace(stockGrupo),
	)
}

func mensajeCancelacionActivacionNoCoincideStock(anulActiv, stockActiv string) string {
	return fmt.Sprintf(
		"CANCELACIÓN: FECHA ACTIVACIÓN NO COINCIDE CON STOCK (ARCHIVO=%s, STOCK=%s)",
		strings.TrimSpace(anulActiv),
		strings.TrimSpace(stockActiv),
	)
}

func mensajePrimaCeroSinCongelamiento() string {
	return "PRIMA CERO: PÓLIZA CANCELADA"
}

func mensajePolizaCongeladaPrimaCero() string {
	return "PRIMA CERO: PÓLIZA CONGELADA"
}

func mensajeResumenCreditoDuplicadoArchivo(cred string, count int, filasTodas, filasDup string) string {
	return fmt.Sprintf("OP BT DUPLICADO: %s (%d VECES, FILAS %s)", cred, count, filasTodas)
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
