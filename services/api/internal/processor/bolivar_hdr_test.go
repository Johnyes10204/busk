package processor

import (
	"os"
	"strings"
	"testing"
)

func TestMicroBancoHeaders(t *testing.T) {
	path := "/Users/johnnelsonflorez/Documents/ipsilon/busk/tools/sftpconnect/downloads/MICRO_BANCO_ABRIL_VF_Pruebas.xlsx"
	if _, err := os.Stat(path); err != nil {
		t.Skip("no file")
	}
	rows, err := readWorkbookRows(path, "")
	if err != nil {
		t.Fatal(err)
	}
	vencCols := 0
	for i, h := range rows[0] {
		u := strings.ToUpper(strings.TrimSpace(h))
		if strings.Contains(u, "VENC") || strings.Contains(u, "FACTUR") || strings.Contains(u, "MES") {
			t.Logf("col %d: %q", i, h)
		}
		if u == "FECHA VENCIMIENTO ACTUAL" {
			vencCols++
			if len(rows) > 1 && i < len(rows[1]) {
				t.Logf("  fila1 col%d=%q", i, rows[1][i])
			}
		}
	}
	t.Logf("columnas FECHA VENCIMIENTO ACTUAL: %d", vencCols)
	if len(rows) > 1 {
		for i, h := range rows[0] {
			if strings.TrimSpace(h) == "FECHA VENCIMIENTO ACTUAL" && i < len(rows[1]) {
				t.Logf("fila1 vencimiento=%q", rows[1][i])
			}
		}
	}
}
