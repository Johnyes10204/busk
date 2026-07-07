package processor

import (
	"strings"
	"testing"
	"time"
)

func TestBolivarTasaFactorDesdeRaw(t *testing.T) {
	cases := []struct {
		raw    string
		factor float64
	}{
		{"0.1%", 0.001},
		{"0,1%", 0.001},
		{"0.001", 0.001},
		{"1E-3", 0.001},
		{"0.1", 0.001},
		{"23", 0.23},
		{"23%", 0.23},
	}
	for _, c := range cases {
		got := bolivarTasaFactorDesdeRaw(c.raw)
		if got < c.factor-1e-9 || got > c.factor+1e-9 {
			t.Fatalf("%q: factor %v want %v", c.raw, got, c.factor)
		}
	}
}

func TestBolivarPrimaEsperada_Anexo4(t *testing.T) {
	prima, factor := bolivarPrimaEsperada(23_717_600, "0.001")
	if factor != 0.001 {
		t.Fatalf("factor %v", factor)
	}
	if prima < 23_717.5 || prima > 23_717.7 {
		t.Fatalf("prima %v", prima)
	}
}

func TestBolivarPrimaEsperada_Stock0_1Pct(t *testing.T) {
	for _, raw := range []string{"0.1%", "0.001", "0.1"} {
		prima, _ := bolivarPrimaEsperada(25_000_000, raw)
		if prima < 24_999 || prima > 25_001 {
			t.Fatalf("%q: prima %v", raw, prima)
		}
	}
}

func TestBolivarPrimaEsperada_Tasa23NoCuadraCon25000(t *testing.T) {
	prima, _ := bolivarPrimaEsperada(25_000_000, "23")
	if prima < 5_749_000 || prima > 5_751_000 {
		t.Fatalf("23%% sobre deuda: prima %v", prima)
	}
}

func TestObservacionJustificaDiferenciaBolivar_PDF(t *testing.T) {
	if !observacionJustificaDiferenciaBolivar("FACTURACION ABRIL 2026") {
		t.Fatal("cualquier observación no vacía justifica según E.4 del PDF")
	}
	if observacionJustificaDiferenciaBolivar("   ") {
		t.Fatal("observación vacía no justifica")
	}
}

func TestApplyBolivarDiagramRules_StockRowSinFalsoPositivoPrima(t *testing.T) {
	values := map[string]string{
		"initial_debt_amount":        "25000000",
		"rate_percent":               "0.1%",
		"monthly_premium":            "25000",
		"loan_award_date":            "01-20-22",
		"loan_due_date_current":      "05-15-26",
		"OBSERVACIONES FEBRERO 2026": "FACTURACION FEBRERO 2026",
	}
	cfg := ruleRuntimeConfig{
		DateLayouts:               defaultDateLayouts(),
		BolivarPrimaCalcTolerance: 1,
		BolivarPlazoDiasTolerance: 31,
	}
	hard, soft := applyBolivarDiagramRules(values, cfg)
	for _, h := range hard {
		if strings.Contains(strings.ToLower(h), "prima mensual") {
			t.Fatalf("no debe fallar prima: hard=%v soft=%v", hard, soft)
		}
	}
	for _, s := range soft {
		if strings.Contains(strings.ToLower(s), "prima") && strings.Contains(strings.ToLower(s), "difiere") {
			t.Fatalf("prima cuadra, no debe haber nota de prima: %v", soft)
		}
	}
}

func TestApplyBolivarDiagramRules_Tasa23ObservacionSuavizaPrima(t *testing.T) {
	values := map[string]string{
		"initial_debt_amount": "25000000",
		"rate_percent":        "23",
		"monthly_premium":     "25000",
		"observacion":         "FACTURACION ABRIL 2026",
	}
	cfg := ruleRuntimeConfig{BolivarPrimaCalcTolerance: 1}
	hard, soft := applyBolivarDiagramRules(values, cfg)
	for _, h := range hard {
		if strings.Contains(strings.ToLower(h), "prima mensual") {
			t.Fatalf("con observación E.4 debe ser nota, no incidencia: hard=%v soft=%v", hard, soft)
		}
	}
	foundSoft := false
	for _, s := range soft {
		if strings.Contains(strings.ToLower(s), "prima") {
			foundSoft = true
		}
	}
	if !foundSoft {
		t.Fatalf("esperada nota de prima justificada: hard=%v soft=%v", hard, soft)
	}
}

func TestApplyBolivarDiagramRules_Tasa23SinObservacionIncidencia(t *testing.T) {
	values := map[string]string{
		"initial_debt_amount": "25000000",
		"rate_percent":        "23",
		"monthly_premium":     "25000",
	}
	cfg := ruleRuntimeConfig{BolivarPrimaCalcTolerance: 1}
	hard, _ := applyBolivarDiagramRules(values, cfg)
	found := false
	for _, h := range hard {
		if strings.Contains(strings.ToUpper(h), "REVISAR PRIMA") {
			found = true
		}
	}
	if !found {
		t.Fatalf("sin observación debe ser incidencia: %v", hard)
	}
}

func TestBolivarApplyDiagramRules_SinIncidenciaEdadSiUnaInterpretacionValida(t *testing.T) {
	cfg := ruleRuntimeConfig{
		DateLayouts:              defaultDateLayouts(),
		HasAgeLimits:             true,
		AgeMin:                   18,
		AgeMax:                   75.997,
		AgeMaxDaysBeforeBirthday: 1,
	}
	values := map[string]string{
		"birth_date":      "01/06/2007",
		"loan_award_date": "02-06-25",
	}
	seen := make(map[string]struct{})
	hard, _ := applyDiagramRules("BOLIVAR_INCLUSION_DEUDORES_BANCO", values, seen, nil, nil, cfg, &Service{})
	for _, h := range hard {
		if strings.Contains(strings.ToLower(h), "edad") || strings.Contains(strings.ToLower(h), "rango permitido") {
			t.Fatalf("no debe registrar incidencia de edad si una lectura cumple: %v", hard)
		}
	}
}

func TestBolivarEdad_DMYCumpleEdadMinima(t *testing.T) {
	layouts := defaultDateLayouts()
	ref := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	ok, edad := edadCumpleRangoBolivarDual("01/06/2007", layouts, ref, 18, 75.997, 1)
	if !ok || edad != 18 {
		t.Fatalf("día/mes/año con 18 cumplidos: ok=%v edad=%d", ok, edad)
	}
	values := map[string]string{
		"birth_date":      "01/06/2007",
		"loan_award_date": "02-06-25",
	}
	cfg := ruleRuntimeConfig{
		DateLayouts:              defaultDateLayouts(),
		HasAgeLimits:             true,
		AgeMin:                   18,
		AgeMax:                   75.997,
		AgeMaxDaysBeforeBirthday: 1,
	}
	det := evaluarEdadDetalle(values, cfg, false, "BOLIVAR_BANCO")
	if !det.cumple {
		t.Fatalf("evaluarEdadDetalle Bolívar debe aceptar interpretación válida: %+v", det)
	}
}

func TestBolivarEdadFueraRango_SoloConDeudaMayor20M(t *testing.T) {
	cfg := ruleRuntimeConfig{
		DateLayouts:                defaultDateLayouts(),
		HasAgeLimits:               true,
		AgeMin:                     18,
		AgeMax:                     75.997,
		AgeMaxDaysBeforeBirthday:   1,
		BolivarDebtManualThreshold: 20_000_000,
	}
	values := map[string]string{
		"birth_date":          "27-09-46",
		"loan_award_date":     "05-04-23",
		"initial_debt_amount": "15000000",
	}
	det := evaluarEdadDetalle(values, cfg, false, "BOLIVAR_BANCO")
	if det.cumple {
		t.Fatalf("caso usuario: debe detectar edad fuera de rango en activación: det=%+v", det)
	}
	if bolivarAplicaIncidenciaEdadFueraRango(values, cfg) {
		t.Fatal("deuda 15M no debe exigir incidencia de edad")
	}
	values["initial_debt_amount"] = "25000000"
	if !bolivarAplicaIncidenciaEdadFueraRango(values, cfg) {
		t.Fatal("deuda 25M debe exigir incidencia de edad")
	}
}

func TestApplyBolivarDiagramRules_SinIncidenciaDeuda20MSola(t *testing.T) {
	values := map[string]string{
		"initial_debt_amount": "25000000",
		"rate_percent":        "0.001",
		"monthly_premium":     "25000",
		"loan_award_date":     "20-01-22",
		"loan_due_date_current": "15-05-26",
	}
	cfg := ruleRuntimeConfig{
		BolivarPrimaCalcTolerance:  1,
		BolivarDebtManualThreshold: 20_000_000,
		BolivarPlazoDiasTolerance:  31,
	}
	hard, _ := applyBolivarDiagramRules(values, cfg)
	for _, h := range hard {
		if strings.Contains(strings.ToLower(h), "deuda") && strings.Contains(h, "20") {
			t.Fatalf("PDF E.8: deuda >20M no genera incidencia sola: %v", hard)
		}
	}
}

func TestBolivarMesMinimoVencimiento_MesVencido(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	y, m := bolivarMesMinimoVencimiento(now, -1)
	if y != 2026 || m != time.April {
		t.Fatalf("en mayo el mes mínimo debe ser abril: got %d-%v", y, m)
	}
	// El día del mes de procesamiento no cambia el umbral.
	nowDia1 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	y2, m2 := bolivarMesMinimoVencimiento(nowDia1, -1)
	if y2 != y || m2 != m {
		t.Fatalf("día 1 vs 21 de mayo: got %d-%v vs %d-%v", y2, m2, y, m)
	}
}

func TestBolivarMesReferenciaE10_AbrilDelArchivoNoMayoCalendario(t *testing.T) {
	now := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	values := map[string]string{"_file_name": "MICRO_BANCO_ABRIL_VF_Pruebas.xlsx"}
	cfg := ruleRuntimeConfig{BolivarDueReferenceMonthOffset: -1}
	y, m := bolivarMesReferenciaE10(values, cfg, now)
	if y != 2026 || m != time.April {
		t.Fatalf("en mayo con archivo ABRIL la referencia debe ser abril, no mayo: got %02d/%d", m, y)
	}
}

func TestBolivarMesFacturacionDesdeArchivo_Abril(t *testing.T) {
	y, m, ok := bolivarMesFacturacionDesdeArchivo("MICRO_BANCO_ABRIL_VF_Pruebas.xlsx")
	if !ok || m != time.April || y != 2026 {
		t.Fatalf("got %v %d ok=%v", m, y, ok)
	}
}

func TestBolivarVencimientoInferior_PrimaCeroConVencPasadoSiGeneraInforme(t *testing.T) {
	values := map[string]string{
		"loan_due_date_current": "16-02-26",
		"monthly_premium":       "0",
		"_file_name":            "MICRO_BANCO_ABRIL_VF_Pruebas.xlsx",
	}
	cfg := ruleRuntimeConfig{
		DateLayouts:             defaultDateLayouts(),
		BolivarValidateDueMonth: true,
	}
	hard, soft := applyBolivarDiagramRules(values, cfg)
	if len(hard) > 0 {
		t.Fatalf("prima 0 con vencimiento pasado no debe generar incidencia dura: %v", hard)
	}
	found := false
	for _, s := range soft {
		up := strings.ToUpper(s)
		if strings.Contains(up, "VENCIMIENTO (E.10)") {
			found = true
		}
	}
	if !found {
		t.Fatalf("prima 0 con vencimiento pasado debe generar informe: %v", soft)
	}
}

func TestBolivarVencimientoInferior_PrimaPositivaInformeNoBloquea(t *testing.T) {
	values := map[string]string{
		"loan_due_date_current": "16-02-26",
		"monthly_premium":       "25000",
		"_file_name":            "MICRO_BANCO_ABRIL_VF_Pruebas.xlsx",
	}
	cfg := ruleRuntimeConfig{
		DateLayouts:             defaultDateLayouts(),
		BolivarValidateDueMonth: true,
	}
	hard, soft := applyBolivarDiagramRules(values, cfg)
	if len(hard) > 0 {
		t.Fatalf("vencimiento inferior no debe ser incidencia bloqueante: %v", hard)
	}
	found := false
	for _, s := range soft {
		if strings.Contains(strings.ToUpper(s), "VENCIMIENTO (E.10)") {
			found = true
		}
	}
	if !found {
		t.Fatalf("prima > 0 con vencimiento inferior debe generar aviso informativo: soft=%v", soft)
	}
}

func TestBolivarVencimiento_MayoCargueAbrilHaciaAtras(t *testing.T) {
	cfg := ruleRuntimeConfig{
		DateLayouts:             defaultDateLayouts(),
		BolivarValidateDueMonth: true,
	}
	base := map[string]string{
		"monthly_premium": "25000",
		"_file_name":      "MICRO_BANCO_MAYO_VF_2026.xlsx",
	}
	cases := []struct {
		due      string
		informar bool
	}{
		{"15-04-26", true},  // abril → informe (mes anterior a mayo)
		{"15-03-26", true},  // marzo → informe
		{"15-05-26", false}, // mayo cargue → OK
		{"15-06-26", false}, // junio → OK
	}
	for _, tc := range cases {
		values := map[string]string{
			"loan_due_date_current": tc.due,
			"monthly_premium":       base["monthly_premium"],
			"_file_name":            base["_file_name"],
		}
		_, soft := applyBolivarDiagramRules(values, cfg)
		tiene := false
		for _, s := range soft {
			low := strings.ToLower(s)
			if strings.Contains(low, "vencimiento (e.10)") {
				tiene = true
			}
		}
		if tiene != tc.informar {
			t.Fatalf("cargue MAYO due=%s informar=%v soft=%v", tc.due, tc.informar, soft)
		}
	}
}

func TestBolivarMesCargueDelLote_TodosLosMeses(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	cfg := ruleRuntimeConfig{BolivarDueReferenceMonthOffset: -1}
	meses := []struct {
		archivo string
		wantM   time.Month
	}{
		{"LOTE_ENERO_2026", time.January},
		{"LOTE_FEBRERO_2026", time.February},
		{"LOTE_MARZO_2026", time.March},
		{"MICRO_BANCO_ABRIL", time.April},
		{"MICRO_BANCO_MAYO", time.May},
		{"FACT_JUNIO_2026", time.June},
		{"FACT_JULIO_2026", time.July},
		{"FACT_AGOSTO_2026", time.August},
		{"FACT_SEPTIEMBRE_2026", time.September},
		{"FACT_OCTUBRE_2026", time.October},
		{"FACT_NOVIEMBRE_2026", time.November},
		{"FACT_DICIEMBRE_2026", time.December},
	}
	for _, tc := range meses {
		values := map[string]string{"_file_name": tc.archivo}
		_, m := bolivarMesCargueDelLote(values, cfg, now)
		if m != tc.wantM {
			t.Fatalf("archivo %q: mes cargue %v want %v", tc.archivo, m, tc.wantM)
		}
	}
}

func TestBolivarVencimiento_AbrilCargueMarzoHaciaAtras(t *testing.T) {
	cfg := ruleRuntimeConfig{
		DateLayouts:             defaultDateLayouts(),
		BolivarValidateDueMonth: true,
	}
	base := map[string]string{
		"monthly_premium": "25000",
		"_file_name":      "MICRO_BANCO_ABRIL_VF_Pruebas.xlsx",
	}
	cases := []struct {
		due      string
		informar bool
	}{
		{"15-03-26", true},  // marzo → informe
		{"16-02-26", true},  // febrero → informe
		{"15-04-26", false}, // abril cargue → OK
		{"15-05-26", false}, // mayo → OK
	}
	for _, tc := range cases {
		values := map[string]string{
			"loan_due_date_current": tc.due,
			"monthly_premium":       base["monthly_premium"],
			"_file_name":            base["_file_name"],
		}
		_, soft := applyBolivarDiagramRules(values, cfg)
		tiene := false
		for _, s := range soft {
			low := strings.ToLower(s)
			if strings.Contains(low, "vencimiento (e.10)") {
				tiene = true
			}
		}
		if tiene != tc.informar {
			t.Fatalf("due=%s informar=%v got soft=%v", tc.due, tc.informar, soft)
		}
	}
}

func TestBolivarVencimientoInferior_MayoOK(t *testing.T) {
	values := map[string]string{
		"loan_due_date_current": "05-15-26",
		"monthly_premium":       "25000",
		"_file_name":            "MICRO_BANCO_ABRIL_VF_Pruebas.xlsx",
	}
	cfg := ruleRuntimeConfig{
		DateLayouts:             defaultDateLayouts(),
		BolivarValidateDueMonth: true,
	}
	hard, _ := applyBolivarDiagramRules(values, cfg)
	for _, h := range hard {
		if strings.Contains(strings.ToLower(h), "vencimiento") && strings.Contains(strings.ToLower(h), "facturación") {
			t.Fatalf("mayo 2026 no debe fallar vs abril: %v", hard)
		}
	}
}

func TestBolivarVencimientoE10_SoloComparaMes(t *testing.T) {
	minY, minM := 2026, time.April
	for _, due := range []time.Time{
		time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC),
	} {
		if bolivarVencimientoAntesMesCargue(due, minY, minM) {
			t.Fatalf("vencimiento %v no debe fallar con mes mínimo abril", due)
		}
	}
	if !bolivarVencimientoAntesMesCargue(time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC), minY, minM) {
		t.Fatal("marzo debe ser anterior al mes mínimo abril")
	}
}

func TestBolivarPlazoCalculadoMeses_Anexo(t *testing.T) {
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	adj := base.AddDate(0, 0, 44466)
	due := base.AddDate(0, 0, 46058)
	// 1592 días / 30.4375 (promedio real) = 52.31 → round = 52 meses.
	if got := bolivarPlazoCalculadoMeses(adj, due); got != 52 {
		t.Fatalf("plazo meses: got %d want 52", got)
	}
}

// Reproduce el caso Pyme BANCO JUNIO 2026 (OP BT 1335056):
// adj 2022-07-06, due 2028-07-04, PLAZO CRÉDITO 72 → no debe emitir REVISAR PLAZO.
func TestBolivarPlazoMesesDesdeValues_PrefierePlazoCredito(t *testing.T) {
	adj := time.Date(2022, 7, 6, 0, 0, 0, 0, time.UTC)
	due := time.Date(2028, 7, 4, 0, 0, 0, 0, time.UTC)
	values := map[string]string{"calculated_term": "72"}
	if got := bolivarPlazoMesesDesdeValues(values, adj, due); got != 72 {
		t.Fatalf("con PLAZO CRÉDITO=72 debe respetar el archivo, got %d", got)
	}
	// Sin el dato, fallback por fechas: 2190 días → round(2190/30.4375)=72.
	if got := bolivarPlazoMesesDesdeValues(nil, adj, due); got != 72 {
		t.Fatalf("fallback por fechas: got %d want 72", got)
	}
}

// Caso end-to-end reportado por el cliente (Pyme BANCO JUNIO 2026, fila 25, OP BT 1335056):
// con la fórmula anterior salía "REVISAR PLAZO: DIFERENCIA -33 DÍAS (CON OBSERVACIÓN)".
// La corrección (usa PLAZO CRÉDITO y tolerancia 5 días default) elimina el falso positivo.
func TestApplyBolivarDiagramRules_Pyme_1335056_NoRevisarPlazo(t *testing.T) {
	values := map[string]string{
		"initial_debt_amount":     "25000000",
		"rate_percent":            "0.1",
		"monthly_premium":         "25000",
		"loan_award_date":         "06/07/2022",
		"loan_due_date_current":   "04/07/2028",
		"calculated_term":         "72",
		"OBSERVACIONES JUNIO 2026": "FACTURACION JUNIO",
	}
	cfg := ruleRuntimeConfig{
		DateLayouts:               defaultDateLayouts(),
		BolivarPrimaCalcTolerance: 1,
		BolivarPlazoDiasTolerance: 5,
	}
	hard, soft := applyBolivarDiagramRules(values, cfg)
	for _, h := range hard {
		if strings.Contains(strings.ToUpper(h), "REVISAR PLAZO") {
			t.Fatalf("no debe reportar REVISAR PLAZO: hard=%v soft=%v", hard, soft)
		}
	}
	for _, s := range soft {
		if strings.Contains(strings.ToUpper(s), "REVISAR PLAZO") {
			t.Fatalf("no debe reportar REVISAR PLAZO ni como soft: soft=%v", soft)
		}
	}
}

func TestMensajeBolivarVencimientoE10_ExplicaControl(t *testing.T) {
	values := map[string]string{
		"loan_due_date_current": "03-15-26",
		"_file_name":            "4. Deudores_Banco_Bolivar__Pyme_BANCO_ABRIL.xlsx",
	}
	due := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	msg := mensajeBolivarVencimientoMesAnteriorAlCargue(values, due, 2026, time.April, 25000)
	for _, want := range []string{
		"VENCIMIENTO (E.10)",
		"FECHA VENCIMIENTO ACTUAL",
		"03-15-26",
		"mes de cargue ABRIL/2026",
		"Pyme_BANCO_ABRIL",
		"04/2026",
		"no bloquea carga",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("falta %q en %q", want, msg)
		}
	}
}
