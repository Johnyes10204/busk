package store

import (
	"sort"
	"strings"

	"github.com/buskseguros-design/services/api/internal/validationnotes"
)

// resumenObservacionesFromNotes agrupa las novedades en etiquetas cortas separadas por coma
// (para filtrar en Excel; el detalle queda en la columna novedades).
func resumenObservacionesFromNotes(notes []string) string {
	seen := make(map[string]struct{})
	tags := make([]string, 0)
	for _, n := range notes {
		for _, tag := range etiquetasResumenFromNote(n) {
			if tag == "" {
				continue
			}
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			tags = append(tags, tag)
		}
	}
	sort.Strings(tags)
	return strings.Join(tags, ", ")
}

func etiquetasResumenFromNote(note string) []string {
	t := strings.ToUpper(strings.TrimSpace(validationnotes.DisplayText(note)))
	if t == "" {
		return nil
	}

	var tags []string
	add := func(tag string) {
		if tag != "" {
			tags = append(tags, tag)
		}
	}

	switch {
	case strings.Contains(t, "PRIMA CERO") && strings.Contains(t, "CONGEL"):
		add("CONGELAMIENTO")
	case strings.Contains(t, "PRIMA CERO") && strings.Contains(t, "CANCEL"):
		add("CANCELACIÓN PRIMA CERO")
	case strings.HasPrefix(t, "CANCELACIÓN:"):
		add("CANCELACIÓN")
		if idx := strings.Index(t, "FALTA "); idx >= 0 {
			add(etiquetaResumenFalta(t[idx:]))
		}
	case strings.Contains(t, "ARCHIVO: POSIBLE CANCELACIÓN"):
		add("AVISO ARCHIVO CANCELACIÓN")
	case strings.Contains(t, "REVISAR FIN VIGENCIA"):
		add("REVISAR FIN VIGENCIA")
	case strings.Contains(t, "INICIO VIGENCIA FUERA DEL MES ESPERADO"):
		add("INICIO VIGENCIA FUERA DEL MES ESPERADO")
	case strings.Contains(t, "PRIMA") && (strings.Contains(t, "NO COINCIDE CON EL PLAN") ||
		strings.Contains(t, "OBLIGATORIA PARA VALIDAR EL PLAN")):
		add("REVISAR PRIMA (PLAN)")
	case strings.Contains(t, "REVISAR PRIMA"):
		add("REVISAR PRIMA")
	case strings.Contains(t, "REVISAR PLAZO"):
		add("REVISAR PLAZO")
	case strings.Contains(t, "REVISAR EDAD"):
		add("REVISAR EDAD")
	case strings.Contains(t, "REVISAR DEUDA"):
		add("REVISAR DEUDA")
	case strings.Contains(t, "PLAN NO VÁLIDO") || strings.Contains(t, "NOMBRE DEL PLAN") || strings.Contains(t, "PLAN ES OBLIGATORIO"):
		add("REVISAR PLAN")
	case strings.Contains(t, "VALOR ASEGURADO"):
		add("REVISAR VALOR ASEGURADO")
	case strings.Contains(t, "OP BT DUPLICADO") || strings.Contains(t, "CRÉDITO U OPERACIÓN") && strings.Contains(t, "APARECE"):
		add("OP BT DUPLICADO")
	case strings.HasPrefix(t, "FALTA "):
		add(etiquetaResumenFalta(t))
	case strings.Contains(t, "VENCIMIENTO (E.10)"):
		add("VENCIMIENTO MES ANTERIOR")
	case strings.Contains(t, "VENCIMIENTO"):
		add("REVISAR VENCIMIENTO")
	case strings.Contains(t, "FECHA AMBIGUA"):
		add("FECHA AMBIGUA")
	case strings.Contains(t, "FECHA NO VÁLIDA") || strings.Contains(t, "FECHA ACTIVACIÓN NO VÁLIDA") || strings.Contains(t, "FECHA NACIMIENTO NO VÁLIDA"):
		add("FECHA NO VÁLIDA")
	case strings.Contains(t, "ES OBLIGATORIO") || strings.Contains(t, "DEBE SER NUMÉRICO") ||
		strings.Contains(t, "DEBE SER MAYOR") || strings.Contains(t, "DEBE ESTAR ENTRE") ||
		strings.Contains(t, "NO ESTÁ EN EL CATÁLOGO"):
		if tag := revisarTagPorCampo(t); tag != "" {
			add(tag)
		} else {
			add("VALIDACIÓN CAMPO")
		}
	case strings.Contains(t, "REVISIÓN MANUAL") || strings.Contains(t, "INCIDENCIA SIN DETALLE"):
		add("REVISIÓN MANUAL")
	case strings.Contains(t, "REGLA DE VALIDACIÓN NO RECONOCIDA"):
		add("CONFIGURACIÓN REGLA")
	default:
		if tag := revisarTagPorCampo(t); tag != "" {
			add(tag)
		} else {
			add("REVISAR")
		}
	}
	return tags
}

// revisarTagPorCampo devuelve un tag específico (REVISAR EDAD, REVISAR PRIMA, …)
// cuando reconoce el campo en el texto de la nota (preferentemente entre «»,
// con un respaldo de palabras clave). Devuelve "" si no encuentra coincidencia.
func revisarTagPorCampo(t string) string {
	if tag := revisarTagFromCampoLiteral(extraerCampoEntreComillas(t)); tag != "" {
		return tag
	}
	switch {
	case strings.Contains(t, "VALOR ASEGURADO"):
		return "REVISAR VALOR ASEGURADO"
	case strings.Contains(t, "FIN DE VIGENCIA") || strings.Contains(t, "FIN VIGENCIA"):
		return "REVISAR FIN VIGENCIA"
	case strings.Contains(t, "INICIO DE VIGENCIA") || strings.Contains(t, "INICIO VIGENCIA"):
		return "REVISAR INICIO VIGENCIA"
	case strings.Contains(t, "EDAD"):
		return "REVISAR EDAD"
	case strings.Contains(t, "PRIMA"):
		return "REVISAR PRIMA"
	case strings.Contains(t, "DEUDA"):
		return "REVISAR DEUDA"
	case strings.Contains(t, "PLAZO"):
		return "REVISAR PLAZO"
	case strings.Contains(t, "VENCIMIENTO"):
		return "REVISAR VENCIMIENTO"
	case strings.Contains(t, "PLAN"):
		return "REVISAR PLAN"
	case strings.Contains(t, "IDENTIFICACIÓN"):
		return "REVISAR IDENTIFICACIÓN"
	case strings.Contains(t, "CRÉDITO") || strings.Contains(t, "OP BT"):
		return "REVISAR CRÉDITO"
	}
	return ""
}

func extraerCampoEntreComillas(t string) string {
	open := strings.Index(t, "«")
	if open < 0 {
		return ""
	}
	rest := t[open+len("«"):]
	close := strings.Index(rest, "»")
	if close < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:close])
}

func revisarTagFromCampoLiteral(campo string) string {
	c := strings.ToUpper(strings.TrimSpace(campo))
	if c == "" {
		return ""
	}
	switch {
	case strings.Contains(c, "VALOR ASEGURADO"):
		return "REVISAR VALOR ASEGURADO"
	case strings.Contains(c, "FIN DE VIGENCIA") || strings.Contains(c, "FIN VIGENCIA"):
		return "REVISAR FIN VIGENCIA"
	case strings.Contains(c, "INICIO DE VIGENCIA") || strings.Contains(c, "INICIO VIGENCIA"):
		return "REVISAR INICIO VIGENCIA"
	case strings.Contains(c, "EDAD"):
		return "REVISAR EDAD"
	case strings.Contains(c, "PRIMA"):
		return "REVISAR PRIMA"
	case strings.Contains(c, "DEUDA"):
		return "REVISAR DEUDA"
	case strings.Contains(c, "PLAZO"):
		return "REVISAR PLAZO"
	case strings.Contains(c, "VENCIMIENTO"):
		return "REVISAR VENCIMIENTO"
	case strings.Contains(c, "PLAN"):
		return "REVISAR PLAN"
	case strings.Contains(c, "IDENTIFICACIÓN"):
		return "REVISAR IDENTIFICACIÓN"
	case strings.Contains(c, "CRÉDITO") || strings.Contains(c, "OP BT"):
		return "REVISAR CRÉDITO"
	case strings.Contains(c, "FECHA"):
		return "REVISAR FECHA"
	}
	return ""
}

// etiquetaResumenFalta acorta «FALTA FECHA DE …» a una etiqueta filtrable con el campo concreto.
func etiquetaResumenFalta(note string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(note)), "FALTA "))
	if rest == "" {
		return "FALTA FECHA"
	}

	known := []struct {
		prefix string
		tag    string
	}{
		{"FECHA DE VENCIMIENTO ACTUAL DEL CRÉDITO", "FALTA FECHA VENCIMIENTO"},
		{"FECHA DE ADJUDICACIÓN DEL CRÉDITO", "FALTA FECHA ADJUDICACIÓN"},
		{"FECHA PROYECTADA FIN DE VIGENCIA", "FALTA FECHA FIN VIGENCIA"},
		{"FECHA DE INICIO DE VIGENCIA", "FALTA FECHA INICIO VIGENCIA"},
		{"FECHA DE FIN DE VIGENCIA", "FALTA FECHA FIN VIGENCIA"},
		{"FECHA DE NACIMIENTO", "FALTA FECHA NACIMIENTO"},
		{"FECHA DE ACTIVACIÓN", "FALTA FECHA ACTIVACIÓN"},
		{"FECHA DE ADJUDICACIÓN", "FALTA FECHA ADJUDICACIÓN"},
	}
	for _, k := range known {
		if rest == k.prefix || strings.HasPrefix(rest, k.prefix) {
			return k.tag
		}
	}

	if strings.HasPrefix(rest, "FECHA DE ") {
		detail := strings.TrimPrefix(rest, "FECHA DE ")
		detail = strings.TrimSuffix(detail, " DEL CRÉDITO")
		detail = strings.TrimSuffix(detail, " ACTUAL DEL CRÉDITO")
		if detail != "" {
			return "FALTA FECHA " + detail
		}
	}
	if strings.HasPrefix(rest, "FECHA ") {
		return "FALTA " + rest
	}

	return "FALTA FECHA"
}

// observacionesForRow resume etiquetas; si no hay notas, infiere desde el estado o mensaje por defecto.
func observacionesForRow(notes []string, policyStatus string) string {
	obs := resumenObservacionesFromNotes(notes)
	if obs != "" {
		return obs
	}
	fallback := defaultPendingRowDetailMessage(policyStatus, len(notes) > 0)
	if strings.TrimSpace(fallback) == "" {
		return ""
	}
	return resumenObservacionesFromNotes([]string{fallback})
}
