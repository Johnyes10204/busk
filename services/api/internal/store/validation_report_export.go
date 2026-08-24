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

const (
	observacionSinNovedad      = "Sin novedad"
	observacionPolizaCancelada = "Póliza cancelada"
)

// FileExportedRow replica una fila del archivo procesada más observaciones resumidas y novedades.
type FileExportedRow struct {
	RowNumber    int               `json:"row_number"`
	PolicyStatus string            `json:"policy_status"`
	Data         map[string]string `json:"data,omitempty"`
	Observaciones string           `json:"observaciones"`
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

// mergeSourceColumns arma la lista de columnas del espejo. Si el archivo trae
// «_excel_column_order» (encabezados originales), respetamos ese orden y solo
// añadimos las variantes «<encabezado>__col_N» que el procesador crea para
// encabezados repetidos; el resto (campos canónicos del mapping como
// document_number, credit_number, etc.) se omite para que el XLSX sea un
// espejo exacto del archivo original.
// Fallback (sin _excel_column_order): mantenemos todas las claves, ordenadas.
func mergeSourceColumns(preferred []string, allKeys map[string]struct{}) []string {
	seen := make(map[string]struct{})
	if len(preferred) == 0 {
		rest := make([]string, 0, len(allKeys))
		for k := range allKeys {
			rest = append(rest, k)
		}
		sort.Strings(rest)
		return rest
	}
	out := make([]string, 0, len(preferred))
	preferredSet := make(map[string]struct{}, len(preferred))
	for _, k := range preferred {
		preferredSet[k] = struct{}{}
		if _, ok := allKeys[k]; !ok {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	type dupCol struct {
		base string
		n    int
		full string
	}
	dups := make([]dupCol, 0)
	for k := range allKeys {
		if _, dup := seen[k]; dup {
			continue
		}
		idx := strings.Index(k, "__col_")
		if idx <= 0 {
			continue
		}
		base := k[:idx]
		if _, ok := preferredSet[base]; !ok {
			continue
		}
		nStr := k[idx+len("__col_"):]
		n, err := strconv.Atoi(nStr)
		if err != nil {
			continue
		}
		dups = append(dups, dupCol{base, n, k})
	}
	sort.Slice(dups, func(i, j int) bool {
		if dups[i].base != dups[j].base {
			return dups[i].base < dups[j].base
		}
		return dups[i].n < dups[j].n
	})
	for _, d := range dups {
		out = append(out, d.full)
	}
	return out
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

// buildFileMirrorRows arma el espejo del archivo original: incluye TODAS las filas.
// Filas con incidencias o avisos: observaciones+novedades con el detalle.
// Filas CANCELLED: observación "Póliza cancelada", sin novedades.
// Filas limpias: observación "Sin novedad", sin novedades.
func buildFileMirrorRows(inputs []policyRowInput) (sourceCols []string, rows []FileExportedRow) {
	rows = make([]FileExportedRow, 0, len(inputs))
	for _, in := range inputs {
		st := strings.TrimSpace(in.PolicyStatus)
		raw := parseRawDataMap(in.RawDataJSON)

		if strings.EqualFold(st, "CANCELLED") {
			rows = append(rows, FileExportedRow{
				RowNumber:     in.RowNumber,
				PolicyStatus:  st,
				Data:          raw,
				Observaciones: observacionPolizaCancelada,
				Novedades:     "",
			})
			continue
		}

		notes := policyRowNotes(in)
		blocking, info := validationnotes.Split(notes)
		if strings.EqualFold(st, "FROZEN") && len(info) == 0 && len(blocking) == 0 {
			info = []string{validationnotes.Informativo(frozenInformativeNote)}
		}

		if len(blocking) == 0 && len(info) == 0 && !strings.EqualFold(st, "MANUAL_REVIEW") {
			rows = append(rows, FileExportedRow{
				RowNumber:     in.RowNumber,
				PolicyStatus:  st,
				Data:          raw,
				Observaciones: observacionSinNovedad,
				Novedades:     "",
			})
			continue
		}

		combined := trimNotesPreserveAll(append(append([]string{}, blocking...), info...))
		novedades := formatNovedadesColumn(combined)
		if strings.TrimSpace(novedades) == "" {
			novedades = defaultPendingRowDetailMessage(in.PolicyStatus, len(combined) > 0)
		}
		rows = append(rows, FileExportedRow{
			RowNumber:     in.RowNumber,
			PolicyStatus:  st,
			Data:          raw,
			Observaciones: observacionesForRow(combined, st),
			Novedades:     novedades,
		})
	}
	sourceCols = collectSourceColumns(inputs)
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].RowNumber < rows[j].RowNumber
	})
	return sourceCols, rows
}

// displayHeaderFromKey pinta el encabezado del archivo original. Las claves
// «<nombre>__col_N» (encabezados repetidos en el Excel de origen) se muestran
// como su nombre base para que dos columnas iguales del archivo original se
// vean iguales en el espejo. El sufijo __col_N solo vive internamente para
// resolver el valor correcto en cada fila.
func displayHeaderFromKey(key string) string {
	if idx := strings.Index(key, "__col_"); idx > 0 {
		rest := key[idx+len("__col_"):]
		if _, err := strconv.Atoi(rest); err == nil {
			return key[:idx]
		}
	}
	return key
}

// validationReportEmailMirrorRows: misma estructura que la hoja "Datos archivo" pero con todas las filas afectadas
// (bloqueantes + informativas + revisión manual). El espejo replica el archivo original tal cual y solo
// añade las columnas «observaciones» y «novedades» al final; no se anteponen columnas de proceso.
func validationReportEmailMirrorRows(r FileValidationReport) [][]string {
	suffix := []string{"observaciones", "novedades"}
	header := make([]string, 0, len(r.EmailSourceColumns)+len(suffix))
	for _, k := range r.EmailSourceColumns {
		header = append(header, displayHeaderFromKey(k))
	}
	header = append(header, suffix...)
	out := [][]string{header}
	for _, ex := range r.EmailExportedRows {
		row := make([]string, len(header))
		for i, col := range r.EmailSourceColumns {
			row[i] = strings.TrimSpace(ex.Data[col])
		}
		row[len(header)-2] = ex.Observaciones
		row[len(header)-1] = ex.Novedades
		out = append(out, row)
	}
	return out
}

// validationReportMirrorRows: columnas del archivo (orden del Excel) + observaciones y novedades al final.
// El archivo descargable es un espejo exacto del original con únicamente esas dos columnas añadidas.
func validationReportMirrorRows(r FileValidationReport) [][]string {
	suffix := []string{"observaciones", "novedades"}
	header := make([]string, 0, len(r.SourceColumns)+len(suffix))
	for _, k := range r.SourceColumns {
		header = append(header, displayHeaderFromKey(k))
	}
	header = append(header, suffix...)
	out := [][]string{header}
	for _, ex := range r.ExportedRows {
		row := make([]string, len(header))
		for i, col := range r.SourceColumns {
			row[i] = strings.TrimSpace(ex.Data[col])
		}
		row[len(header)-2] = ex.Observaciones
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
	if ncols > 2 {
		firstData, _ := excelize.ColumnNumberToName(1)
		lastData, _ := excelize.ColumnNumberToName(ncols - 2)
		if firstData != "" && lastData != "" {
			_ = f.SetColWidth(sheet, firstData, lastData, 18)
		}
	}
	obsCol, _ := excelize.ColumnNumberToName(ncols - 1)
	novCol, _ := excelize.ColumnNumberToName(ncols)
	if obsCol != "" {
		_ = f.SetColWidth(sheet, obsCol, obsCol, 36)
	}
	if novCol != "" {
		_ = f.SetColWidth(sheet, novCol, novCol, 72)
	}
	if len(rows) > 1 {
		if wrapID, err := f.NewStyle(&excelize.Style{
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
		}); err == nil {
			lastRow := strconv.Itoa(len(rows))
			if obsCol != "" {
				_ = f.SetCellStyle(sheet, obsCol+"2", obsCol+lastRow, wrapID)
			}
			if novCol != "" {
				_ = f.SetCellStyle(sheet, novCol+"2", novCol+lastRow, wrapID)
			}
		}
	}
	return nil
}
