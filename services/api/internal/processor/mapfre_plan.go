package processor

import (
	"fmt"
	"math"
	"strings"
)

// mapfreTariffLine tarifario por nombre de plan (diagrama B.3): prima mensual y valor asegurado esperados.
type mapfreTariffLine struct {
	planName      string
	premium       float64
	insuredAmount float64
}

func mapfreTariffsForProduct(code string) ([]mapfreTariffLine, bool) {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "MAPFRE_VIDA", "MAPFRE_INCLUSION_VIDA_VOLUNTARIO", "MAPFRE_STOCK", "MAPFRE_STOCK_CARTERA":
		// Voluntario
		return []mapfreTariffLine{
			{planName: "PLAN 1", premium: 8600, insuredAmount: 5_000_000},
			{planName: "PLAN 2", premium: 17100, insuredAmount: 10_000_000},
		}, true
	case "MAPFRE_ACC_MEN", "MAPFRE_INCLUSION_AP_MENORES":
		// AP + Accidentes menores
		return []mapfreTariffLine{
			{planName: "PLAN 1", premium: 7800, insuredAmount: 5_000_000},
			{planName: "PLAN 1", premium: 7410, insuredAmount: 5_000_000},
			{planName: "PLAN 2", premium: 10600, insuredAmount: 8_000_000},
			{planName: "PLAN 2", premium: 10070, insuredAmount: 8_000_000},
		}, true
	case "MAPFRE_CANCER", "MAPFRE_INCLUSION_AP_CANCER":
		// AP + Cáncer (primas en semilla + muestras INCLUSION-CANCER-MAPFRE.xlsx)
		return []mapfreTariffLine{
			{planName: "PLAN 1", premium: 8500, insuredAmount: 7_000_000},
			{planName: "PLAN 1", premium: 8075, insuredAmount: 7_000_000},
			{planName: "PLAN 2", premium: 13000, insuredAmount: 10_000_000},
			{planName: "PLAN 2", premium: 12350, insuredAmount: 10_000_000},
			{planName: "PLAN 2", premium: 12000, insuredAmount: 10_000_000},
		}, true
	default:
		return nil, false
	}
}

// validarPlanMapfre cruza plan_name (Plan 1 / Plan 2) con monthly_premium y valor asegurado del archivo.
func validarPlanMapfre(code string, values map[string]string) []string {
	tariffs, ok := mapfreTariffsForProduct(code)
	if !ok {
		return nil
	}

	planName := strings.TrimSpace(values["plan_name"])
	premRaw := strings.TrimSpace(values["monthly_premium"])
	prem, _ := parseFlexibleNumber(premRaw)

	if planName == "" {
		return []string{mensajePlanNombreObligatorio()}
	}

	if !planNombreEnTarifario(planName, tariffs) {
		return []string{mensajePlanNombreNoPermitido(planName, nombresPlanUnicos(tariffs))}
	}

	lines := tariffLinesForPlanName(planName, tariffs)
	if premRaw == "" || prem <= 0 {
		return []string{mensajePrimaObligatoriaParaPlan(planName)}
	}

	termMonths, _ := parseFlexibleNumber(strings.TrimSpace(values["initial_term_months"]))
	var matched *mapfreTariffLine
	for i := range lines {
		if mapfrePrimaCoincideTarifa(prem, lines[i].premium, termMonths) {
			matched = &lines[i]
			break
		}
	}
	if matched == nil {
		return []string{mensajePrimaNoCoincideConPlan(planName, premRaw, primasParaPlan(lines), termMonths)}
	}

	insuredRaw := valorAseguradoDesdeValues(values)
	insured, _ := parseFlexibleNumber(insuredRaw)
	if insuredRaw == "" || insured <= 0 {
		return []string{mensajeValorAseguradoObligatorioParaPlan(planName, matched.premium, matched.insuredAmount)}
	}
	if !montosEquivalentes(insured, matched.insuredAmount) {
		return []string{mensajeValorAseguradoNoCoincideConPlan(planName, premRaw, insuredRaw, matched.insuredAmount)}
	}

	return nil
}

func tariffLinesForPlanName(planName string, tariffs []mapfreTariffLine) []mapfreTariffLine {
	var out []mapfreTariffLine
	for _, t := range tariffs {
		if planNombreCoincideLinea(planName, t.planName) {
			out = append(out, t)
		}
	}
	return out
}

func primasParaPlan(lines []mapfreTariffLine) []float64 {
	seen := map[float64]struct{}{}
	var out []float64
	for _, t := range lines {
		if _, ok := seen[t.premium]; ok {
			continue
		}
		seen[t.premium] = struct{}{}
		out = append(out, t.premium)
	}
	return out
}

func montosEquivalentes(a, b float64) bool {
	if a <= 0 || b <= 0 {
		return false
	}
	return math.Abs(a-b) < 0.01
}

// mapfrePrimaCoincideTarifa: la prima del archivo puede venir como mensual o como total del plazo.
// Si prima/plazo (meses) coincide con el tarifario, se acepta como prima mensual válida.
func mapfrePrimaCoincideTarifa(premArchivo, primaTarifa, plazoMeses float64) bool {
	if montosEquivalentes(premArchivo, primaTarifa) {
		return true
	}
	if plazoMeses > 0 {
		return montosEquivalentes(premArchivo/plazoMeses, primaTarifa)
	}
	return false
}

func planNombreCoincideLinea(planName, linePlan string) bool {
	n := normalizePlanNombre(planName)
	ln := normalizePlanNombre(linePlan)
	return n == ln || strings.HasSuffix(n, " "+ln) || strings.Contains(n, ln)
}

func planNombreEnTarifario(planName string, tariffs []mapfreTariffLine) bool {
	for _, t := range tariffs {
		if planNombreCoincideLinea(planName, t.planName) {
			return true
		}
	}
	return false
}

func nombresPlanUnicos(tariffs []mapfreTariffLine) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, t := range tariffs {
		if _, ok := seen[t.planName]; ok {
			continue
		}
		seen[t.planName] = struct{}{}
		out = append(out, t.planName)
	}
	return out
}

func normalizePlanNombre(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	return strings.Join(strings.Fields(s), " ")
}

func mensajePlanNombreObligatorio() string {
	return "El nombre del plan es obligatorio; solo se aceptan «Plan 1» o «Plan 2» según el tarifario MAPFRE."
}

func mensajePlanNombreNoPermitido(planName string, allowed []string) string {
	return fmt.Sprintf(
		"El nombre del plan «%s» no es válido; solo se aceptan: %s.",
		strings.TrimSpace(planName), strings.Join(allowed, ", "),
	)
}

func mensajePrimaObligatoriaParaPlan(planName string) string {
	return fmt.Sprintf(
		"La prima mensual es obligatoria para validar el plan «%s».",
		strings.TrimSpace(planName),
	)
}

func mensajePrimaNoCoincideConPlan(planName, premRaw string, primasPermitidas []float64, plazoMeses float64) string {
	hint := ""
	if plazoMeses > 0 {
		hint = fmt.Sprintf(
			" También se validó prima ÷ plazo (%s meses) frente al tarifario.",
			formatoMontoNegocio(plazoMeses),
		)
	}
	return fmt.Sprintf(
		"La prima (%s) no coincide con el plan «%s» como prima mensual ni como total del plazo dividido entre meses; primas mensuales permitidas: %s.%s",
		strings.TrimSpace(premRaw), strings.TrimSpace(planName), formatoListaMontos(primasPermitidas), hint,
	)
}

func mensajeValorAseguradoObligatorioParaPlan(planName string, premium, insuredExpected float64) string {
	return fmt.Sprintf(
		"Plan no válido: el valor asegurado es obligatorio para «%s» con prima %s (valor asegurado esperado: %s).",
		strings.TrimSpace(planName),
		formatoMontoNegocio(premium),
		formatoMontoNegocio(insuredExpected),
	)
}

func mensajeValorAseguradoNoCoincideConPlan(planName, premRaw, insuredRaw string, insuredExpected float64) string {
	return fmt.Sprintf(
		"Plan no válido: el valor asegurado (%s) no corresponde a «%s» con prima %s (valor asegurado esperado: %s).",
		strings.TrimSpace(insuredRaw),
		strings.TrimSpace(planName),
		strings.TrimSpace(premRaw),
		formatoMontoNegocio(insuredExpected),
	)
}

// valorAseguradoDesdeValues lee el monto desde el campo canónico o desde encabezados del archivo.
func valorAseguradoDesdeValues(values map[string]string) string {
	for _, key := range []string{"insured_amount", "VALORARECONOCER", "VALOR ASEGURADO", "VALOR_ASEGURADO"} {
		if v := strings.TrimSpace(values[key]); v != "" {
			return v
		}
	}
	for key, val := range values {
		upper := strings.ToUpper(strings.TrimSpace(key))
		if strings.Contains(upper, "VALOR") && strings.Contains(upper, "ASEGURADO") {
			if v := strings.TrimSpace(val); v != "" {
				return v
			}
		}
	}
	return ""
}
