package processor

import (
	"os"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestExcelizeGetRowsVsGetCellValue_Dates(t *testing.T) {
	path := "../../data/files-archive/file_1781726592390556000_4. Deudores_Banco_Bolivar_MICRO_BANCO_ABRIL.xlsx"
	if _, err := os.Stat(path); err != nil {
		t.Skip(path)
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := f.GetSheetList()[0]
	rows, _ := f.GetRows(sheet)
	if len(rows) < 2 {
		t.Fatal("no rows")
	}
	// find birth col
	birthCol := -1
	for i, h := range rows[0] {
		if h == "FECHA DE NACIMIENTO" {
			birthCol = i
			break
		}
	}
	if birthCol < 0 {
		t.Fatal("no birth col")
	}
	for ri := 1; ri <= 5 && ri < len(rows); ri++ {
		getRowsVal := ""
		if birthCol < len(rows[ri]) {
			getRowsVal = rows[ri][birthCol]
		}
		cell, _ := excelize.CoordinatesToCellName(birthCol+1, ri+1)
		formatted, _ := f.GetCellValue(sheet, cell)
		raw, _ := f.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
		t.Logf("row=%d GetRows=%q GetCellValue=%q RawCellValue=%q", ri+1, getRowsVal, formatted, raw)
	}
}
