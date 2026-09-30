package processor

import (
	"testing"

	"github.com/buskseguros-design/services/api/internal/model"
	"github.com/xuri/excelize/v2"
)

// construye un libro con las hojas dadas (cada hoja: [] de filas []string) y devuelve la ruta.
func buildWorkbook(t *testing.T, sheets map[string][][]string, order []string) string {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	primera := order[0]
	if err := f.SetSheetName("Sheet1", primera); err != nil {
		t.Fatalf("rename sheet: %v", err)
	}
	for _, name := range order[1:] {
		if _, err := f.NewSheet(name); err != nil {
			t.Fatalf("new sheet %q: %v", name, err)
		}
	}
	for _, name := range order {
		for r, row := range sheets[name] {
			for c, val := range row {
				cell, err := excelize.CoordinatesToCellName(c+1, r+1)
				if err != nil {
					t.Fatalf("cell: %v", err)
				}
				if err := f.SetCellValue(name, cell, val); err != nil {
					t.Fatalf("set: %v", err)
				}
			}
		}
	}
	path := t.TempDir() + "/libro.xlsx"
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	return path
}

func productoDePrueba(code string, headerRow int, req []string, sheetName string) model.Product {
	mappings := make([]model.FieldMap, 0, len(req))
	for _, h := range req {
		mappings = append(mappings, model.FieldMap{
			SourceHeader:   h,
			CanonicalField: h,
			Required:       true,
		})
	}
	return model.Product{ID: "prod_" + code, Code: code, HeaderRow: headerRow, Mappings: mappings, SheetName: sheetName}
}

// La fase 1 no debe leer el cuerpo de las hojas. Con este libro, cargar todo en memoria
// significa Retention(20.000 filas) + Detalle(10.000); con la lectura por encabezado son
// 25 filas por hoja.
func TestFase1SoloLeeEncabezados(t *testing.T) {
	retencion := [][]string{{"DOCUMENTO", "EDAD"}}
	for i := 0; i < 20000; i++ {
		retencion = append(retencion, []string{"9000000", "40"})
	}
	detalle := [][]string{{"DOCUMENTO", "EDAD"}}
	for i := 0; i < 10000; i++ {
		detalle = append(detalle, []string{"8000000", "30"})
	}

	path := buildWorkbook(t,
		map[string][][]string{"Retention": retencion, "Detalle": detalle},
		[]string{"Retention", "Detalle"})

	sheets, err := readSheetHeadersFromWorkbook(path, 25)
	if err != nil {
		t.Fatalf("read headers: %v", err)
	}
	if len(sheets) != 2 {
		t.Fatalf("se esperaban 2 hojas, vinieron %d", len(sheets))
	}
	for _, sh := range sheets {
		if len(sh.rows) != 25 {
			t.Fatalf("hoja %q trajo %d filas; la fase 1 debe traer a lo sumo 25", sh.name, len(sh.rows))
		}
		if sh.rows[0][0] != "DOCUMENTO" {
			t.Fatalf("hoja %q no arrancó en el encabezado: %v", sh.name, sh.rows[0])
		}
	}
}

// El contrato central: se elige por score de encabezados, NO por posición de hoja. Si la
// hoja con los headers requeridos es la segunda, tiene que ganar; fijar "solo hoja 1"
// cargaría la tabla equivocada.
func TestSeleccionEligePorScoreNoPorPosicion(t *testing.T) {
	// Hoja 1: sólo 1 columna requerida. Hoja 2: las 3 requeridas. Gana la segunda.
	path := buildWorkbook(t, map[string][][]string{
		"Portada":   {{"TITULO DEL REPORTE"}, {"Generado en 2026"}, {"DOCUMENTO"}, {"fila1"}},
		"Operacion": {{"DOCUMENTO", "EDAD", "PRIMA"}, {"111", "40", "5000"}, {"222", "35", "6000"}},
	}, []string{"Portada", "Operacion"})

	prod := productoDePrueba("BOLIVAR_TEST", 1, []string{"DOCUMENTO", "EDAD", "PRIMA"}, "")

	p, header, sheetName, err := selectProductCandidateFromWorkbook(path, []model.Product{prod})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if sheetName != "Operacion" {
		t.Fatalf("debe elegir la hoja con mejor score, eligió %q", sheetName)
	}
	if len(header) != 3 {
		t.Fatalf("header inesperado: %v", header)
	}
	if p.Code != "BOLIVAR_TEST" {
		t.Fatalf("producto inesperado: %q", p.Code)
	}
}

// El encabezado puede estar desplazado varios filas por títulos. maxHeaderRowsForCandidates
// tiene que dar margen suficiente o la fase 1 truncaría justo la fila que decide el match.
func TestFase1CubreEncabezadoDesplazado(t *testing.T) {
	// header_row = 4 => el encabezado está en la cuarta fila (base 1).
	rows := [][]string{
		{"REPORTE DE VIGENCIAS"},
		{"Generado el 30/09/2026"},
		{"Area Comercial"},
		{"DOCUMENTO", "EDAD"},
		{"111", "40"},
	}

	path := buildWorkbook(t, map[string][][]string{"Datos": rows}, []string{"Datos"})

	prod := productoDePrueba("TEST", 4, []string{"DOCUMENTO", "EDAD"}, "")

	_, header, sheetName, err := selectProductCandidateFromWorkbook(path, []model.Product{prod})
	if err != nil {
		t.Fatalf("select con header_row=4: %v", err)
	}
	if sheetName != "Datos" || len(header) != 2 {
		t.Fatalf("no encontró el encabezado desplazado: hoja=%q header=%v", sheetName, header)
	}
}

func TestMaxHeaderRowsDaMargenSobreHeaderRow(t *testing.T) {
	if got := maxHeaderRowsForCandidates([]model.Product{{HeaderRow: 9}}); got < 9 {
		t.Fatalf("no puede leer menos filas que header_row: %d", got)
	}
	if got := maxHeaderRowsForCandidates([]model.Product{{HeaderRow: 3}}); got < 20 {
		t.Fatalf("debe mantener un piso para títulos: %d", got)
	}
	if got := maxHeaderRowsForCandidates(nil); got < 20 {
		t.Fatalf("sin candidatos debe devolver un piso usable: %d", got)
	}
}

// Fase 2: el cuerpo se carga completo y sólo de la hoja elegida, y debe traer todas las
// filas, no el truncado de la fase 1.
func TestFase2CargaCuerpoCompletoDeUnaHoja(t *testing.T) {
	retencion := [][]string{{"DOCUMENTO", "EDAD"}}
	for i := 0; i < 5000; i++ {
		retencion = append(retencion, []string{"9000000", "40"})
	}
	detalle := [][]string{{"DOCUMENTO", "EDAD"}, {"111", "40"}, {"222", "35"}}

	path := buildWorkbook(t,
		map[string][][]string{"Retention": retencion, "Detalle": detalle},
		[]string{"Retention", "Detalle"})

	rows, err := loadSelectedSheetRows(path, "Detalle")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("la hoja elegida debe traer su cuerpo completo, trajo %d filas", len(rows))
	}
	if rows[1][0] != "111" {
		t.Fatalf("contenido inesperado: %v", rows)
	}
}

func TestFase2ErrorSiLaHojaNoExiste(t *testing.T) {
	path := buildWorkbook(t, map[string][][]string{
		"Datos": {{"DOCUMENTO"}, {"111"}},
	}, []string{"Datos"})

	if _, err := loadSelectedSheetRows(path, "NoExiste"); err == nil {
		t.Fatalf("debe fallar si la hoja seleccionada no existe")
	}
}
