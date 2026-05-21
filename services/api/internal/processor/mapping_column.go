package processor

import (
	"strings"

	"github.com/buskseguros-design/services/api/internal/model"
)

// columnIndexForFieldMap resuelve la columna del encabezado para un mapeo canónico.
// Evita colisiones cuando varias columnas se normalizan igual (ej. "%" y "% " en Bolívar Banco).
func columnIndexForFieldMap(header []string, m model.FieldMap) (int, bool) {
	source := strings.TrimSpace(m.SourceHeader)
	if source == "" {
		return -1, false
	}
	field := strings.TrimSpace(m.CanonicalField)

	// Tasa de prima mensual: solo la columna "%" sin espacios trailing (no "% " ni "Valor servicio").
	if field == "rate_percent" && source == "%" {
		for i, h := range header {
			if strings.TrimSpace(h) != "%" {
				continue
			}
			if strings.HasSuffix(h, " ") || strings.HasSuffix(h, "\t") {
				continue
			}
			return i, true
		}
		return -1, false
	}

	target := strings.ToUpper(source)
	for i, h := range header {
		if strings.ToUpper(strings.TrimSpace(h)) == target {
			return i, true
		}
	}
	return -1, false
}
