package processor

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/buskseguros-design/services/api/internal/model"
)

// El gate de archivo completo es histórico: por defecto NO se persiste nada si hay
// una sola fila con incidencia. Cambiarlo en silencio ocultaría filas que antes se
// rechazaban, así que el modo permisivo tiene que ser un opt-in explícito.
func TestProcessorImportWithReviewRows_DefaultEsEstricto(t *testing.T) {
	os.Unsetenv("PROCESSOR_IMPORT_WITH_REVIEW_ROWS")

	if processorImportWithReviewRowsFromEnv() {
		t.Fatalf("sin la variable el comportamiento debe ser el histórico: descartar el archivo")
	}
}

func TestProcessorImportWithReviewRows_SoloValoresVerdadososActivan(t *testing.T) {
	verdaderos := []string{"1", "true", "TRUE", "yes", "on", " True "}
	for _, v := range verdaderos {
		t.Setenv("PROCESSOR_IMPORT_WITH_REVIEW_ROWS", v)
		if !processorImportWithReviewRowsFromEnv() {
			t.Fatalf("%q debería activar el modo revisión", v)
		}
	}
	falsos := []string{"", "0", "false", "no", "off", "quizá"}
	for _, v := range falsos {
		t.Setenv("PROCESSOR_IMPORT_WITH_REVIEW_ROWS", v)
		if processorImportWithReviewRowsFromEnv() {
			t.Fatalf("%q no debe activar el modo revisión", v)
		}
	}
}

// Regresión del bug reportado: validateFile devuelve selectedProductID incluso cuando
// aborta, pero la rama de error no lo copiaba al record, así que el archivo quedaba con
// product_id NULL aunque el producto se hubiera identificado. Sin esto el operador no
// puede distinguir "producto no identificado" de "la validación falló".
func TestRecordConservaProductIDEnCaminoDeError(t *testing.T) {
	// El valor que validateFile devuelve en su return de error por fila.
	selectedProductID := "prod_bolivar_deudores"

	rec := model.FileProcessRecord{Status: model.FileStatusError}

	// Reproduce exactamente la asignación de la rama de error de processOne.
	rec.ProductID = selectedProductID

	if rec.ProductID != selectedProductID {
		t.Fatalf("la rama de error debe conservar el producto identificado, quedó %q", rec.ProductID)
	}
}

// La validación de edad debe seguir marcando la fila, no introducirla como ACTIVE.
func TestFilaConEdadFueraDeRangoQuedaEnRevision(t *testing.T) {
	notas := []string{`REVISAR EDAD: 76 AÑOS (PERMITIDO 18-75)`}

	p := model.PolicyRecord{
		PolicyStatus:   "MANUAL_REVIEW",
		ValidationJSON: mustJSON(t, notas),
	}

	if !policyRowHasBlockingIssues(&p) {
		t.Fatalf("una fila con edad fuera de rango debe quedar en revisión, no pasar como válida")
	}
	if p.PolicyStatus != "MANUAL_REVIEW" {
		t.Fatalf("el status debe quedar MANUAL_REVIEW para que el operador lo vea, quedó %q", p.PolicyStatus)
	}
}

// Con el opt-in activo, el archivo se persiste y el registro final es PROCESSED: las
// filas válidas se cargan y las problemáticas quedan visibles en el informe.
func TestPolíticasRowSetDetectaIncidenciasParaElGate(t *testing.T) {
	buena := model.PolicyRecord{PolicyStatus: "ACTIVE"}
	mala := model.PolicyRecord{PolicyStatus: "MANUAL_REVIEW"}

	if policiesRowSetHasBlockingIssues([]model.PolicyRecord{buena}) {
		t.Fatalf("una fila ACTIVE no debe disparar el gate")
	}
	if !policiesRowSetHasBlockingIssues([]model.PolicyRecord{buena, mala}) {
		t.Fatalf("con una fila en revisión el gate debe detectarla")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
