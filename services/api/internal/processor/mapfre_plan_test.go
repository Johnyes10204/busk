package processor

import (
	"strings"
	"testing"
)

func TestValidarPlanMapfre_PorNombrePlan(t *testing.T) {
	values := map[string]string{
		"plan_name":       "PLAN 1",
		"monthly_premium": "8600",
	}
	if v := validarPlanMapfre("MAPFRE_VIDA", values); len(v) != 0 {
		t.Fatalf("plan 1 + prima 8600 debe pasar: %v", v)
	}
}

func TestValidarPlanMapfre_PrimaNoCoincidePlan(t *testing.T) {
	values := map[string]string{
		"plan_name":       "PLAN 1",
		"monthly_premium": "17100",
	}
	v := validarPlanMapfre("MAPFRE_VIDA", values)
	if len(v) != 1 {
		t.Fatalf("want 1 violation got %v", v)
	}
	if !strings.Contains(strings.ToLower(v[0]), "no coincide") {
		t.Fatalf("mensaje: %s", v[0])
	}
}

func TestValidarPlanMapfre_IgnoraPlanCode(t *testing.T) {
	values := map[string]string{
		"plan_name":       "PLAN 1",
		"plan_code":       "99999",
		"monthly_premium": "8600",
	}
	if v := validarPlanMapfre("MAPFRE_VIDA", values); len(v) != 0 {
		t.Fatalf("plan_code no debe usarse en validación: %v", v)
	}
}

func TestValidarPlanMapfre_SinNombrePlan(t *testing.T) {
	values := map[string]string{"monthly_premium": "8600"}
	v := validarPlanMapfre("MAPFRE_VIDA", values)
	if len(v) == 0 {
		t.Fatal("falta plan_name debe fallar")
	}
}

func TestValidarPlanMapfre_AccPlan1DosPrimas(t *testing.T) {
	for _, prem := range []string{"7800", "7410"} {
		values := map[string]string{"plan_name": "PLAN 1", "monthly_premium": prem}
		if v := validarPlanMapfre("MAPFRE_ACC_MEN", values); len(v) != 0 {
			t.Fatalf("ACC plan 1 prima %s: %v", prem, v)
		}
	}
}
