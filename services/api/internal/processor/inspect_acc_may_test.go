package processor

import (
	"os"
	"strings"
	"testing"
)

// TestInspectACCMenMayoFormatoCeldas verifica que el lector entregue las fechas
// en día/mes/año tal como están en la celda, sin reformateo a MDY.
func TestInspectACCMenMayoFormatoCeldas(t *testing.T) {
	path := "/Users/johnnelsonflorez/Documents/ipsilon/busk/tools/sftpconnect/downloads/2. Poliza_5024526900105_ACC MEN RM-INCLUSION MAYO 2026_VF.xlsx"
	if _, err := os.Stat(path); err != nil {
		t.Skip(path)
	}
	rows, err := readWorkbookRows(path, "")
	if err != nil {
		t.Fatal(err)
	}
	header := rows[0]
	col := -1
	for i, h := range header {
		u := strings.ToUpper(strings.TrimSpace(h))
		if strings.Contains(u, "INICIO") && strings.Contains(u, "VIGEN") {
			col = i
			break
		}
	}
	if col < 0 {
		t.Fatal("no encontrada col INICIO VIGENCIA")
	}
	for r := 1; r <= 10 && r < len(rows); r++ {
		if col >= len(rows[r]) {
			continue
		}
		raw := strings.TrimSpace(rows[r][col])
		t.Logf("row=%d raw=%q", r+1, raw)
		if !strings.Contains(raw, "/") {
			t.Errorf("row %d esperaba formato día/mes/año con \"/\", got %q", r+1, raw)
		}
	}
}
