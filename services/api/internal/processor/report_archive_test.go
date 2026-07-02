package processor

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buskseguros-design/services/api/internal/model"
	"github.com/buskseguros-design/services/api/internal/store"
	"github.com/buskseguros-design/services/api/internal/validationnotes"
	"github.com/xuri/excelize/v2"
)

func TestSaveValidationReportArchive_WritesXLSXWithMirrorSheet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REPORTS_ARCHIVE_DIR", dir)

	colOrder, _ := json.Marshal([]string{"IDENTIFICACION"})
	rawNote, _ := json.Marshal(map[string]string{
		"_excel_column_order": string(colOrder),
		"IDENTIFICACION":      "111",
	})
	rawLimpia, _ := json.Marshal(map[string]string{
		"_excel_column_order": string(colOrder),
		"IDENTIFICACION":      "222",
	})
	policies := []model.PolicyRecord{
		{
			RowNumber:      3,
			PolicyStatus:   "MANUAL_REVIEW",
			ValidationJSON: `["` + validationnotes.Incidencia("prima inválida") + `"]`,
			RawDataJSON:    string(rawNote),
		},
		{
			RowNumber:    5,
			PolicyStatus: "ACTIVE",
			RawDataJSON:  string(rawLimpia),
		},
	}
	report := store.BuildFileValidationReportFromPolicies(
		"file_audit_1",
		"INCLUSION_TEST.xlsx",
		"prod_test",
		"PROCESSED",
		"",
		"",
		policies,
	)

	path := saveValidationReportArchive(report, "file_audit_1", "INCLUSION_TEST.xlsx")
	if path == "" {
		t.Fatal("se esperaba ruta de archivo de auditoría")
	}
	if !strings.HasPrefix(path, dir) {
		t.Fatalf("archivo fuera del dir esperado: %s", path)
	}
	if filepath.Base(path) != "file_audit_1_INCLUSION_TEST.xlsx.reporte.xlsx" {
		t.Fatalf("nombre inesperado: %s", filepath.Base(path))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != "Datos archivo" {
		t.Fatalf("se espera una sola hoja «Datos archivo», got %v", sheets)
	}
	rows, _ := f.GetRows("Datos archivo")
	if len(rows) != 3 {
		t.Fatalf("Datos archivo debe traer encabezado + 2 filas, got %d", len(rows))
	}
}

func TestBuildReportArchivePath_HonorsEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REPORTS_ARCHIVE_DIR", dir)
	path, err := buildReportArchivePath("abc", "foo.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, dir) {
		t.Fatalf("ruta fuera del dir env: %s", path)
	}
}
