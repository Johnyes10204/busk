package notify

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Política: si el contenido excede el límite de SendGrid, NO se propone ningún adjunto
// (ni XLSX ni ZIP). El correo debe salir solo con el enlace de descarga.
func TestEmailAttachmentCandidates_SobreLimiteNoProponeNada(t *testing.T) {
	grande := make([]byte, maxEmailAttachmentBytes+1)

	got := emailAttachmentCandidates("reporte.xlsx", "file_1", grande)

	if len(got) != 0 {
		names := make([]string, 0, len(got))
		for _, a := range got {
			names = append(names, a.filename)
		}
		t.Fatalf("sobre el límite no debe proponer adjuntos, propuso %v", names)
	}
}

func TestEmailAttachmentCandidates_ExactoEnElLimiteSiSeAdjunta(t *testing.T) {
	// El límite es inclusivo: exactamente maxEmailAttachmentBytes todavía cabe.
	exacto := make([]byte, maxEmailAttachmentBytes)

	got := emailAttachmentCandidates("reporte.xlsx", "file_1", exacto)

	if len(got) == 0 {
		t.Fatalf("un archivo justo en el límite sí debe adjuntarse")
	}
	for _, a := range got {
		if len(a.data) > maxEmailAttachmentBytes {
			t.Fatalf("%s excede el límite: %d", a.filename, len(a.data))
		}
	}
}

func TestEmailAttachmentCandidates_PequenoSinZipPrimero(t *testing.T) {
	// 100 KiB está bajo emailZipPreferMinBytes: el ZIP no se prefiere, solo queda como
	// respaldo ante un 413 de nginx. El XLSX va primero.
	peq := bytes.Repeat([]byte("a"), 100*1024)

	got := emailAttachmentCandidates("reporte.xlsx", "file_1", peq)

	if len(got) == 0 {
		t.Fatalf("un archivo pequeño sí debe adjuntarse")
	}
	if !strings.HasSuffix(got[0].filename, ".xlsx") {
		t.Fatalf("el XLSX debe ir primero, iba %q", got[0].filename)
	}
}

// Un payload que ya roza el límite no debe gastar memoria construir un ZIP que no lo
// salvaría: el proceso ya sufre presión de memoria al cargar los libros.
func TestEmailAttachmentCandidates_CercaDelLimiteNoConstruyeZip(t *testing.T) {
	// 20 MiB: cabe en el límite pero supera la mitad, así que el ZIP no se intenta.
	pesado := bytes.Repeat([]byte("c"), 20<<20)

	got := emailAttachmentCandidates("reporte.xlsx", "file_1", pesado)

	if len(got) != 1 {
		t.Fatalf("cerca del límite se esperaba solo el XLSX, hubo %d adjuntos", len(got))
	}
	if !strings.HasSuffix(got[0].filename, ".xlsx") {
		t.Fatalf("se esperaba el XLSX, hubo %q", got[0].filename)
	}
}

func TestEmailAttachmentCandidates_GrandePrefiereZipPrimero(t *testing.T) {
	// 3 MiB: supera emailZipPreferMinBytes pero cabe en el límite, así que se intenta el ZIP
	// primero para ahorrar ancho de banda.
	mediano := bytes.Repeat([]byte("b"), 3<<20)

	got := emailAttachmentCandidates("reporte.xlsx", "file_1", mediano)

	if len(got) < 2 {
		t.Fatalf("un archivo de 3 MiB debería offerser XLSX y ZIP, hubo %d", len(got))
	}
	if !strings.HasSuffix(got[0].filename, ".zip") {
		t.Fatalf("el ZIP debe ir primero, iba %q", got[0].filename)
	}
	if len(got[0].data) > maxEmailAttachmentBytes {
		t.Fatalf("el ZIP no debe exceder el límite: %d", len(got[0].data))
	}
}

// processedSuccessAttachments debe señalar linkOnly cuando el espejo no cabe, para que el
// correo de éxito salga sin adjunto en vez de caer al resumen mínimo.
func TestProcessedSuccessAttachments_EspejoSobreLimiteIndicaLinkOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reporte.xlsx")
	if err := os.WriteFile(path, make([]byte, maxEmailAttachmentBytes+1), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	n := &sendGridNotifier{}
	in := FileEmailInput{FileID: "file_1", FileName: "x.xlsx", ReportArchivePath: path}

	atts, _, linkOnly, err := n.processedSuccessAttachments(in)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !linkOnly {
		t.Fatalf("un espejo sobre el límite debe activar linkOnly")
	}
	if len(atts) != 0 {
		t.Fatalf("no debe haber candidatos de adjunto cuando linkOnly, hubo %d", len(atts))
	}
}

func TestProcessedSuccessAttachments_SinEspejoNoEsLinkOnly(t *testing.T) {
	n := &sendGridNotifier{}
	in := FileEmailInput{FileID: "file_1", FileName: "x.xlsx", ReportArchivePath: ""}

	atts, hasNovedades, linkOnly, err := n.processedSuccessAttachments(in)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if linkOnly {
		t.Fatalf("sin espejo no es un caso de límite, el flujo normal sigue válido")
	}
	if len(atts) != 0 || hasNovedades {
		t.Fatalf("atts=%d hasNovedades=%t", len(atts), hasNovedades)
	}
}
