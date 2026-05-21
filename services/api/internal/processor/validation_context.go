package processor

import (
	"fmt"
	"strings"
	"time"

	"github.com/buskseguros-design/services/api/internal/model"
)

const fieldHeaderPrefix = "_hdr_"

// attachFieldHeaders guarda en values el encabezado del archivo por campo canónico (_hdr_<campo>).
func attachFieldHeaders(values map[string]string, mappings []model.FieldMap) {
	for _, m := range mappings {
		canon := strings.TrimSpace(m.CanonicalField)
		hdr := strings.TrimSpace(m.SourceHeader)
		if canon == "" || hdr == "" {
			continue
		}
		values[fieldHeaderPrefix+canon] = hdr
	}
}

type campoArchivo struct {
	Canon  string
	Header string
	Valor  string
}

func campoDesdeValues(values map[string]string, canon string) campoArchivo {
	hdr := strings.TrimSpace(values[fieldHeaderPrefix+canon])
	if hdr == "" {
		hdr = etiquetaCampoCanónico(canon)
	}
	val := strings.TrimSpace(values[canon])
	if val == "" && canon == "rate_percent" {
		for _, key := range []string{"%", "% "} {
			if v := strings.TrimSpace(values[key]); v != "" {
				if hdr == etiquetaCampoCanónico(canon) {
					hdr = key
				}
				val = v
				break
			}
		}
	}
	return campoArchivo{Canon: canon, Header: hdr, Valor: val}
}

func lineaColumnaValor(c campoArchivo) string {
	if c.Valor == "" {
		return fmt.Sprintf("columna «%s» (vacío)", c.Header)
	}
	return fmt.Sprintf("columna «%s» valor «%s»", c.Header, c.Valor)
}

func lineaValorUsado(usado string) string {
	usado = strings.TrimSpace(usado)
	if usado == "" {
		return ""
	}
	return fmt.Sprintf("valor usado «%s»", usado)
}

func lineaReferencia(esperado string) string {
	esperado = strings.TrimSpace(esperado)
	if esperado == "" {
		return ""
	}
	return fmt.Sprintf("referencia «%s»", esperado)
}

func mensajeConContexto(msg string, partes ...string) string {
	var ctx []string
	for _, p := range partes {
		p = strings.TrimSpace(p)
		if p != "" {
			ctx = append(ctx, p)
		}
	}
	if len(ctx) == 0 {
		return msg
	}
	return msg + " (" + strings.Join(ctx, "; ") + ")"
}

func mesAnioRef(y int, m time.Month) string {
	return fmt.Sprintf("%02d/%d", m, y)
}

func archivoFacturacionRef(values map[string]string) string {
	fn := strings.TrimSpace(values["_file_name"])
	if fn == "" {
		return ""
	}
	if y, m, ok := bolivarMesFacturacionDesdeArchivo(fn); ok {
		return fmt.Sprintf("mes facturación archivo «%s» (%s)", fn, mesAnioRef(y, m))
	}
	return fmt.Sprintf("archivo «%s»", fn)
}
