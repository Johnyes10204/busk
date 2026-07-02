package store

import (
	"strings"
	"testing"

	"github.com/buskseguros-design/services/api/internal/validationnotes"
)

func TestResumenObservacionesFromNotes_variasEtiquetas(t *testing.T) {
	notes := []string{
		validationnotes.Incidencia("REVISAR PRIMA: ESPERADA 8600"),
		validationnotes.Incidencia("REVISAR FIN VIGENCIA: DIFERENCIA 0 DÍAS"),
		validationnotes.Informativo("PRIMA CERO: PÓLIZA CONGELADA"),
	}
	got := resumenObservacionesFromNotes(notes)
	for _, want := range []string{"CONGELAMIENTO", "REVISAR FIN VIGENCIA", "REVISAR PRIMA"} {
		if !strings.Contains(got, want) {
			t.Fatalf("falta %q en %q", want, got)
		}
	}
}

func TestResumenObservacionesFromNotes_sinDuplicados(t *testing.T) {
	notes := []string{
		validationnotes.Incidencia("REVISAR PRIMA: ESPERADA 8600"),
		validationnotes.Incidencia("REVISAR PRIMA: VALOR 7410 NO PERMITIDO"),
	}
	got := resumenObservacionesFromNotes(notes)
	if got != "REVISAR PRIMA" {
		t.Fatalf("got %q", got)
	}
}

func TestEtiquetasResumenFromNote_cancelacion(t *testing.T) {
	tags := etiquetasResumenFromNote(validationnotes.Incidencia("CANCELACIÓN: CRÉDITO NO ENCONTRADO EN STOCK MAPFRE (OP BT=123)"))
	if len(tags) != 1 || tags[0] != "CANCELACIÓN" {
		t.Fatalf("got %v", tags)
	}
}

func TestEtiquetasResumenFromNote_faltaFechaEspecifica(t *testing.T) {
	cases := []struct {
		note string
		want string
	}{
		{"FALTA FECHA DE NACIMIENTO", "FALTA FECHA NACIMIENTO"},
		{"FALTA FECHA DE ACTIVACIÓN", "FALTA FECHA ACTIVACIÓN"},
		{"FALTA FECHA DE INICIO DE VIGENCIA", "FALTA FECHA INICIO VIGENCIA"},
		{"FALTA FECHA DE FIN DE VIGENCIA", "FALTA FECHA FIN VIGENCIA"},
		{"FALTA FECHA DE ADJUDICACIÓN DEL CRÉDITO", "FALTA FECHA ADJUDICACIÓN"},
		{"FALTA FECHA DE VENCIMIENTO ACTUAL DEL CRÉDITO", "FALTA FECHA VENCIMIENTO"},
		{"CANCELACIÓN: FALTA FECHA PROYECTADA FIN DE VIGENCIA", "FALTA FECHA FIN VIGENCIA"},
		{"CANCELACIÓN: FALTA FECHA DE ACTIVACIÓN", "FALTA FECHA ACTIVACIÓN"},
	}
	for _, tc := range cases {
		t.Run(tc.note, func(t *testing.T) {
			tags := etiquetasResumenFromNote(validationnotes.Incidencia(tc.note))
			found := false
			for _, tag := range tags {
				if tag == tc.want {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("want %q in %v", tc.want, tags)
			}
		})
	}
}

func TestEtiquetasResumenFromNote_cancelacionConFaltaFecha(t *testing.T) {
	tags := etiquetasResumenFromNote(validationnotes.Incidencia("CANCELACIÓN: FALTA FECHA DE ACTIVACIÓN"))
	if len(tags) != 2 {
		t.Fatalf("got %v", tags)
	}
	if tags[0] != "CANCELACIÓN" || tags[1] != "FALTA FECHA ACTIVACIÓN" {
		t.Fatalf("got %v", tags)
	}
}

func TestEtiquetasResumenFromNote_validacionCampoEsEspecifica(t *testing.T) {
	cases := []struct {
		note string
		want string
	}{
		{"El dato «Prima mensual» debe ser numérico.", "REVISAR PRIMA"},
		{"El dato «Edad» debe estar entre 18 y 75.", "REVISAR EDAD"},
		{"El dato «Plazo inicial (meses)» es obligatorio.", "REVISAR PLAZO"},
		{"El dato «Deuda inicial» debe ser mayor que 0.", "REVISAR DEUDA"},
		{"El dato «Valor asegurado» es obligatorio.", "REVISAR VALOR ASEGURADO"},
		{"El valor no está en el catálogo permitido para «Nombre del plan».", "REVISAR PLAN"},
		{"El dato «Identificación del afiliado» es obligatorio.", "REVISAR IDENTIFICACIÓN"},
		{"El dato «Número de crédito u operación (OP BT)» es obligatorio.", "REVISAR CRÉDITO"},
		{"El dato «Fecha de vencimiento actual del crédito» es obligatorio.", "REVISAR VENCIMIENTO"},
	}
	for _, tc := range cases {
		t.Run(tc.note, func(t *testing.T) {
			tags := etiquetasResumenFromNote(validationnotes.Incidencia(tc.note))
			if len(tags) != 1 || tags[0] != tc.want {
				t.Fatalf("want [%s], got %v", tc.want, tags)
			}
		})
	}
}

func TestEtiquetasResumenFromNote_revisarPorCampoEnDefault(t *testing.T) {
	cases := []struct {
		note string
		want string
	}{
		{"Edad fuera de tabla técnica.", "REVISAR EDAD"},
		{"La prima difiere del valor calculado en el archivo.", "REVISAR PRIMA"},
		{"Valor asegurado fuera de rango permitido.", "REVISAR VALOR ASEGURADO"},
	}
	for _, tc := range cases {
		t.Run(tc.note, func(t *testing.T) {
			tags := etiquetasResumenFromNote(validationnotes.Incidencia(tc.note))
			if len(tags) != 1 || tags[0] != tc.want {
				t.Fatalf("want [%s], got %v", tc.want, tags)
			}
		})
	}
}

func TestEtiquetasResumenFromNote_primaDiscrepanciaPlan(t *testing.T) {
	cases := []struct {
		note string
		want string
	}{
		{"La prima (8000) no coincide con el plan «PLAN 1» como prima mensual ni como total del plazo dividido entre meses; primas mensuales permitidas: 8600.", "REVISAR PRIMA (PLAN)"},
		{"La prima mensual es obligatoria para validar el plan «PLAN 1».", "REVISAR PRIMA (PLAN)"},
	}
	for _, tc := range cases {
		t.Run(tc.note, func(t *testing.T) {
			tags := etiquetasResumenFromNote(validationnotes.Incidencia(tc.note))
			if len(tags) != 1 || tags[0] != tc.want {
				t.Fatalf("want [%s], got %v", tc.want, tags)
			}
		})
	}
}

func TestEtiquetasResumenFromNote_defaultGenericoSoloSinCampo(t *testing.T) {
	tags := etiquetasResumenFromNote(validationnotes.Incidencia("Situación no clasificada en la fila."))
	if len(tags) != 1 || tags[0] != "REVISAR" {
		t.Fatalf("want [REVISAR] fallback, got %v", tags)
	}
}
