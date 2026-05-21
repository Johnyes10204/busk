package processor

import (
	"testing"

	"github.com/buskseguros-design/services/api/internal/model"
)

func TestColumnIndexForFieldMap_DosColumnasPorcentaje(t *testing.T) {
	header := []string{"DEUDA INICIAL", "%", "PLAZO", "% ", "Valor servicio"}
	m := model.FieldMap{CanonicalField: "rate_percent", SourceHeader: "%", Required: true}
	col, ok := columnIndexForFieldMap(header, m)
	if !ok || col != 1 {
		t.Fatalf("rate_percent debe usar columna %% sin espacio: col=%d ok=%v", col, ok)
	}
}
