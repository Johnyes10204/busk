package store

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/buskseguros-design/services/api/internal/validationnotes"
	"github.com/xuri/excelize/v2"
)

// FileExportedRow replica una fila del archivo procesada más el texto de novedades.
type FileExportedRow struct {
	RowNumber    int               `json:"row_number"`
	PolicyStatus string            `json:"policy_status"`
	Data         map[string]string `json:"data,omitempty"`
	Novedades    string            `json:"novedades"`
}

func parseRawDataMap(rawJSON string) map[string]string {
	out := make(map[string]string)
	if strings.TrimSpace(rawJSON) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(rawJSON), &out)
	return out
}

func isInternalRawDataKey(key string) bool {
	k := strings.TrimSpace(key)
	if k == "" || k == "_file_name" || k == "product_id" || k == "_excel_column_order" {
		return true
	}
	return strings.HasPrefix(k, "_hdr_")
}

func excelColumnOrderFromRaw(raw map[string]string) []string {
	s := strings.TrimSpace(raw["_excel_column_order"])
	if s == "" {
		return nil
	}
	var order []string
	if err := json.Unmarshal([]byte(s), &order); err != nil {
		return nil
	}
	out := make([]string, 0, len(order))
	for _, h := range order {
		h = strings.TrimSpace(h)
		if h != "" && !isInternalRawDataKey(h) {
			out = append(out, h)
		}
	}
	return out
}

func mergeSourceColumns(preferred []string, allKeys map[string]struct{}) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(allKeys))
	for _, k := range preferred {
		if _, ok := allKeys[k]; !ok {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	rest := make([]string, 0)
	for k := range allKeys {
		if _, ok := seen[k]; ok {
			continue
		}
		rest = append(rest, k)
	}
	sort.Strings(rest)
	return append(out, rest...)
}

func collectSourceColumns(inputs []policyRowInput) []string {
	allKeys := make(map[string]struct{})
	var preferred []string
	for _, in := range inputs {
		raw := parseRawDataMap(in.RawDataJSON)
		if len(preferred) == 0 {
			preferred = excelColumnOrderFromRaw(raw)
		}
		for k := range raw {
			if !isInternalRawDataKey(k) {
				allKeys[k] = struct{}{}
			}
		}
	}
	return mergeSourceColumns(preferred, allKeys)
}

func notesForExportedRow(in policyRowInput) []string {
	var notes []string
	if strings.TrimSpace(in.ValidationJSON) != "" {
		_ = json.Unmarshal([]byte(in.ValidationJSON), &notes)
	}
	blocking, info := validationnotes.Split(notes)
	return append(blocking, info...)
}

func shouldExportMirrorRow(in policyRowInput, notes []string) bool {
	if len(notes) > 0 {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(in.PolicyStatus), "MANUAL_REVIEW")
}

func buildFileExportedRows(inputs []policyRowInput) (sourceCols []string, rows []FileExportedRow) {
	sourceCols = collectSourceColumns(inputs)
	rows = make([]FileExportedRow, 0)
	for _, in := range inputs {
		if strings.EqualFold(strings.TrimSpace(in.PolicyStatus), "CANCELLED") {
			continue
		}
		notes := notesForExportedRow(in)
		if !shouldExportMirrorRow(in, notes) {
			continue
		}
		raw := parseRawDataMap(in.RawDataJSON)
		novedades := formatNovedadesColumn(trimNotesPreserveAll(notes))
		if strings.TrimSpace(novedades) == "" {
			novedades = defaultPendingRowDetailMessage(in.PolicyStatus, len(notes) > 0)
		}
		rows = append(rows, FileExportedRow{
			RowNumber:    in.RowNumber,
			PolicyStatus: strings.TrimSpace(in.PolicyStatus),
			Data:         raw,
			Novedades:    novedades,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].RowNumber < rows[j].RowNumber
	})
	return sourceCols, rows
}

// validationReportMirrorRows: columnas del archivo (orden del Excel) + fila, estado y novedades al final.
func validationReportMirrorRows(r FileValidationReport) [][]string {
	prefix := []string{"fila_excel", "estado_poliza"}
	suffix := []string{"novedades"}
	header := append(append(append([]string{}, prefix...), r.SourceColumns...), suffix...)
	out := [][]string{header}
	for _, ex := range r.ExportedRows {
		row := make([]string, len(header))
		row[0] = strconv.Itoa(ex.RowNumber)
		row[1] = etiquetaEstadoPolizaInforme(ex.PolicyStatus)
		for i, col := range r.SourceColumns {
			row[2+i] = strings.TrimSpace(ex.Data[col])
		}
		row[len(header)-1] = ex.Novedades
		out = append(out, row)
	}
	return out
}

func writeValidationReportMirrorSheet(f *excelize.File, sheet string, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}
	for ri, row := range rows {
		for ci, val := range row {
			cell, err := excelize.CoordinatesToCellName(ci+1, ri+1)
			if err != nil {
				return err
			}
			if err := f.SetCellValue(sheet, cell, val); err != nil {
				return err
			}
		}
	}
	ncols := len(rows[0])
	if ncols == 0 {
		return nil
	}
	_ = f.SetColWidth(sheet, "A", "B", 14)
	if ncols > 2 {
		firstData, _ := excelize.ColumnNumberToName(3)
		lastData, _ := excelize.ColumnNumberToName(ncols - 1)
		if firstData != "" && lastData != "" && ncols > 3 {
			_ = f.SetColWidth(sheet, firstData, lastData, 18)
		}
	}
	lastCol, _ := excelize.ColumnNumberToName(ncols)
	if lastCol != "" {
		_ = f.SetColWidth(sheet, lastCol, lastCol, 72)
	}
	if len(rows) > 1 {
		if wrapID, err := f.NewStyle(&excelize.Style{
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
		}); err == nil {
			lastColName, _ := excelize.ColumnNumberToName(ncols)
			lastRow := strconv.Itoa(len(rows))
			_ = f.SetCellStyle(sheet, lastColName+"2", lastColName+lastRow, wrapID)
		}
	}
	return nil
}
