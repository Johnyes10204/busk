package processor

import (
	"fmt"
	"math"
	"strings"
)

// mapfreTariffLine tarifario por nombre de plan (diagrama B.3) y prima mensual permitida.
type mapfreTariffLine struct {
	planName string
	premium  float64
}

func mapfreTariffsForProduct(code string) ([]mapfreTariffLine, bool) {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "MAPFRE_VIDA", "MAPFRE_STOCK":
		return []mapfreTariffLine{
			{planName: "PLAN 1", premium: 8600},
			{planName: "PLAN 2", premium: 17100},
		}, true
	case "MAPFRE_ACC_MEN":
		return []mapfreTariffLine{
			{planName: "PLAN 1", premium: 7800},
			{planName: "PLAN 1", premium: 7410},
			{planName: "PLAN 2", premium: 10600},
			{planName: "PLAN 2", premium: 10070},
		}, true
	case "MAPFRE_CANCER":
		return []mapfreTariffLine{
			{planName: "PLAN 1", premium: 8500},
			{planName: "PLAN 2", premium: 12000},
		}, true
	default:
		return nil, false
	}
}

// validarPlanMapfre cruza plan_name (Plan 1 / Plan 2) con monthly_premium (prima mensual del archivo).
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

	var matched *mapfreTariffLine
	for i := range lines {
		if montosEquivalentes(prem, lines[i].premium) {
			matched = &lines[i]
			break
		}
	}
	if matched == nil {
		return []string{mensajePrimaNoCoincideConPlan(planName, premRaw, primasParaPlan(lines))}
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

func mensajePrimaNoCoincideConPlan(planName, premRaw string, primasPermitidas []float64) string {
	return fmt.Sprintf(
		"La prima mensual (%s) no coincide con el plan indicado «%s»; primas permitidas para ese plan: %s.",
		strings.TrimSpace(premRaw), strings.TrimSpace(planName), formatoListaMontos(primasPermitidas),
	)
}
