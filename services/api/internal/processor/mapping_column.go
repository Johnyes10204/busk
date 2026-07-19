package processor

import (
	"strings"

	"github.com/buskseguros-design/services/api/internal/model"
)

// columnIndexForFieldMap resuelve la columna del encabezado para un mapeo canónico.
// Prueba SourceHeader primero; si no lo encuentra intenta cada Aliases en orden.
// Evita colisiones cuando varias columnas se normalizan igual (ej. "%" y "% " en Bolívar Banco).
func columnIndexForFieldMap(header []string, m model.FieldMap) (int, bool) {
	candidates := append([]string{m.SourceHeader}, m.Aliases...)
	for _, source := range candidates {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}

		// Tasa de prima mensual: solo la columna "%" sin espacios trailing.
		if strings.TrimSpace(m.CanonicalField) == "rate_percent" && source == "%" {
			for i, h := range header {
				if strings.TrimSpace(h) != "%" {
					continue
				}
				if strings.HasSuffix(h, " ") || strings.HasSuffix(h, "\t") {
					continue
				}
				return i, true
			}
			continue
		}

		target := strings.ToUpper(source)
		for i, h := range header {
			if strings.ToUpper(strings.TrimSpace(h)) == target {
				return i, true
			}
		}
	}
	return -1, false
}
