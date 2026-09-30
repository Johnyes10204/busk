package notify

import (
	"strings"
	"testing"
	"time"
)

func inputDePrueba() FileEmailInput {
	return FileEmailInput{
		FileID:               "abc123def456",
		FileName:             "INCLUSION-2026-MAPFRE.xlsx",
		ProductID:            "mapfre_vida_voluntario",
		Status:               "ERROR",
		ErrorReason:          "archivo con novedades de validación",
		ValidationReportJSON: `{"policy_row_count":120,"total_pending_validations":7,"total_duplicate_credits":2,"total_duplicate_rows":3}`,
		ProcessedAt:          time.Date(2026, 4, 15, 10, 30, 0, 0, time.UTC),
	}
}

const urlPrueba = "http://62.146.228.79/api/v1/files/validation-xlsx?file_id=abc123def456"

func conEnv(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value)
}

// El enlace de descarga debe ir en todos los correos, con y sin adjunto:
// ese es el contrato que se pidió para el equipo operativo.

func TestBuildPlainBody_EnlaceDescargaSiempreConAdjuntoEspejo(t *testing.T) {
	conEnv(t, "API_PUBLIC_BASE_URL", "http://62.146.228.79")
	in := inputDePrueba()
	url := validationReportDownloadURL(in.FileID)

	body := buildPlainBody(in, true, false, false, url)

	if !strings.Contains(body, url) {
		t.Fatalf("el cuerpo con adjunto espejo debe incluir el enlace de descarga:\n%s", body)
	}
	if !strings.Contains(body, "Adjunto: Excel espejo") {
		t.Fatalf("el cuerpo con adjunto espejo debe mencionar el adjunto:\n%s", body)
	}
}

func TestBuildPlainBody_EnlaceDescargaSiempreSinAdjunto(t *testing.T) {
	conEnv(t, "API_PUBLIC_BASE_URL", "http://62.146.228.79")
	in := inputDePrueba()
	url := validationReportDownloadURL(in.FileID)

	body := buildPlainBody(in, false, false, false, url)

	if !strings.Contains(body, url) {
		t.Fatalf("el cuerpo sin adjunto debe incluir el enlace de descarga:\n%s", body)
	}
}

func TestBuildPlainBody_EnlaceDescargaSiempreReporteMuyGrande(t *testing.T) {
	conEnv(t, "API_PUBLIC_BASE_URL", "http://62.146.228.79")
	in := inputDePrueba()
	url := validationReportDownloadURL(in.FileID)

	body := buildPlainBody(in, false, false, true, url)

	if !strings.Contains(body, url) {
		t.Fatalf("el cuerpo con reporte demasiado grande debe incluir el enlace de descarga:\n%s", body)
	}
	if !strings.Contains(body, "~28 MB") {
		t.Fatalf("debe maintained el aviso de tamaño máximo del proveedor:\n%s", body)
	}
}

func TestBuildHTMLBody_EnlaceDescargaSiempre(t *testing.T) {
	conEnv(t, "API_PUBLIC_BASE_URL", "http://62.146.228.79")
	in := inputDePrueba()
	url := validationReportDownloadURL(in.FileID)

	casos := []struct {
		nombre              string
		adjuntoExcel        bool
		adjuntoOriginal     bool
		reporteMuyGrande    bool
		debeMencionarAccion string
	}{
		{"con_espejo", true, false, false, "revise el Excel adjunto"},
		{"con_original", false, true, false, "revise el archivo original adjunto"},
		{"sin_adjunto", false, false, false, "no lleva adjunto"},
		{"reporte_grande", false, false, true, "supera el tamaño máximo"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			html := buildHTMLBody(in, c.adjuntoExcel, c.adjuntoOriginal, c.reporteMuyGrande, url)
			if !strings.Contains(html, url) {
				t.Fatalf("el HTML debe incluir el enlace de descarga:\n%s", html)
			}
			if !strings.Contains(html, `href="`+url+`"`) {
				t.Fatalf("el HTML debe incluir el enlace como ancla descargable:\n%s", html)
			}
			if !strings.Contains(html, c.debeMencionarAccion) {
				t.Fatalf("el bloque de acción requerida debe decir %q:\n%s", c.debeMencionarAccion, html)
			}
			if strings.Contains(html, "{{") {
				t.Fatalf("el HTML no debe quedar con marcadores sin resolver:\n%s", html)
			}
		})
	}
}

func TestBuildSuccessPlainBody_EnlaceDescargaSiempreConYSinAdjunto(t *testing.T) {
	conEnv(t, "API_PUBLIC_BASE_URL", "http://62.146.228.79")
	in := inputDePrueba()
	in.Status = "PROCESSED"
	url := validationReportDownloadURL(in.FileID)

	for _, conAdjuntos := range []bool{true, false} {
		body := buildSuccessPlainBody(in, conAdjuntos, false, url)
		if !strings.Contains(body, url) {
			t.Fatalf("adjuntos=%t: el correo de éxito debe incluir el enlace de descarga:\n%s", conAdjuntos, body)
		}
	}
}

func TestBuildSuccessHTMLBody_EnlaceDescargaSiempre(t *testing.T) {
	conEnv(t, "API_PUBLIC_BASE_URL", "http://62.146.228.79")
	in := inputDePrueba()
	in.Status = "PROCESSED"
	url := validationReportDownloadURL(in.FileID)

	for _, conAdjuntos := range []bool{true, false} {
		html := buildSuccessHTMLBody(in, conAdjuntos, false, url)
		if !strings.Contains(html, url) {
			t.Fatalf("adjuntos=%t: el HTML de éxito debe incluir el enlace de descarga:\n%s", conAdjuntos, html)
		}
		if strings.Contains(html, "{{") {
			t.Fatalf("adjuntos=%t: el HTML no debe quedar con marcadores sin resolver:\n%s", conAdjuntos, html)
		}
	}
}

func TestValidationReportDownloadURL_BaseYFallback(t *testing.T) {
	fileID := "abc123def456"
	ruta := "/api/v1/files/validation-xlsx?file_id=" + fileID

	t.Run("usa_API_PUBLIC_BASE_URL", func(t *testing.T) {
		t.Setenv("BUSK_PUBLIC_API_URL", "")
		t.Setenv("API_PUBLIC_BASE_URL", "http://62.146.228.79")
		if got := validationReportDownloadURL(fileID); got != "http://62.146.228.79"+ruta {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("normaliza_barra_final", func(t *testing.T) {
		t.Setenv("BUSK_PUBLIC_API_URL", "")
		t.Setenv("API_PUBLIC_BASE_URL", "http://62.146.228.79/")
		if got := validationReportDownloadURL(fileID); got != "http://62.146.228.79"+ruta {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("fallback_BUSK_PUBLIC_API_URL", func(t *testing.T) {
		t.Setenv("API_PUBLIC_BASE_URL", "")
		t.Setenv("BUSK_PUBLIC_API_URL", "https://api.buskseguros.com")
		if got := validationReportDownloadURL(fileID); got != "https://api.buskseguros.com"+ruta {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("sin_base_devuelve_vacio", func(t *testing.T) {
		t.Setenv("API_PUBLIC_BASE_URL", "")
		t.Setenv("BUSK_PUBLIC_API_URL", "")
		if got := validationReportDownloadURL(fileID); got != "" {
			t.Fatalf("got %q, se esperaba vacío", got)
		}
	})
}

func TestValidationReportDownloadURL_FileIDVacio(t *testing.T) {
	t.Setenv("API_PUBLIC_BASE_URL", "http://62.146.228.79")
	if got := validationReportDownloadURL("   "); got != "" {
		t.Fatalf("got %q, se esperaba vacío", got)
	}
}
