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

func TestBuildHTMLBody_DiagnosticoMultilineaSeRenderiza(t *testing.T) {
	conEnv(t, "API_PUBLIC_BASE_URL", "http://62.146.228.79")
	in := inputDePrueba()
	in.ErrorReason = "no existe producto configurado para el prefijo del archivo\n" +
		"Causa: el nombre del archivo sí coincide con un formato, pero está DESACTIVADO (active=0): " +
		"prefijo \"INCLUSION-NUEVO\" en el formato MAPFRE_INCLUSION_AP_MENORES.\n" +
		"Acción: active el formato en /api/v1/product-formats."
	url := validationReportDownloadURL(in.FileID)

	html := buildHTMLBody(in, false, false, false, url)

	// Los saltos de línea deben convertirse en <br>; si no, el correo llega como un
	// bloque compacto ilegible y el diagnóstico se pierde.
	if n := strings.Count(html, "Causa: el nombre del archivo"); n != 1 {
		t.Fatalf("el diagnóstico debe aparecer una vez, apareció %d", n)
	}
	if !strings.Contains(html, "active=0") {
		t.Fatalf("debe conservar el motivo técnico:\n%s", html)
	}
	if !strings.Contains(html, "Acción: active el formato") {
		t.Fatalf("debe conservar la acción sugerida:\n%s", html)
	}
	// El separador entre el motivo genérico y el diagnóstico debe ser un <br>.
	if !strings.Contains(html, "prefijo del archivo<br>") {
		t.Fatalf("los saltos de línea del detalle deben convertirse en <br>:\n%s", html)
	}
	// Y debe seguir escapado: nada de HTML inyectado desde el motivo.
	if strings.Contains(html, "<script>") {
		t.Fatalf("el detalle no debe interpretarse como HTML")
	}
}

func TestBuildHTMLBody_EscapaHTMLEnElDiagnostico(t *testing.T) {
	conEnv(t, "API_PUBLIC_BASE_URL", "http://62.146.228.79")
	in := inputDePrueba()
	in.ErrorReason = "prefijo <b>MICRO_BANCO</b> & \"comillas\" <img src=x onerror=alert(1)>"
	url := validationReportDownloadURL(in.FileID)

	html := buildHTMLBody(in, false, false, false, url)

	if strings.Contains(html, "<b>MICRO_BANCO</b>") {
		t.Fatalf("el HTML del motivo debe venir escapado:\n%s", html)
	}
	if strings.Contains(html, "<img src=x") {
		t.Fatalf("no debe permitir inyección de etiquetas:\n%s", html)
	}
	if !strings.Contains(html, "&lt;b&gt;MICRO_BANCO&lt;/b&gt;") {
		t.Fatalf("debe escapar las etiquetas del motivo:\n%s", html)
	}
}

func TestDownloadURLForFile_SinReporteApuntaAlOriginal(t *testing.T) {
	conEnv(t, "API_PUBLIC_BASE_URL", "http://62.146.228.79")

	// Es el caso real de MICRO_BANCO.xlsx: no se identifica el producto, no hay reporte, y el
	// enlace debe llevar al original. Apuntar a /validation-xlsx devolvería 404.
	in := inputDePrueba()
	in.ReportArchivePath = ""

	got := downloadURLForFile(in)

	want := "http://62.146.228.79/api/v1/files/download?file_id=" + in.FileID
	if got != want {
		t.Fatalf("sin reporte el enlace debe ser el original:\n got=%q\nwant=%q", got, want)
	}
}

func TestDownloadURLForFile_ConReporteApuntaAlReporte(t *testing.T) {
	conEnv(t, "API_PUBLIC_BASE_URL", "http://62.146.228.79")

	in := inputDePrueba()
	in.ReportArchivePath = "./data/reports-archive/reporte.xlsx"

	got := downloadURLForFile(in)

	want := "http://62.146.228.79/api/v1/files/validation-xlsx?file_id=" + in.FileID
	if got != want {
		t.Fatalf("con reporte el enlace debe ser el XLSX:\n got=%q\nwant=%q", got, want)
	}
}

func TestDownloadURLForFile_ReportArchivePathSoloEspaciosCuentaComoAusente(t *testing.T) {
	conEnv(t, "API_PUBLIC_BASE_URL", "http://62.146.228.79")

	in := inputDePrueba()
	in.ReportArchivePath = "   \n\t "

	if !strings.Contains(downloadURLForFile(in), "/api/v1/files/download?") {
		t.Fatalf("una ruta en blanco no es un reporte existente; debe caer al original")
	}
}

func TestDownloadURLForFile_SinBaseDevuelveVacio(t *testing.T) {
	conEnv(t, "API_PUBLIC_BASE_URL", "")
	conEnv(t, "BUSK_PUBLIC_API_URL", "")

	if got := downloadURLForFile(inputDePrueba()); got != "" {
		t.Fatalf("sin URL pública no se debe inventar un enlace: %q", got)
	}
}

func TestNoopNotifier_NoReportaAdjuntoEntregado(t *testing.T) {
	// El noop se usa cuando falta la config de SendGrid. Si reportara Attached=true el
	// processor borraría el archivo pensando que ya viaja en un correo que nunca salió.
	d, err := (&noopNotifier{}).NotifyFileProcessing(inputDePrueba())
	if err != nil {
		t.Fatalf("noop no debe fallar: %v", err)
	}
	if d.Attached {
		t.Fatalf("el noop no entrega nada: Attached debe ser false")
	}
}

func TestNotifyFileProcessing_EstadoNoTerminalNoEnvía(t *testing.T) {
	in := inputDePrueba()
	in.Status = "PROCESSING"
	d, err := (&noopNotifier{}).NotifyFileProcessing(in)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if d.Attached || d.DownloadURL != "" {
		t.Fatalf("un estado no terminal no debe reportar entrega: %+v", d)
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
