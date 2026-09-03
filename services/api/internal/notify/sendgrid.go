package notify

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/buskseguros-design/services/api/internal/store"
	"github.com/sendgrid/sendgrid-go"
	sgmail "github.com/sendgrid/sendgrid-go/helpers/mail"
)

const (
	maxEmailAttachmentBytes = 28 << 20 // SendGrid admite ~30 MiB
	emailZipPreferMinBytes  = 2 << 20 // archivos grandes: intentar ZIP primero (evita 413 de nginx)
)

type FileEmailInput struct {
	FileID               string
	FileName             string
	ProductID            string
	Status               string
	ErrorReason          string
	ValidationReportJSON string
	ArchivePath          string
	ReportArchivePath    string
	ProcessedAt          time.Time
}

// ErrorEmailInput mantiene compatibilidad con referencias previas.
type ErrorEmailInput = FileEmailInput

type FileNotifier interface {
	NotifyFileProcessing(input FileEmailInput) error
}

// ErrorNotifier mantiene compatibilidad con referencias previas.
type ErrorNotifier = FileNotifier

type noopNotifier struct{}

func (n *noopNotifier) NotifyFileProcessing(input FileEmailInput) error {
	return nil
}

type sendGridNotifier struct {
	apiKey     string
	from       string
	recipients []string
}

func NewFileNotifierFromEnv() FileNotifier {
	apiKey := strings.TrimSpace(os.Getenv("SENDGRID_API_KEY"))
	from := strings.TrimSpace(os.Getenv("SENDGRID_FROM_EMAIL"))
	recipients := splitRecipients(os.Getenv("SENDGRID_ERROR_TO_EMAILS"))
	if apiKey == "" || from == "" || len(recipients) == 0 {
		log.Printf("[notify] sendgrid deshabilitado (faltan SENDGRID_API_KEY, SENDGRID_FROM_EMAIL o SENDGRID_ERROR_TO_EMAILS)")
		return &noopNotifier{}
	}
	return &sendGridNotifier{
		apiKey:     apiKey,
		from:       from,
		recipients: recipients,
	}
}

// NewErrorNotifierFromEnv es alias de NewFileNotifierFromEnv.
func NewErrorNotifierFromEnv() FileNotifier {
	return NewFileNotifierFromEnv()
}

func (n *sendGridNotifier) NotifyFileProcessing(input FileEmailInput) error {
	// Invariante: todo archivo procesado (PROCESSED/ERROR/SKIPPED) debe generar un correo con
	// al menos un adjunto (archivo original, reporte de novedades o resumen mínimo).
	switch strings.ToUpper(strings.TrimSpace(input.Status)) {
	case "PROCESSED":
		return n.notifyProcessedSuccess(input)
	case "ERROR", "SKIPPED":
		return n.notifyProcessingError(input)
	default:
		return nil
	}
}

// originalFromArchive lee la copia local del archivo original que el procesador guardó al
// descargarlo del SFTP (rec.ArchivePath) y la devuelve como emailAttachment. Se usa como
// adjunto en errores genéricos (no de fila) para que el operador reciba el archivo tal
// cual llegó — mucho más útil que un XLSX espejo con fila_excel=0 y un mensaje del sistema.
// Devuelve nil si no hay ruta, el archivo no existe en disco, está vacío o fue borrado
// (p.ej. tras un download parcial por caída de red).
func (n *sendGridNotifier) originalFromArchive(input FileEmailInput) *emailAttachment {
	path := strings.TrimSpace(input.ArchivePath)
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[notify] original_miss file_id=%s path=%q err=%v", input.FileID, path, err)
		return nil
	}
	if len(data) == 0 {
		log.Printf("[notify] original_miss file_id=%s path=%q motivo=archivo_vacío", input.FileID, path)
		return nil
	}
	filename := sanitizeAttachmentFilename(input.FileName)
	if filename == "" {
		filename = "archivo_original" + filepath.Ext(input.FileName)
	}
	return &emailAttachment{
		data:     data,
		filename: filename,
		mime:     mimeForFile(input.FileName),
	}
}

func mimeForFile(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".csv":
		return "text/csv"
	default:
		return "application/octet-stream"
	}
}

// mirrorFromReportArchive lee el XLSX espejo que el procesador ya escribió a disco
// (rec.ReportArchivePath) y lo devuelve como bytes. Se usa como fuente preferida del
// adjunto para garantizar que el correo lleve exactamente el mismo Excel que se archivó
// aun cuando el JSON en memoria haya llegado vacío o corrupto.
func (n *sendGridNotifier) mirrorFromReportArchive(input FileEmailInput) []byte {
	path := strings.TrimSpace(input.ReportArchivePath)
	if path == "" {
		log.Printf("[notify] mirror_archive_miss file_id=%s motivo=report_archive_path_vacío", input.FileID)
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[notify] mirror_archive_miss file_id=%s path=%q motivo=read_error err=%v",
			input.FileID, path, err)
		return nil
	}
	if len(data) == 0 {
		log.Printf("[notify] mirror_archive_miss file_id=%s path=%q motivo=archivo_vacío",
			input.FileID, path)
		return nil
	}
	log.Printf("[notify] mirror_archive_hit file_id=%s path=%q bytes=%d",
		input.FileID, path, len(data))
	return data
}

// summaryFallbackAttachment genera un adjunto XLSX mínimo con el metadato del archivo. Nunca
// devuelve nil salvo que el generador de XLSX del store falle catastróficamente. Se usa como
// última red de seguridad para respetar la invariante "todo correo lleva adjunto".
func (n *sendGridNotifier) summaryFallbackAttachment(input FileEmailInput) *emailAttachment {
	b, err := store.ValidationReportXLSXForErrorEmail(input.FileName, input.FileID, input.ProductID, input.ErrorReason)
	if err != nil || len(b) == 0 {
		log.Printf("[notify] no se pudo generar resumen mínimo file_id=%s err=%v", input.FileID, err)
		return nil
	}
	return &emailAttachment{
		data:     b,
		filename: validationReportAttachmentFilename(input.FileName, input.FileID, ".xlsx"),
		mime:     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	}
}

func (n *sendGridNotifier) notifyProcessedSuccess(input FileEmailInput) error {
	attachments, hasNovedades, err := n.processedSuccessAttachments(input)
	if err != nil {
		return err
	}
	reportURL := validationReportDownloadURL(input.FileID)
	client := sendgrid.NewSendClient(n.apiKey)

	send := func(atts []emailAttachment) (statusCode int, body string, err error) {
		msg := n.composeSuccessMail(input, atts, hasNovedades, reportURL)
		return sendWithBackoff(client, msg, input.FileID)
	}

	trySend := func(atts []emailAttachment) (ok bool, status int, body string, err error) {
		if len(atts) == 0 {
			return false, 0, "", nil
		}
		total := 0
		for _, a := range atts {
			total += len(a.data)
		}
		if total > maxEmailAttachmentBytes {
			log.Printf("[notify] adjuntos éxito demasiado grandes file_id=%s bytes=%d max=%d", input.FileID, total, maxEmailAttachmentBytes)
			return false, 0, "", nil
		}
		st, b, e := send(atts)
		if e != nil {
			return false, st, b, e
		}
		if st >= 200 && st < 300 {
			log.Printf("[notify] sendgrid aceptó correo éxito file_id=%s adjuntos=%d bytes=%d novedades=%t",
				input.FileID, len(atts), total, hasNovedades)
			return true, st, b, nil
		}
		return false, st, b, nil
	}

	for _, att := range attachments {
		ok, st, body, err := trySend([]emailAttachment{att})
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if st == 413 {
			label := "reporte-xlsx"
			if strings.HasSuffix(strings.ToLower(att.filename), ".zip") {
				label = "reporte-zip"
			}
			log.Printf("[notify] sendgrid 413 con %s (éxito); siguiente formato file_id=%s body=%s",
				label, input.FileID, truncateForLog(body, 400))
			continue
		}
		if st != 0 {
			log.Printf("[notify] sendgrid rechazó adjunto éxito file_id=%s status=%d body=%s",
				input.FileID, st, truncateForLog(body, 1800))
		}
	}

	// Invariante "todo correo con adjunto": antes de mandar sin nada, intentar con el resumen
	// mínimo (metadatos del archivo). Solo si eso también falla, mandar sin adjunto y devolver
	// error para que el operador vea el problema.
	if att := n.summaryFallbackAttachment(input); att != nil {
		log.Printf("[notify] reintentando correo de éxito con resumen mínimo file_id=%s", input.FileID)
		if ok, _, _, err := trySend([]emailAttachment{*att}); err != nil {
			return err
		} else if ok {
			return nil
		}
	}

	log.Printf("[notify] ADVERTENCIA: reintentando correo de éxito SIN adjunto file_id=%s", input.FileID)
	status2, body2, err2 := send(nil)
	if err2 != nil {
		return err2
	}
	if status2 >= 200 && status2 < 300 {
		return nil
	}
	return fmt.Errorf("sendgrid (éxito sin adjunto) status=%d body=%s", status2, strings.TrimSpace(truncateForLog(body2, 1800)))
}

func (n *sendGridNotifier) processedSuccessAttachments(input FileEmailInput) ([]emailAttachment, bool, error) {
	// El XLSX espejo archivado a disco por el procesador es la única fuente autorizada
	// del adjunto: garantiza que el operador reciba exactamente el mismo Excel que quedó
	// registrado. Sin dependencias de JSON en MySQL ni regeneración en tiempo de envío.
	hasNovedades := strings.TrimSpace(input.ValidationReportJSON) != ""
	b := n.mirrorFromReportArchive(input)
	if len(b) == 0 {
		return nil, hasNovedades, nil
	}
	return emailAttachmentCandidates(input.FileName, input.FileID, b), hasNovedades, nil
}

func (n *sendGridNotifier) notifyProcessingError(input FileEmailInput) error {
	client := sendgrid.NewSendClient(n.apiKey)

	// Prioridad del adjunto en errores:
	//   1) Espejo XLSX (rec.ReportArchivePath): existe solo cuando hubo novedades por fila
	//      — es el reporte útil con OBSERVACIÓN por cada fila. Se prefiere si cabe.
	//   2) Archivo original (rec.ArchivePath): la copia local del archivo tal cual llegó
	//      del SFTP. Se manda en errores genéricos (no de fila) para que el operador
	//      trabaje con el archivo real, no con un XLSX espejo con fila_excel=0.
	//   3) Sin adjunto: si no hay ninguno de los dos (p.ej. descarga parcial por caída
	//      de red), correo texto-solo con el motivo del error. Nunca stub engañoso.
	xlsxBytes := n.mirrorFromReportArchive(input)
	downloadURL := validationReportDownloadURL(input.FileID)
	reportTooLarge := len(xlsxBytes) > 0 && len(xlsxBytes) > maxEmailAttachmentBytes
	if reportTooLarge {
		log.Printf("[notify] espejo excede el límite de SendGrid file_id=%s bytes=%d max=%d — se envía correo con URL",
			input.FileID, len(xlsxBytes), maxEmailAttachmentBytes)
	}

	send := func(attachments []emailAttachment, kind errorAttachmentKind) (statusCode int, body string, err error) {
		msg := n.composeErrorMail(input, attachments, kind, downloadURL)
		return sendWithBackoff(client, msg, input.FileID)
	}

	trySend := func(attachments []emailAttachment, kind errorAttachmentKind, label string) (ok bool, status int, body string, err error) {
		total := 0
		for _, a := range attachments {
			total += len(a.data)
		}
		if total == 0 {
			return false, 0, "", nil
		}
		if total > maxEmailAttachmentBytes {
			log.Printf("[notify] adjuntos %s demasiado grandes file_id=%s bytes=%d max=%d",
				label, input.FileID, total, maxEmailAttachmentBytes)
			return false, 0, "", nil
		}
		st, b, e := send(attachments, kind)
		if e != nil {
			return false, st, b, e
		}
		if st >= 200 && st < 300 {
			log.Printf("[notify] sendgrid aceptó %s file_id=%s status=%d bytes=%d adjuntos=%d destinatarios=%d",
				label, input.FileID, st, total, len(attachments), len(n.recipients))
			return true, st, b, nil
		}
		return false, st, b, nil
	}

	// Caso 1: espejo XLSX útil y cabe → adjuntarlo (con fallback a ZIP si es grande).
	if len(xlsxBytes) > 0 && !reportTooLarge {
		for _, att := range emailAttachmentCandidates(input.FileName, input.FileID, xlsxBytes) {
			label := "reporte-xlsx"
			if strings.HasSuffix(strings.ToLower(att.filename), ".zip") {
				label = "reporte-zip"
			}
			ok, st, body, err := trySend([]emailAttachment{att}, errAttEspejo, label)
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
			if st == 413 {
				log.Printf("[notify] sendgrid 413 con %s; siguiente formato file_id=%s body=%s",
					label, input.FileID, truncateForLog(body, 400))
				continue
			}
			if st != 0 {
				log.Printf("[notify] sendgrid rechazó %s file_id=%s status=%d body=%s",
					label, input.FileID, st, truncateForLog(body, 1800))
			}
		}
		log.Printf("[notify] ADVERTENCIA: no se pudo adjuntar espejo file_id=%s bytes=%d — se enviará solo la URL de descarga",
			input.FileID, len(xlsxBytes))
		reportTooLarge = true
	}

	// Caso 2: espejo existe pero es demasiado grande para SendGrid → correo sin
	// adjunto con URL de descarga y mensaje claro. Nunca stub engañoso.
	if reportTooLarge {
		status, body, err := send(nil, errAttEspejoOversized)
		if err != nil {
			return err
		}
		if status >= 200 && status < 300 {
			log.Printf("[notify] correo enviado con URL de descarga (espejo %d bytes excede límite) file_id=%s",
				len(xlsxBytes), input.FileID)
			return nil
		}
		return fmt.Errorf("sendgrid (correo con URL) status=%d body=%s", status, strings.TrimSpace(truncateForLog(body, 1800)))
	}

	// Caso 3: no hay espejo útil → error genérico. Adjuntamos el archivo ORIGINAL
	// tal cual llegó del SFTP si aún lo tenemos en disco.
	if att := n.originalFromArchive(input); att != nil {
		ok, st, body, err := trySend([]emailAttachment{*att}, errAttOriginal, "original-sftp")
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if st != 0 {
			log.Printf("[notify] sendgrid rechazó original file_id=%s status=%d body=%s",
				input.FileID, st, truncateForLog(body, 1800))
		}
	} else {
		log.Printf("[notify] sin archivo original en disco file_id=%s archive_path=%q — se envía correo sin adjunto",
			input.FileID, input.ArchivePath)
	}

	// Caso 4: ni espejo ni original disponibles → correo texto solo con el motivo del error.
	// (Sucede típicamente con caídas de red durante la descarga: el original quedó parcial
	//  y se borró.)
	status2, body2, err2 := send(nil, errAttNone)
	if err2 != nil {
		return err2
	}
	if status2 >= 200 && status2 < 300 {
		log.Printf("[notify] correo de error enviado sin adjunto file_id=%s", input.FileID)
		return nil
	}
	return fmt.Errorf("sendgrid (sin adjunto) status=%d body=%s", status2, strings.TrimSpace(truncateForLog(body2, 1800)))
}

// errorAttachmentKind identifica qué adjunto (si hay) lleva el correo de error, y
// determina qué texto va en el cuerpo. Se prefiere una enum a booleanos sueltos porque
// los estados son mutuamente excluyentes.
type errorAttachmentKind int

const (
	errAttNone            errorAttachmentKind = iota // sin adjunto
	errAttEspejo                                     // XLSX espejo con novedades por fila
	errAttEspejoOversized                            // espejo existe pero excede el límite → URL en el body
	errAttOriginal                                   // archivo original tal cual llegó del SFTP
)

// emailAttachmentCandidates ordena .xlsx y .zip; ZIP primero si el libro supera emailZipPreferMinBytes.
func emailAttachmentCandidates(fileName, fileID string, xlsxBytes []byte) []emailAttachment {
	xlsxName := validationReportAttachmentFilename(fileName, fileID, ".xlsx")
	xlsxAtt := emailAttachment{
		data:     xlsxBytes,
		filename: xlsxName,
		mime:     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	}
	out := []emailAttachment{xlsxAtt}
	zipBytes, zipName, err := zipEmailAttachment(xlsxName, xlsxBytes)
	if err != nil || len(zipBytes) == 0 {
		return out
	}
	zipAtt := emailAttachment{data: zipBytes, filename: zipName, mime: "application/zip"}
	if len(xlsxBytes) >= emailZipPreferMinBytes {
		return []emailAttachment{zipAtt, xlsxAtt}
	}
	return []emailAttachment{xlsxAtt, zipAtt}
}

type emailAttachment struct {
	data     []byte
	filename string
	mime     string
}

func zipEmailAttachment(innerName string, data []byte) ([]byte, string, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create(innerName)
	if err != nil {
		return nil, "", err
	}
	if _, err := f.Write(data); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	zipName := strings.TrimSuffix(innerName, ".xlsx") + ".zip"
	if !strings.HasSuffix(zipName, ".zip") {
		zipName = innerName + ".zip"
	}
	return buf.Bytes(), zipName, nil
}

func validationReportDownloadURL(fileID string) string {
	return publicFileURL("/api/v1/files/validation-xlsx?file_id=", fileID)
}

func publicFileURL(pathPrefix, fileID string) string {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("API_PUBLIC_BASE_URL")), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(os.Getenv("BUSK_PUBLIC_API_URL")), "/")
	}
	if base == "" {
		return ""
	}
	return base + pathPrefix + url.QueryEscape(fileID)
}

func truncateForLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// sendBackoffs define la escalera de espera entre reintentos ante errores transitorios
// de SendGrid (fallo de red o status 5xx). Cuatro intentos totales: 1 original + 3
// reintentos separados por 1s, 4s y 15s. 4xx no se reintenta — un cambio de estrategia
// (menos adjunto, ZIP, etc.) es lo que puede salvar esos casos.
var sendBackoffs = []time.Duration{
	1 * time.Second,
	4 * time.Second,
	15 * time.Second,
}

// sendWithBackoff envuelve client.Send con reintentos ante fallos transitorios. Si
// SendGrid responde 2xx, retorna de inmediato. Si retorna 4xx, también corta porque
// el problema es del payload y el caller ya sabe cómo hacer fallback de estrategia.
// Solo error de red o 5xx entran al backoff. En el peor caso agrega ~20s de espera
// por Send antes de rendirse; con 2 workers, es un costo aceptable frente a perder
// la notificación por un blip de SendGrid.
func sendWithBackoff(client *sendgrid.Client, msg *sgmail.SGMailV3, fileID string) (int, string, error) {
	totalIntentos := len(sendBackoffs) + 1
	var lastStatus int
	var lastBody string
	var lastErr error
	for attempt := 0; attempt < totalIntentos; attempt++ {
		r, err := client.Send(msg)
		if err != nil {
			lastErr = err
			lastStatus = 0
			lastBody = ""
			log.Printf("[notify] sendgrid intento=%d/%d file_id=%s err=%v (red/transitorio)",
				attempt+1, totalIntentos, fileID, err)
		} else {
			lastStatus = r.StatusCode
			lastBody = r.Body
			lastErr = nil
			if r.StatusCode >= 200 && r.StatusCode < 300 {
				if attempt > 0 {
					log.Printf("[notify] sendgrid OK tras reintento file_id=%s intentos=%d status=%d",
						fileID, attempt+1, r.StatusCode)
				}
				return r.StatusCode, r.Body, nil
			}
			if r.StatusCode < 500 {
				// 4xx: no reintentar; delegar el fallback de estrategia al caller.
				return r.StatusCode, r.Body, nil
			}
			log.Printf("[notify] sendgrid intento=%d/%d file_id=%s status=%d body=%s (5xx transitorio)",
				attempt+1, totalIntentos, fileID, r.StatusCode, truncateForLog(r.Body, 400))
		}
		if attempt < len(sendBackoffs) {
			time.Sleep(sendBackoffs[attempt])
		}
	}
	if lastErr != nil {
		return 0, "", fmt.Errorf("sendgrid tras %d intentos: %w", totalIntentos, lastErr)
	}
	return lastStatus, lastBody, nil
}

func (n *sendGridNotifier) composeErrorMail(input FileEmailInput, attachments []emailAttachment, kind errorAttachmentKind, downloadURL string) *sgmail.SGMailV3 {
	from := sgmail.NewEmail("Busk Seguros", n.from)
	subject := fmt.Sprintf("[Busk Seguros] Novedades en archivo %s", strings.TrimSpace(input.FileName))
	switch {
	case strings.EqualFold(strings.TrimSpace(input.Status), "SKIPPED"):
		subject = fmt.Sprintf("[Busk Seguros] Archivo omitido: %s", strings.TrimSpace(input.FileName))
	case kind == errAttOriginal || kind == errAttNone:
		// Errores genéricos (no de fila): el asunto refleja que hay que revisar el sistema.
		subject = fmt.Sprintf("[Busk Seguros] Error procesando archivo %s", strings.TrimSpace(input.FileName))
	}

	message := sgmail.NewV3Mail()
	message.SetFrom(from)
	message.Subject = subject
	p := sgmail.NewPersonalization()
	for _, to := range n.recipients {
		p.AddTos(sgmail.NewEmail("", to))
	}
	message.AddPersonalizations(p)

	adjunto := false
	for _, attach := range attachments {
		if len(attach.data) == 0 {
			continue
		}
		att := sgmail.NewAttachment()
		att.SetContent(base64.StdEncoding.EncodeToString(attach.data))
		att.SetType(attach.mime)
		att.SetFilename(attach.filename)
		att.SetDisposition("attachment")
		message.AddAttachment(att)
		adjunto = true
	}

	adjuntoEspejo := adjunto && kind == errAttEspejo
	adjuntoOriginal := adjunto && kind == errAttOriginal
	reportTooLarge := kind == errAttEspejoOversized
	bodyMentionsReport := kind == errAttEspejo || kind == errAttEspejoOversized

	plainText := buildPlainBody(input, adjuntoEspejo, adjuntoOriginal, bodyMentionsReport, reportTooLarge, downloadURL)
	htmlBody := buildHTMLBody(input, adjuntoEspejo, adjuntoOriginal, bodyMentionsReport, reportTooLarge, downloadURL)
	message.AddContent(sgmail.NewContent("text/plain", plainText))
	message.AddContent(sgmail.NewContent("text/html", htmlBody))
	return message
}

func (n *sendGridNotifier) composeSuccessMail(input FileEmailInput, attachments []emailAttachment, hasNovedades bool, reportDownloadURL string) *sgmail.SGMailV3 {
	from := sgmail.NewEmail("Busk Seguros", n.from)
	subject := fmt.Sprintf("[Busk Seguros] Archivo procesado correctamente: %s", strings.TrimSpace(input.FileName))
	if hasNovedades {
		subject = fmt.Sprintf("[Busk Seguros] Archivo procesado con novedades: %s", strings.TrimSpace(input.FileName))
	}

	message := sgmail.NewV3Mail()
	message.SetFrom(from)
	message.Subject = subject
	p := sgmail.NewPersonalization()
	for _, to := range n.recipients {
		p.AddTos(sgmail.NewEmail("", to))
	}
	message.AddPersonalizations(p)

	for _, attach := range attachments {
		if len(attach.data) == 0 {
			continue
		}
		att := sgmail.NewAttachment()
		att.SetContent(base64.StdEncoding.EncodeToString(attach.data))
		att.SetType(attach.mime)
		att.SetFilename(attach.filename)
		att.SetDisposition("attachment")
		message.AddAttachment(att)
	}

	plainText := buildSuccessPlainBody(input, len(attachments) > 0, hasNovedades, reportDownloadURL)
	htmlBody := buildSuccessHTMLBody(input, len(attachments) > 0, hasNovedades, reportDownloadURL)
	message.AddContent(sgmail.NewContent("text/plain", plainText))
	message.AddContent(sgmail.NewContent("text/html", htmlBody))
	return message
}

type reportEmailData struct {
	FileName            string
	ProductID           string
	Estado              string
	ProcesadoEn         string
	ResumenOperacion    string
	MensajePrincipal    string
	FilasRevisadas      int
	FilasConIncidencias int
	CreditosDuplicados  int
	FilasDuplicadas     int
	TieneInforme        bool
	AdjuntoExcel        bool
	AdjuntoOriginal     bool
	ReporteMuyGrande    bool
	DescargaInformeURL  string
	DetalleCarga        string
}

func buildReportEmailData(input FileEmailInput, adjuntoExcel, adjuntoOriginal, reporteMuyGrande bool, downloadURL string) reportEmailData {
	data := reportEmailData{
		FileName:           strings.TrimSpace(input.FileName),
		ProductID:          emptyFallback(input.ProductID, "No identificado"),
		Estado:             etiquetaEstadoArchivo(input.Status),
		ProcesadoEn:        "No disponible",
		ResumenOperacion:   "El archivo no pudo completar el procesamiento automático.",
		MensajePrincipal:   "Se registró un error durante el procesamiento. Revise el detalle indicado más abajo.",
		AdjuntoExcel:       adjuntoExcel,
		AdjuntoOriginal:    adjuntoOriginal,
		ReporteMuyGrande:   reporteMuyGrande,
		DescargaInformeURL: strings.TrimSpace(downloadURL),
		DetalleCarga:       strings.TrimSpace(input.ErrorReason),
	}
	if data.DetalleCarga == "" {
		data.DetalleCarga = "Sin detalle adicional en el sistema."
	}
	if !input.ProcessedAt.IsZero() {
		data.ProcesadoEn = input.ProcessedAt.UTC().Format("02/01/2006 15:04") + " (UTC)"
	}

	report, ok := parseValidationReport(input.ValidationReportJSON)
	if !ok {
		if adjuntoOriginal {
			// Error genérico: no hay reporte por fila, se adjunta el archivo original.
			data.MensajePrincipal = "El archivo no pudo procesarse por un error del sistema. Se adjunta el archivo original tal cual llegó del SFTP para su revisión."
			data.ResumenOperacion = "Error de sistema — requiere revisión técnica."
		}
		return data
	}

	data.TieneInforme = true
	data.FilasRevisadas = report.PolicyRowCount
	data.FilasConIncidencias = report.TotalPendingValidations
	data.CreditosDuplicados = report.TotalDuplicateCredits
	data.FilasDuplicadas = report.TotalDuplicateRows

	if data.FilasConIncidencias > 0 || data.CreditosDuplicados > 0 {
		data.MensajePrincipal = fmt.Sprintf(
			"Se han generado novedades de validación en %d fila(s) del archivo. La carga de pólizas no se completó hasta corregir o revisar estos hallazgos.",
			data.FilasConIncidencias,
		)
		data.ResumenOperacion = "Validación con incidencias — requiere revisión operativa."
	} else if strings.TrimSpace(input.ErrorReason) != "" {
		data.MensajePrincipal = "El procesamiento finalizó con observaciones. Consulte el detalle y el informe adjunto si está disponible."
		data.ResumenOperacion = "Procesamiento con observaciones."
	}

	return data
}

func etiquetaEstadoArchivo(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ERROR":
		return "Error en procesamiento"
	case "PROCESSED":
		return "Procesado"
	case "PENDING":
		return "Pendiente"
	case "PROCESSING":
		return "En proceso"
	case "SKIPPED":
		return "Omitido"
	default:
		if strings.TrimSpace(status) == "" {
			return "Desconocido"
		}
		return status
	}
}

func buildPlainBody(input FileEmailInput, adjuntoExcel, adjuntoOriginal, tieneInforme, reporteMuyGrande bool, downloadURL string) string {
	d := buildReportEmailData(input, adjuntoExcel, adjuntoOriginal, reporteMuyGrande, downloadURL)
	var b strings.Builder
	b.WriteString("Busk Seguros — Informe de procesamiento de archivo\n")
	b.WriteString(strings.Repeat("=", 48) + "\n\n")
	b.WriteString(d.MensajePrincipal + "\n\n")
	b.WriteString("Resumen: " + d.ResumenOperacion + "\n\n")
	b.WriteString("Archivo: " + d.FileName + "\n")
	b.WriteString("Producto: " + d.ProductID + "\n")
	b.WriteString("Estado: " + d.Estado + "\n")
	b.WriteString("Procesado: " + d.ProcesadoEn + "\n\n")
	if d.TieneInforme {
		b.WriteString("Indicadores:\n")
		b.WriteString(fmt.Sprintf("  · Filas analizadas en el informe: %d\n", d.FilasRevisadas))
		b.WriteString(fmt.Sprintf("  · Filas con incidencias: %d\n", d.FilasConIncidencias))
		if d.CreditosDuplicados > 0 {
			b.WriteString(fmt.Sprintf("  · Créditos u operaciones duplicados en el archivo: %d\n", d.CreditosDuplicados))
			b.WriteString(fmt.Sprintf("  · Filas afectadas por duplicidad: %d\n", d.FilasDuplicadas))
		}
		b.WriteString("\n")
	}
	b.WriteString("Detalle del sistema:\n" + d.DetalleCarga + "\n\n")
	switch {
	case adjuntoExcel:
		b.WriteString("Adjunto: Excel espejo del archivo original con las columnas OBSERVACIÓN y novedades por fila.\n\n")
	case adjuntoOriginal:
		b.WriteString("Adjunto: archivo original tal cual llegó del SFTP, para su revisión manual.\n\n")
	case reporteMuyGrande:
		b.WriteString("Aviso importante: el reporte de novedades supera el tamaño máximo permitido por el proveedor de correo (~28 MB), por eso este mensaje no lleva adjunto.\n")
		b.WriteString("El reporte completo (espejo del archivo original con OBSERVACIÓN y novedades por fila) está disponible para descarga:\n")
		if d.DescargaInformeURL != "" {
			b.WriteString(d.DescargaInformeURL + "\n\n")
		} else {
			b.WriteString("Solicítelo al equipo técnico con file_id=" + strings.TrimSpace(input.FileID) + " (no está configurada la URL pública API_PUBLIC_BASE_URL).\n\n")
		}
	case tieneInforme && d.DescargaInformeURL != "":
		b.WriteString("El adjunto no pudo enviarse. Descargue el informe Excel aquí:\n")
		b.WriteString(d.DescargaInformeURL + "\n\n")
	case tieneInforme:
		b.WriteString("El adjunto no pudo enviarse. Solicite el Excel al equipo técnico con file_id=" + strings.TrimSpace(input.FileID) + ".\n\n")
	default:
		b.WriteString("Este mensaje no lleva adjunto. Revise el detalle del sistema arriba para el motivo del error.\n\n")
	}
	b.WriteString("— Busk Seguros · Procesamiento automático de inclusiones\n")
	return strings.TrimSpace(b.String())
}

func buildHTMLBody(input FileEmailInput, adjuntoExcel, adjuntoOriginal, tieneInforme, reporteMuyGrande bool, downloadURL string) string {
	data := buildReportEmailData(input, adjuntoExcel, adjuntoOriginal, reporteMuyGrande, downloadURL)
	tpl, err := template.New("error-email").Parse(errorEmailTemplate)
	if err != nil {
		return strings.ReplaceAll(buildPlainBody(input, adjuntoExcel, adjuntoOriginal, tieneInforme, reporteMuyGrande, downloadURL), "\n", "<br>")
	}
	var b bytes.Buffer
	if err := tpl.Execute(&b, data); err != nil {
		return strings.ReplaceAll(buildPlainBody(input, adjuntoExcel, adjuntoOriginal, tieneInforme, reporteMuyGrande, downloadURL), "\n", "<br>")
	}
	return b.String()
}

func parseValidationReport(raw string) (store.FileValidationReport, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return store.FileValidationReport{}, false
	}
	var report store.FileValidationReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		return store.FileValidationReport{}, false
	}
	return report, true
}

func validationReportAttachmentFilename(fileName, fileID, ext string) string {
	if ext == "" {
		ext = ".xlsx"
	}
	if ext[0] != '.' {
		ext = "." + ext
	}
	base := sanitizeAttachmentFilename(fileName)
	if base == "" {
		base = "archivo"
	}
	if len(base) > 80 {
		base = base[:80]
	}
	id := strings.TrimSpace(fileID)
	if len(id) > 12 {
		id = id[:12]
	}
	if id == "" {
		return "novedades_" + base + ext
	}
	return "novedades_" + base + "_" + id + ext
}

func sanitizeAttachmentFilename(fileName string) string {
	base := strings.TrimSpace(fileName)
	if base == "" {
		return ""
	}
	base = filepath.Base(base)
	var b strings.Builder
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' || r == ' ' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return strings.TrimSpace(b.String())
}

func buildSuccessPlainBody(input FileEmailInput, adjuntos bool, hasNovedades bool, reportDownloadURL string) string {
	fileName := strings.TrimSpace(input.FileName)
	productID := emptyFallback(input.ProductID, "No identificado")
	processedAt := "No disponible"
	if !input.ProcessedAt.IsZero() {
		processedAt = input.ProcessedAt.UTC().Format("02/01/2006 15:04") + " (UTC)"
	}
	var b strings.Builder
	b.WriteString("Busk Seguros — Archivo procesado\n")
	b.WriteString(strings.Repeat("=", 48) + "\n\n")
	if hasNovedades {
		b.WriteString("El archivo se cargó correctamente, pero hay filas con novedades informativas que debe revisar.\n\n")
	} else {
		b.WriteString("El archivo se cargó en el sistema sin novedades.\n\n")
	}
	b.WriteString("Archivo: " + fileName + "\n")
	b.WriteString("Producto: " + productID + "\n")
	b.WriteString("Estado: Procesado\n")
	b.WriteString("Procesado: " + processedAt + "\n\n")
	if adjuntos {
		b.WriteString("Adjunto: Excel espejo del archivo original con las columnas OBSERVACIÓN y novedades.\n\n")
	} else if strings.TrimSpace(reportDownloadURL) != "" {
		b.WriteString("Descargue el Excel espejo con OBSERVACIÓN y novedades:\n" + reportDownloadURL + "\n\n")
	}
	b.WriteString("— Busk Seguros · Procesamiento automático de inclusiones\n")
	return strings.TrimSpace(b.String())
}

func buildSuccessHTMLBody(input FileEmailInput, adjuntos bool, hasNovedades bool, reportDownloadURL string) string {
	tpl, err := template.New("success-email").Parse(successEmailTemplate)
	if err != nil {
		return strings.ReplaceAll(buildSuccessPlainBody(input, adjuntos, hasNovedades, reportDownloadURL), "\n", "<br>")
	}
	data := struct {
		FileName           string
		ProductID          string
		ProcesadoEn        string
		HasNovedades       bool
		Adjuntos           bool
		DescargaInformeURL string
	}{
		FileName:           strings.TrimSpace(input.FileName),
		ProductID:          emptyFallback(input.ProductID, "No identificado"),
		HasNovedades:       hasNovedades,
		Adjuntos:           adjuntos,
		DescargaInformeURL: strings.TrimSpace(reportDownloadURL),
	}
	if !input.ProcessedAt.IsZero() {
		data.ProcesadoEn = input.ProcessedAt.UTC().Format("02/01/2006 15:04") + " (UTC)"
	} else {
		data.ProcesadoEn = "No disponible"
	}
	var b bytes.Buffer
	if err := tpl.Execute(&b, data); err != nil {
		return strings.ReplaceAll(buildSuccessPlainBody(input, adjuntos, hasNovedades, reportDownloadURL), "\n", "<br>")
	}
	return b.String()
}

func splitRecipients(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func emptyFallback(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

const errorEmailTemplate = `
<!doctype html>
<html lang="es">
<head><meta charset="UTF-8"></head>
<body style="margin:0;padding:0;background:#f4f6f8;font-family:Arial,Helvetica,sans-serif;color:#1a1a1a;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f6f8;padding:24px 12px;">
    <tr><td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;background:#ffffff;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,0.06);">
        <tr>
          <td style="background:#0b3d91;padding:20px 28px;">
            <p style="margin:0;font-size:13px;color:#a8c4f0;letter-spacing:0.5px;text-transform:uppercase;">Busk Seguros</p>
            <h1 style="margin:6px 0 0;font-size:20px;font-weight:600;color:#ffffff;line-height:1.3;">Informe de procesamiento de archivo</h1>
          </td>
        </tr>
        <tr>
          <td style="padding:28px;">
            <p style="margin:0 0 8px;font-size:12px;color:#0b3d91;font-weight:600;text-transform:uppercase;">{{.ResumenOperacion}}</p>
            <p style="margin:0 0 20px;font-size:15px;line-height:1.5;color:#333;">{{.MensajePrincipal}}</p>

            <div style="background:#fff8e6;border-left:4px solid #e6a800;padding:14px 16px;margin-bottom:24px;border-radius:0 4px 4px 0;">
              <p style="margin:0;font-size:14px;line-height:1.5;color:#5c4a00;">
                {{if .AdjuntoExcel}}
                <strong>Acción requerida:</strong> revise el Excel adjunto (espejo del archivo original con columnas OBSERVACIÓN y novedades).
                {{else if .AdjuntoOriginal}}
                <strong>Acción requerida:</strong> revise el archivo original adjunto (tal cual llegó del SFTP) y el detalle del sistema para identificar el motivo.
                {{else if .ReporteMuyGrande}}
                <strong>Acción requerida:</strong> descargue el reporte completo desde el enlace indicado más abajo. El adjunto no se incluye porque supera el tamaño máximo del proveedor de correo (~28 MB).
                {{else if .DescargaInformeURL}}
                <strong>Acción requerida:</strong> descargue el informe Excel desde el enlace indicado más abajo (el adjunto no pudo enviarse por tamaño).
                {{else}}
                <strong>Acción requerida:</strong> revise el detalle del sistema para identificar el motivo del error. Este mensaje no lleva adjunto.
                {{end}}
              </p>
            </div>

            <p style="margin:0 0 10px;font-size:13px;font-weight:600;color:#555;text-transform:uppercase;letter-spacing:0.3px;">Datos del archivo</p>
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="font-size:14px;margin-bottom:24px;">
              <tr><td style="padding:6px 0;color:#666;width:140px;vertical-align:top;">Archivo</td><td style="padding:6px 0;color:#1a1a1a;"><strong>{{.FileName}}</strong></td></tr>
              <tr><td style="padding:6px 0;color:#666;">Producto</td><td style="padding:6px 0;color:#1a1a1a;">{{.ProductID}}</td></tr>
              <tr><td style="padding:6px 0;color:#666;">Estado</td><td style="padding:6px 0;color:#1a1a1a;">{{.Estado}}</td></tr>
              <tr><td style="padding:6px 0;color:#666;">Fecha de proceso</td><td style="padding:6px 0;color:#1a1a1a;">{{.ProcesadoEn}}</td></tr>
            </table>

            {{if .TieneInforme}}
            <p style="margin:0 0 10px;font-size:13px;font-weight:600;color:#555;text-transform:uppercase;letter-spacing:0.3px;">Resumen de novedades</p>
            <ul style="margin:0 0 24px;padding-left:20px;font-size:14px;line-height:1.7;color:#333;">
              <li><strong>{{.FilasConIncidencias}}</strong> fila(s) con incidencias de validación</li>
              <li><strong>{{.FilasRevisadas}}</strong> fila(s) analizadas en el informe</li>
              {{if gt .CreditosDuplicados 0}}
              <li><strong>{{.CreditosDuplicados}}</strong> crédito(s) u operación(es) repetidos en el archivo ({{.FilasDuplicadas}} fila(s) afectadas)</li>
              {{end}}
            </ul>
            {{end}}

            <p style="margin:0 0 8px;font-size:13px;font-weight:600;color:#555;text-transform:uppercase;letter-spacing:0.3px;">Detalle del sistema</p>
            <p style="margin:0 0 24px;font-size:13px;line-height:1.5;color:#555;background:#f8f9fa;padding:12px 14px;border-radius:4px;">{{.DetalleCarga}}</p>

            {{if .AdjuntoExcel}}
            <div style="background:#eef4fc;padding:16px;border-radius:6px;margin-bottom:8px;">
              <p style="margin:0;font-size:14px;line-height:1.5;color:#0b3d91;">
                <strong>Informe adjunto (Excel):</strong> espejo del archivo original con las columnas OBSERVACIÓN y novedades por fila.
              </p>
            </div>
            {{else if .AdjuntoOriginal}}
            <div style="background:#eef4fc;padding:16px;border-radius:6px;margin-bottom:8px;">
              <p style="margin:0;font-size:14px;line-height:1.5;color:#0b3d91;">
                <strong>Archivo adjunto:</strong> archivo original tal cual llegó del SFTP, para su revisión manual.
              </p>
            </div>
            {{else if .ReporteMuyGrande}}
            <div style="background:#fff8e6;padding:16px;border-radius:6px;margin-bottom:8px;border-left:4px solid #e6a800;">
              <p style="margin:0 0 8px;font-size:14px;line-height:1.5;color:#5c4a00;">
                <strong>Reporte demasiado grande para adjuntar.</strong> El espejo del archivo original excede el tamaño máximo permitido por el proveedor de correo (~28 MB), por eso este mensaje no lleva adjunto.
              </p>
              {{if .DescargaInformeURL}}
              <p style="margin:0;font-size:14px;line-height:1.5;color:#5c4a00;">
                <strong>Descargar reporte completo:</strong>
                <a href="{{.DescargaInformeURL}}" style="color:#5c4a00;">{{.DescargaInformeURL}}</a>
              </p>
              {{else}}
              <p style="margin:0;font-size:14px;line-height:1.5;color:#5c4a00;">
                Solicite el Excel al equipo técnico (no está configurada la URL pública <code>API_PUBLIC_BASE_URL</code>).
              </p>
              {{end}}
            </div>
            {{else if .DescargaInformeURL}}
            <div style="background:#eef4fc;padding:16px;border-radius:6px;margin-bottom:8px;">
              <p style="margin:0;font-size:14px;line-height:1.5;color:#0b3d91;">
                <strong>Descargar informe Excel:</strong>
                <a href="{{.DescargaInformeURL}}" style="color:#0b3d91;">{{.DescargaInformeURL}}</a>
              </p>
            </div>
            {{end}}
          </td>
        </tr>
        <tr>
          <td style="background:#f0f2f5;padding:16px 28px;border-top:1px solid #e5e7eb;">
            <p style="margin:0;font-size:12px;color:#888;line-height:1.4;">
              Mensaje automático del sistema de procesamiento Busk Seguros. No responda a este correo.
            </p>
          </td>
        </tr>
      </table>
    </td></tr>
  </table>
</body>
</html>
`

const successEmailTemplate = `
<!doctype html>
<html lang="es">
<head><meta charset="UTF-8"></head>
<body style="margin:0;padding:0;background:#f4f6f8;font-family:Arial,Helvetica,sans-serif;color:#1a1a1a;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f6f8;padding:24px 12px;">
    <tr><td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;background:#ffffff;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,0.06);">
        <tr>
          <td style="background:#0d6b3a;padding:20px 28px;">
            <p style="margin:0;font-size:13px;color:#b8e6cc;letter-spacing:0.5px;text-transform:uppercase;">Busk Seguros</p>
            <h1 style="margin:6px 0 0;font-size:20px;font-weight:600;color:#ffffff;line-height:1.3;">Archivo procesado correctamente</h1>
          </td>
        </tr>
        <tr>
          <td style="padding:28px;">
            {{if .HasNovedades}}
            <p style="margin:0 0 20px;font-size:15px;line-height:1.5;color:#333;">El archivo se cargó correctamente, pero hay filas con novedades informativas que debe revisar.</p>
            {{else}}
            <p style="margin:0 0 20px;font-size:15px;line-height:1.5;color:#333;">El archivo se cargó en el sistema sin novedades.</p>
            {{end}}
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="font-size:14px;margin-bottom:24px;">
              <tr><td style="padding:6px 0;color:#666;width:140px;vertical-align:top;">Archivo</td><td style="padding:6px 0;color:#1a1a1a;"><strong>{{.FileName}}</strong></td></tr>
              <tr><td style="padding:6px 0;color:#666;">Producto</td><td style="padding:6px 0;color:#1a1a1a;">{{.ProductID}}</td></tr>
              <tr><td style="padding:6px 0;color:#666;">Estado</td><td style="padding:6px 0;color:#1a1a1a;">Procesado</td></tr>
              <tr><td style="padding:6px 0;color:#666;">Fecha de proceso</td><td style="padding:6px 0;color:#1a1a1a;">{{.ProcesadoEn}}</td></tr>
            </table>
            {{if .Adjuntos}}
            <div style="background:#eef9f1;padding:16px;border-radius:6px;margin-bottom:8px;">
              <p style="margin:0;font-size:14px;line-height:1.5;color:#0d6b3a;">
                <strong>Adjunto:</strong> Excel espejo del archivo original con las columnas OBSERVACIÓN y novedades.
              </p>
            </div>
            {{else if .DescargaInformeURL}}
            <div style="background:#eef9f1;padding:16px;border-radius:6px;margin-bottom:8px;">
              <p style="margin:0;font-size:14px;line-height:1.5;color:#0d6b3a;">
                <strong>Descargar Excel con OBSERVACIÓN y novedades:</strong>
                <a href="{{.DescargaInformeURL}}" style="color:#0d6b3a;">{{.DescargaInformeURL}}</a>
              </p>
            </div>
            {{end}}
          </td>
        </tr>
        <tr>
          <td style="background:#f0f2f5;padding:16px 28px;border-top:1px solid #e5e7eb;">
            <p style="margin:0;font-size:12px;color:#888;line-height:1.4;">
              Mensaje automático del sistema de procesamiento Busk Seguros. No responda a este correo.
            </p>
          </td>
        </tr>
      </table>
    </td></tr>
  </table>
</body>
</html>
`
