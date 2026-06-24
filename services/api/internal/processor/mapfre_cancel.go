package processor

import (
	"strings"
	"strconv"
	"time"

	"github.com/buskseguros-design/services/api/internal/model"
	"github.com/buskseguros-design/services/api/internal/store"
)

type mapfreStockCrossReader interface {
	FindLatestMapfreStockPolicy(creditNumber, documentNumber string) (store.MapfreStockMatch, bool)
}

func isMapfreCancelacionProduct(code string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "MAPFRE_ANULACION", "MAPFRE_ANULACION_MASIVA":
		return true
	default:
		return false
	}
}

// mapfreCancelacionViolaciones aplica controles i–iii del requerimiento y cruce C.1.e contra stock MAPFRE.
func mapfreCancelacionViolaciones(values map[string]string, cfg ruleRuntimeConfig, stockReader mapfreStockCrossReader) []string {
	out := mapfreCancelacionViolacionesFechas(values, cfg)
	out = append(out, mapfreCancelacionViolacionesStock(values, cfg, stockReader)...)
	return out
}

// mapfreCancelacionViolacionesFechas: reglas i–iii (fechas y mes de etiquetado).
func mapfreCancelacionViolacionesFechas(values map[string]string, cfg ruleRuntimeConfig) []string {
	endRaw := strings.TrimSpace(values["coverage_end_date"])
	actRaw := strings.TrimSpace(values["activation_date"])
	if endRaw == "" {
		return []string{mensajeCancelacionFaltaFinProyectado()}
	}
	if actRaw == "" {
		return []string{mensajeCancelacionFaltaFechaActivacion()}
	}

	end, act, ok := parseMapfreCancelacionDatePair(endRaw, actRaw, cfg.DateLayouts)
	if !ok {
		return []string{mensajeCancelacionFechasNoValidas(endRaw, actRaw)}
	}

	var out []string
	if end.Day() != act.Day() {
		out = append(out, mensajeCancelacionFinActivMismoDia(endRaw, actRaw))
	}

	labelYear, labelMonth, okLabel := mapfreMesEtiquetaCancelacion(values)
	if !okLabel {
		out = append(out, mensajeCancelacionMesEtiquetaIndeterminado())
		return out
	}
	if end.Year() != labelYear || end.Month() != labelMonth {
		out = append(out, mensajeCancelacionFinFueraMesEtiqueta(endRaw, labelYear, labelMonth))
	}
	return out
}

// mapfreCancelacionViolacionesStock: cruce anulación ↔ stock (diagrama C.1.e / SDD cancelaciones).
func mapfreCancelacionViolacionesStock(values map[string]string, cfg ruleRuntimeConfig, stockReader mapfreStockCrossReader) []string {
	if stockReader == nil {
		return nil
	}
	credit := strings.TrimSpace(values["credit_number"])
	if credit == "" {
		return nil
	}
	doc := strings.TrimSpace(values["document_number"])

	match, found := stockReader.FindLatestMapfreStockPolicy(credit, doc)
	if !found {
		return []string{mensajeCancelacionPolizaNoEnStock(credit)}
	}

	var out []string
	if doc != "" && !mapfreDocumentosCoinciden(doc, match.DocumentNumber) {
		out = append(out, mensajeCancelacionDocumentoNoCoincide(doc, match.DocumentNumber))
	}

	groupAnul := strings.TrimSpace(values["group_policy_number"])
	policyAnul := strings.TrimSpace(values["policy_number"])
	if groupAnul != "" && policyAnul != "" && !mapfrePolicyNumbersEqual(groupAnul, policyAnul) {
		out = append(out, mensajeCancelacionGrupoPolizaInconsistente(groupAnul, policyAnul))
	}
	if groupAnul != "" && match.GroupPolicy != "" && !mapfrePolicyNumbersEqual(groupAnul, match.GroupPolicy) {
		out = append(out, mensajeCancelacionGrupoPolizaNoCoincideStock(groupAnul, match.GroupPolicy))
	}

	if actRaw := strings.TrimSpace(values["activation_date"]); actRaw != "" && match.ActivationDate != "" {
		anulAct, stockAct, ok := parseMapfreCancelacionDatePair(actRaw, match.ActivationDate, cfg.DateLayouts)
		if ok && !fechasMismoDiaCalendario(anulAct, stockAct) {
			out = append(out, mensajeCancelacionActivacionNoCoincideStock(actRaw, match.ActivationDate))
		}
	}
	return out
}

func mapfreDocumentosCoinciden(a, b string) bool {
	return storeNormalizeDocument(a) == storeNormalizeDocument(b)
}

func mapfrePolicyNumbersEqual(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return true
	}
	if a == b {
		return true
	}
	return storeNormalizeDocument(a) == storeNormalizeDocument(b)
}

func storeNormalizeDocument(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	out := strings.TrimLeft(b.String(), "0")
	if out == "" && b.Len() > 0 {
		return "0"
	}
	return out
}

func fechasMismoDiaCalendario(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func mapfreCancelacionObservacionStock(values map[string]string, cfg ruleRuntimeConfig) string {
	endRaw := strings.TrimSpace(values["coverage_end_date"])
	if end, ok := parseVigenciaDate(endRaw, cfg.DateLayouts); ok {
		return formatCancelarMesAnio(end.Month(), end.Year())
	}
	if y, m, ok := mapfreMesEtiquetaCancelacion(values); ok {
		return formatCancelarMesAnio(m, y)
	}
	return "cancelar"
}

func formatCancelarMesAnio(m time.Month, y int) string {
	return "cancelar/" + pad2(int(m)) + "/" + strconv.Itoa(y)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func parseVigenciaDate(raw string, layouts []string) (time.Time, bool) {
	t := parseDateField(raw, layouts, dateYearContextVigencia)
	return t, !t.IsZero()
}

func parseMapfreCancelacionDatePair(endRaw, actRaw string, layouts []string) (end, act time.Time, ok bool) {
	end = parseDateField(endRaw, layouts, dateYearContextVigencia)
	act = parseDateField(actRaw, layouts, dateYearContextVigencia)
	if !end.IsZero() && !act.IsZero() {
		return end, act, true
	}
	return time.Time{}, time.Time{}, false
}

// mapfreMesEtiquetaCancelacion: mes de etiquetado desde OBS (ABRIL, MAYO, …) o nombre del archivo.
func mapfreMesEtiquetaCancelacion(values map[string]string) (year int, month time.Month, ok bool) {
	if obs := strings.TrimSpace(values["observacion"]); obs != "" {
		upper := strings.ToUpper(obs)
		for name, m := range bolivarMesesNombre {
			if strings.Contains(upper, name) {
				month = m
				break
			}
		}
	}
	if month == 0 {
		return bolivarMesFacturacionDesdeArchivo(strings.TrimSpace(values["_file_name"]))
	}
	year = time.Now().UTC().Year()
	if fn := strings.TrimSpace(values["_file_name"]); fn != "" {
		if match := bolivarAnioEnNombreArchivo.FindString(strings.ToUpper(fn)); match != "" {
			if y, err := strconv.Atoi(match); err == nil && y >= 2000 {
				year = y
			}
		}
	}
	return year, month, true
}

// applyMapfreCancellationsToStock marca el stock MAPFRE al procesar un archivo de anulación válido.
func (svc *Service) applyMapfreCancellationsToStock(policies []model.PolicyRecord) (int64, error) {
	if svc == nil || svc.store == nil {
		return 0, nil
	}
	var total int64
	for _, pol := range policies {
		if strings.ToUpper(strings.TrimSpace(pol.PolicyStatus)) != "CANCELLED" {
			continue
		}
		credit := strings.TrimSpace(pol.CreditNumber)
		if credit == "" {
			continue
		}
		n, err := svc.store.ApplyMapfreStockCancellation(credit, pol.DocumentNumber)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}
