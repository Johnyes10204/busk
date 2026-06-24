package processor

import (
	"os"
	"strings"
	"testing"
)

func TestInspectRealArchiveDates_All(t *testing.T) {
	paths := []string{
		"../../data/files-archive/file_1781049017697597000_4. Deudores_Banco_Bolivar__Pyme_BANCO_ABRIL.xlsx",
		"../../data/files-archive/file_1781726592390556000_4. Deudores_Banco_Bolivar_MICRO_BANCO_ABRIL.xlsx",
		"../../data/files-archive/file_1781049017692118000_1. 5024424900103_VIDA_VOL RM-INCLUSION ABRIL2026.xlsx",
		"../../data/files-archive/file_1781706380721368000_5. Deudores_ESAL_Bolivar_Pyme_ESAL_ABRIL.xlsx",
	}
	for _, path := range paths {
		name := path
		if i := strings.LastIndex(path, "/"); i >= 0 {
			name = path[i+1:]
		}
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(path); err != nil {
				t.Skip(path)
			}
			inspectArchiveDates(t, path)
		})
	}
}

func inspectArchiveDates(t *testing.T, path string) {
	rows, err := readWorkbookRows(path, "")
	if err != nil {
		t.Fatal(err)
	}
	header := rows[0]
	layouts := defaultDateLayouts()
	invalid := 0
	for ri := 1; ri < len(rows); ri++ {
		row := rows[ri]
		if rowEmpty(row) {
			continue
		}
		for i, h := range header {
			u := strings.ToUpper(strings.TrimSpace(h))
			var ctx dateYearContext
			var label string
			switch {
			case strings.Contains(u, "NACIMIENTO"):
				label, ctx = "birth", dateYearContextBirth
			case strings.Contains(u, "ADJUDIC"):
				label, ctx = "adj", dateYearContextVigencia
			case strings.Contains(u, "VENCIMIENTO"):
				label, ctx = "due", dateYearContextVigencia
			case strings.Contains(u, "ACTIV"):
				label, ctx = "act", dateYearContextVigencia
			case strings.Contains(u, "INICIO") && strings.Contains(u, "VIGEN"):
				label, ctx = "start", dateYearContextVigencia
			case strings.Contains(u, "FIN") && strings.Contains(u, "VIGEN"):
				label, ctx = "end", dateYearContextVigencia
			default:
				continue
			}
			raw := ""
			if i < len(row) {
				raw = strings.TrimSpace(row[i])
			}
			if raw == "" {
				continue
			}
			if parseDateField(raw, layouts, ctx).IsZero() {
				invalid++
				if invalid <= 10 {
					t.Logf("INVALID %s row=%d col=%q raw=%q norm=%q", label, ri+1, h, raw, normalizeDateRaw(raw))
				}
			}
		}
	}
	t.Logf("data_rows=%d invalid_date_cells=%d", len(rows)-1, invalid)
}

func TestInspectSampleDates_Parse(t *testing.T) {
	layouts := defaultDateLayouts()
	samples := []struct {
		raw string
		ctx dateYearContext
	}{
		{"08-14-76", dateYearContextBirth},
		{"11-22-69", dateYearContextBirth},
		{"12-12-91", dateYearContextBirth},
		{"11-19-21", dateYearContextVigencia},
		{"10-13-22", dateYearContextVigencia},
		{"01-31-23", dateYearContextVigencia},
		{"32461", dateYearContextBirth},
		{"8/14/1976", dateYearContextBirth},
		{"08/14/76", dateYearContextBirth},
		{"14/08/1976", dateYearContextBirth},
		{"04-08-26", dateYearContextVigencia},
		{"06-19-50", dateYearContextBirth},
	}
	for _, s := range samples {
		got := parseDateField(s.raw, layouts, s.ctx)
		t.Logf("%q ctx=%v -> %s zero=%v", s.raw, s.ctx, FormatDateCanonical(got), got.IsZero())
	}
}

func TestPymeHeaderRow(t *testing.T) {
	path := "../../data/files-archive/file_1781049017697597000_4. Deudores_Banco_Bolivar__Pyme_BANCO_ABRIL.xlsx"
	if _, err := os.Stat(path); err != nil {
		t.Skip(path)
	}
	rows, _ := readWorkbookRows(path, "")
	for i := 0; i < 3 && i < len(rows); i++ {
		n := len(rows[i])
		if n > 8 {
			n = 8
		}
		t.Logf("row%d: %v", i, rows[i][:n])
	}
}
