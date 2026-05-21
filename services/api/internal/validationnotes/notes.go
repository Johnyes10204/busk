package validationnotes

import "strings"

const (
	PrefixIncidencia  = "Incidencia: "
	PrefixInformativo = "Informe: "
)

func IsBlocking(n string) bool {
	n = strings.TrimSpace(n)
	return strings.HasPrefix(n, PrefixIncidencia) || strings.HasPrefix(n, "NOVEDAD:")
}

func IsInformative(n string) bool {
	return strings.HasPrefix(strings.TrimSpace(n), PrefixInformativo)
}

func Incidencia(msg string) string {
	msg = strings.TrimSpace(msg)
	for strings.HasPrefix(msg, PrefixIncidencia) {
		msg = strings.TrimSpace(strings.TrimPrefix(msg, PrefixIncidencia))
	}
	if strings.HasPrefix(strings.ToUpper(msg), "NOVEDAD:") {
		msg = strings.TrimSpace(msg[7:])
	}
	return PrefixIncidencia + msg
}

func Informativo(msg string) string {
	msg = strings.TrimSpace(msg)
	for strings.HasPrefix(msg, PrefixInformativo) {
		msg = strings.TrimSpace(strings.TrimPrefix(msg, PrefixInformativo))
	}
	return PrefixInformativo + msg
}

// Split separa notas de validation_json en incidencias (bloquean carga) e informes (solo aviso).
// Texto sin prefijo conocido se trata como informativo (compatibilidad con notas blandas históricas).
func Split(notes []string) (blocking, informative []string) {
	for _, n := range notes {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		switch {
		case IsInformative(n):
			informative = append(informative, n)
		case IsBlocking(n):
			blocking = append(blocking, n)
		default:
			informative = append(informative, n)
		}
	}
	return blocking, informative
}
