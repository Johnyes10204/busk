package store

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/buskseguros-design/services/api/internal/validationnotes"
	"github.com/xuri/excelize/v2"
)

const frozenInformativeNote = "La prima mensual es cero; la póliza se registra como congelada (no bloquea la carga del archivo)."

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

func policyRowNotes(in policyRowInput) []string {
	var notes []string
	if strings.TrimSpace(in.ValidationJSON) != "" {
		_ = json.Unmarshal([]byte(in.ValidationJSON), &notes)
	}
	return notes
}

// shouldExportMirrorRow: hoja «Datos archivo» solo filas que fallaron la validación (incidencias o revisión manual).
// No incluye congeladas ni filas con únicamente informes informativos.
func shouldExportMirrorRow(in policyRowInput, blocking []string) bool {
	if len(blocking) > 0 {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(in.PolicyStatus), "MANUAL_REVIEW")
}

func buildFileExportedRows(inputs []policyRowInput) (sourceCols []string, rows []FileExportedRow) {
	rows = make([]FileExportedRow, 0)
	failedInputs := make([]policyRowInput, 0)
	for _, in := range inputs {
		if strings.EqualFold(strings.TrimSpace(in.PolicyStatus), "CANCELLED") {
			continue
		}
		notes := policyRowNotes(in)
		blocking, _ := validationnotes.Split(notes)
		if !shouldExportMirrorRow(in, blocking) {
			continue
		}
		raw := parseRawDataMap(in.RawDataJSON)
		novedades := formatNovedadesColumn(trimNotesPreserveAll(blocking))
		if strings.TrimSpace(novedades) == "" {
			novedades = defaultPendingRowDetailMessage(in.PolicyStatus, len(blocking) > 0)
		}
		failedInputs = append(failedInputs, in)
		rows = append(rows, FileExportedRow{
			RowNumber:    in.RowNumber,
			PolicyStatus: strings.TrimSpace(in.PolicyStatus),
			Data:         raw,
			Novedades:    novedades,
		})
	}
	sourceCols = collectSourceColumns(failedInputs)
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].RowNumber < rows[j].RowNumber
	})
	return sourceCols, rows
}

// buildFileExportedRowsEmail: filas para la única hoja del adjunto de correo.
// Incluye cualquier fila con incidencias bloqueantes, revisión manual o avisos informativos.
// Concatena en novedades primero las bloqueantes y luego las informativas.
func buildFileExportedRowsEmail(inputs []policyRowInput) (sourceCols []string, rows []FileExportedRow) {
	rows = make([]FileExportedRow, 0)
	keptInputs := make([]policyRowInput, 0)
	for _, in := range inputs {
		st := strings.TrimSpace(in.PolicyStatus)
		if strings.EqualFold(st, "CANCELLED") {
			continue
		}
		notes := policyRowNotes(in)
		blocking, info := validationnotes.Split(notes)
		if strings.EqualFold(st, "FROZEN") && len(info) == 0 && len(blocking) == 0 {
			info = []string{validationnotes.Informativo(frozenInformativeNote)}
		}
		if len(blocking) == 0 && len(info) == 0 && !strings.EqualFold(st, "MANUAL_REVIEW") {
			continue
		}
		combined := trimNotesPreserveAll(append(append([]string{}, blocking...), info...))
		novedades := formatNovedadesColumn(combined)
		if strings.TrimSpace(novedades) == "" {
			novedades = defaultPendingRowDetailMessage(in.PolicyStatus, len(combined) > 0)
		}
		keptInputs = append(keptInputs, in)
		rows = append(rows, FileExportedRow{
			RowNumber:    in.RowNumber,
			PolicyStatus: st,
			Data:         parseRawDataMap(in.RawDataJSON),
			Novedades:    novedades,
		})
	}
	sourceCols = collectSourceColumns(keptInputs)
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].RowNumber < rows[j].RowNumber
	})
	return sourceCols, rows
}

// validationReportEmailMirrorRows: misma estructura que la hoja "Datos archivo" pero con todas las filas afectadas
// (bloqueantes + informativas + revisión manual).
func validationReportEmailMirrorRows(r FileValidationReport) [][]string {
	prefix := []string{"fila_excel", "estado_poliza"}
	suffix := []string{"novedades"}
	header := append(append(append([]string{}, prefix...), r.EmailSourceColumns...), suffix...)
	out := [][]string{header}
	for _, ex := range r.EmailExportedRows {
		row := make([]string, len(header))
		row[0] = strconv.Itoa(ex.RowNumber)
		row[1] = etiquetaEstadoPolizaInforme(ex.PolicyStatus)
		for i, col := range r.EmailSourceColumns {
			row[2+i] = strings.TrimSpace(ex.Data[col])
		}
		row[len(header)-1] = ex.Novedades
		out = append(out, row)
	}
	return out
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
