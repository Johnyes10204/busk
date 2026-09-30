package processor

import (
	"strings"
	"testing"

	"github.com/buskseguros-design/services/api/internal/model"
	"github.com/buskseguros-design/services/api/internal/store"
)

func TestMotivoFaltaProducto_PrefijoInactivo(t *testing.T) {
	diag := store.FormatPrefixDiagnosis{
		FileName:    "INCLUSION-NUEVO.xlsx",
		ActiveCount: 20,
		InactiveMatches: []store.InactiveFormatMatch{
			{Code: "MAPFRE_INCLUSION_AP_MENORES", FilePrefix: "INCLUSION-NUEVO", Priority: 130, SheetName: "Hoja1"},
		},
	}

	motivo := motivoFaltaProducto(diag)

	if !strings.Contains(motivo, "no existe producto configurado") {
		t.Fatalf("debe conservar el motivo genérico:\n%s", motivo)
	}
	if !strings.Contains(motivo, "DESACTIVADO") {
		t.Fatalf("debe decir que el formato está desactivado:\n%s", motivo)
	}
	if !strings.Contains(motivo, `"INCLUSION-NUEVO"`) {
		t.Fatalf("debe nombrar el prefijo que matchea:\n%s", motivo)
	}
	if !strings.Contains(motivo, "MAPFRE_INCLUSION_AP_MENORES") {
		t.Fatalf("debe nombrar el formato:\n%s", motivo)
	}
	if !strings.Contains(motivo, "Hoja1") {
		t.Fatalf("debe nombrar la hoja configurada:\n%s", motivo)
	}
	if !strings.Contains(motivo, "Acción:") {
		t.Fatalf("debe indicar la acción a seguir:\n%s", motivo)
	}
}

func TestMotivoFaltaProducto_NingunPrefijoCoincide(t *testing.T) {
	diag := store.FormatPrefixDiagnosis{FileName: "archivo_raro.xlsx", ActiveCount: 20}

	motivo := motivoFaltaProducto(diag)

	if strings.Contains(motivo, "DESACTIVADO") {
		t.Fatalf("no debe afirmar desactivación cuando nada matchea:\n%s", motivo)
	}
	if !strings.Contains(motivo, "ninguno de los prefijos") && !strings.Contains(motivo, "ninguno matchea") {
		t.Fatalf("debe explicar que ningún prefijo aparece en el nombre:\n%s", motivo)
	}
	if !strings.Contains(motivo, "20") {
		t.Fatalf("debe informar cuántos formatos activos hay:\n%s", motivo)
	}
}

func TestMotivoFaltaProducto_SinFormatosActivos(t *testing.T) {
	diag := store.FormatPrefixDiagnosis{FileName: "cualquiera.xlsx", ActiveCount: 0}

	motivo := motivoFaltaProducto(diag)

	if !strings.Contains(motivo, "No hay formatos activos") {
		t.Fatalf("debe avisar que no hay formatos activos:\n%s", motivo)
	}
}

func TestMotivoFaltaProducto_MultiplesInactivos(t *testing.T) {
	diag := store.FormatPrefixDiagnosis{
		FileName:    "ANULACION.xlsx",
		ActiveCount: 20,
		InactiveMatches: []store.InactiveFormatMatch{
			{Code: "MAPFRE_ANULACION_MASIVA", FilePrefix: "Anulacion masiva", Priority: 100},
			{Code: "MAPFRE_ANULACION_MASIVA_2", FilePrefix: "ANULACION", Priority: 90},
		},
	}

	motivo := motivoFaltaProducto(diag)

	if !strings.Contains(motivo, "2 formatos") {
		t.Fatalf("debe indicar cuántos formatos inactivos matchean:\n%s", motivo)
	}
	if !strings.Contains(motivo, "Anulacion masiva") || !strings.Contains(motivo, "ANULACION") {
		t.Fatalf("debe listar ambos prefijos:\n%s", motivo)
	}
}

func TestRequiredHeaderGap(t *testing.T) {
	// "  valor prima  " con minúsculas y espacios debe matchear "VALOR PRIMA":
	// la comparación normaliza con UPPER+TRIM.
	header := []string{"NUMERO DE CREDITO", "  valor prima  ", "FECHA"}
	mappings := []model.FieldMap{
		{SourceHeader: "NUMERO DE CREDITO", Required: true},
		{SourceHeader: "VALOR PRIMA", Required: true},
		{SourceHeader: "FECHA INCLUSION", Required: true},
		{SourceHeader: "IGNORADO", Required: false},
	}

	present, total, missing := requiredHeaderGap(header, mappings)

	if present != 2 {
		t.Fatalf("present=%d, se esperaba 2 (NUMERO DE CREDITO y VALOR PRIMA normalizado)", present)
	}
	if total != 3 {
		t.Fatalf("total=%d, se esperaba 3 (solo los Required)", total)
	}
	if len(missing) != 1 {
		t.Fatalf("missing=%v, se esperaba 1", missing)
	}
	if missing[0] != "FECHA INCLUSION" {
		t.Fatalf("missing=%v, se esperaba solo FECHA INCLUSION", missing)
	}
}

func TestRequiredHeaderGap_NoReportaNoRequeridos(t *testing.T) {
	header := []string{"NUMERO DE CREDITO"}
	mappings := []model.FieldMap{
		{SourceHeader: "NUMERO DE CREDITO", Required: true},
		{SourceHeader: "IGNORADO", Required: false},
	}

	_, _, missing := requiredHeaderGap(header, mappings)

	if len(missing) != 0 {
		t.Fatalf("no debe reportar mappings no requeridos: %v", missing)
	}
}

func TestMotivoHeadersNoCoinciden_ReportaColumnasQueFaltan(t *testing.T) {
	sheets := []workbookSheet{
		{name: "INFORMACIÓN", rows: [][]string{{"NUMERO DE CREDITO", "FECHA"}}},
		{name: "FACTURACIÓN AGOSTO", rows: [][]string{
			{"NUMERO DE CREDITO", "VALOR PRIMA", "FECHA INCLUSION"},
			{"111", "5000", "01/08/2026"},
		}},
	}
	candidates := []model.Product{{
		Code:       "BOLIVAR_INCLUSION_DEUDORES_BANCO",
		HeaderRow:  1,
		FilePrefix: "MICRO_BANCO",
		Mappings: []model.FieldMap{
			{SourceHeader: "NUMERO DE CREDITO", Required: true},
			{SourceHeader: "VALOR PRIMA", Required: true},
			{SourceHeader: "FECHA INCLUSION", Required: true},
			{SourceHeader: "NOMBRE COMPLETO", Required: true},
		},
	}}

	motivo := motivoHeadersNoCoinciden(sheets, candidates)

	if !strings.Contains(motivo, "ningún formato coincide") {
		t.Fatalf("debe conservar el motivo genérico:\n%s", motivo)
	}
	// La mejor combinación es FACTURACIÓN AGOSTO con 3 de 4 (solo falta NOMBRE COMPLETO).
	if !strings.Contains(motivo, "FACTURACIÓN AGOSTO") {
		t.Fatalf("debe señalar la hoja más cercana:\n%s", motivo)
	}
	if !strings.Contains(motivo, "3 de 4") {
		t.Fatalf("debe informar cuántas columnas encontró de cuántas espera:\n%s", motivo)
	}
	if !strings.Contains(motivo, `"NOMBRE COMPLETO"`) {
		t.Fatalf("debe listar la columna que falta:\n%s", motivo)
	}
	if strings.Contains(motivo, `"FECHA INCLUSION"`) {
		t.Fatalf("no debe listar columnas que sí existen:\n%s", motivo)
	}
	if !strings.Contains(motivo, "2 hoja(s)") || !strings.Contains(motivo, "1 formato(s)") {
		t.Fatalf("debe informar el alcance evaluado:\n%s", motivo)
	}
	if !strings.Contains(motivo, "Acción:") {
		t.Fatalf("debe indicar la acción a seguir:\n%s", motivo)
	}
}

func TestMotivoHeadersNoCoinciden_SinParesEvaluables(t *testing.T) {
	sheets := []workbookSheet{{name: "VACIA", rows: nil}}
	candidates := []model.Product{{Code: "X", HeaderRow: 3, Mappings: []model.FieldMap{{SourceHeader: "A", Required: true}}}}

	motivo := motivoHeadersNoCoinciden(sheets, candidates)

	if !strings.Contains(motivo, "no se pudo evaluar ningún par") {
		t.Fatalf("debe explicar que no había par evaluable:\n%s", motivo)
	}
}

func TestListarHeadersFaltantes_AcotaLaLista(t *testing.T) {
	if got := listarHeadersFaltantes(nil); !strings.Contains(got, "header_row") {
		t.Fatalf("lista vacía debe dirigir a revisar header_row: %q", got)
	}

	many := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		many = append(many, "COL")
	}
	got := listarHeadersFaltantes(many)
	if !strings.Contains(got, "(+8 más)") {
		t.Fatalf("debe acotar a 12 y reportar el resto: %q", got)
	}
	if n := strings.Count(got, `"COL"`); n != 12 {
		t.Fatalf("debe listar exactamente 12, listó %d", n)
	}
}
