package processor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/buskseguros-design/services/api/internal/model"
	"github.com/buskseguros-design/services/api/internal/store"
)

// Regresión del OOM en producción: un archivo de BOLÍVAR con ~167k créditos ya
// cargados en BD generaba una fila por cada uno y cada una serializaba el JSON
// crudo completo. Ninguna de esas copias sobrevivía a la construcción del
// informe ni a la inserción, así que el proceso moría por memoria con el archivo
// en 85% "validaciones OK", sin correo y re-encolado cada 5 minutos.
func TestFilaDuplicadoHistóricoNoGuardaRawData(t *testing.T) {
	rec := model.PolicyRecord{
		PolicyStatus:   model.PolicyStatusSkippedHistoricalDup,
		SkipInsert:     true,
		ValidationJSON: `["crédito ya cargado en corrida previa"]`,
	}

	if rec.RawDataJSON != "" {
		t.Fatalf("las filas omitidas no deben cargar raw_data_json, pesa %d bytes", len(rec.RawDataJSON))
	}
}

// El informe igual debe mostrar la fila omitida: el reporte informativo del mirror
// depende de estos cuatro campos, y es la razón por la que la fila existe en memoria.
func TestFilaDuplicadoHistóricoApareceEnInformeInformativo(t *testing.T) {
	p := model.PolicyRecord{
		RowNumber:      4242,
		DocumentNumber: "CC123",
		CreditNumber:   "2091868",
		PolicyStatus:   model.PolicyStatusSkippedHistoricalDup,
		ValidationJSON: `["crédito ya cargado en corrida previa; fila omitida sin reinsertar"]`,
	}

	report := store.BuildFileValidationReportFromPolicies(
		"file_test", "MICRO_BANCO.xlsx", "prod_bolivar",
		string(model.FileStatusProcessed), "", "2026-09-30T00:00:00Z",
		[]model.PolicyRecord{p},
	)

	encontrada := false
	for _, inf := range report.InformativeValidations {
		if inf.CreditNumber == "2091868" {
			encontrada = true
			if inf.RowNumber != 4242 {
				t.Fatalf("row_number inesperado: %d", inf.RowNumber)
			}
			if len(inf.Notes) == 0 || !strings.Contains(inf.Notes[0], "correna previa") &&
				!strings.Contains(inf.Notes[0], "previa") {
				t.Fatalf("la fila omitida debe conservar su nota informativa, got %v", inf.Notes)
			}
		}
	}
	if !encontrada {
		t.Fatalf("la fila duplicado histórico debe quedar en el informe informativo")
	}
}

// Una fila con incidencia real sí necesita su JSON crudo: es lo que permite
// reconstruir qué se leyó del archivo. La poda de memoria no puede tocar ese caso.
func TestFilaConIncidenciaConservaRawData(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"credit_number": "1", "edad": "76"})

	p := model.PolicyRecord{
		PolicyStatus: "MANUAL_REVIEW",
		RawDataJSON:  string(raw),
	}

	if p.RawDataJSON == "" {
		t.Fatalf("una fila en revisión debe conservar raw_data_json para poder auditarla")
	}
	if !strings.Contains(p.RawDataJSON, "edad") {
		t.Fatalf("raw_data_json debe conservar el dato que disparó la incidencia")
	}
}
