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
	switch strings.ToUpper(strings.TrimSpace(input.Status)) {
	case "PROCESSED":
		return n.notifyProcessedSuccess(input)
	case "ERROR":
		return n.notifyProcessingError(input)
	default:
		return nil
	}
}

func (n *sendGridNotifier) notifyProcessedSuccess(input FileEmailInput) error {
	attachments, hasNovedades, err := n.processedSuccessAttachments(input)
	if err != nil {
		return err
	}
	archiveURL := archiveDownloadURL(input.FileID)
	reportURL := validationReportDownloadURL(input.FileID)
	client := sendgrid.NewSendClient(n.apiKey)

	send := func(atts []emailAttachment) (statusCode int, body string, err error) {
		msg := n.composeSuccessMail(input, atts, hasNovedades, archiveURL, reportURL)
		r, e := client.Send(msg)
		if e != nil {
			return 0, "", e
		}
		return r.StatusCode, r.Body, nil
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

	if ok, st, _, err := trySend(attachments); err != nil {
		return err
	} else if ok {
		return nil
	} else if st == 413 && len(attachments) > 1 {
		log.Printf("[notify] 413 con todos los adjuntos; reintento solo informe file_id=%s", input.FileID)
		if ok, _, _, err := trySend(attachments[:1]); err != nil {
			return err
		} else if ok {
			return nil
		}
	}

	log.Printf("[notify] reintentando correo de éxito sin adjunto file_id=%s", input.FileID)
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
	var out []emailAttachment
	hasNovedades := false

	if report, ok := parseValidationReport(input.ValidationReportJSON); ok && validationReportHasNovedades(report) {
		hasNovedades = true
		b, err := store.ValidationReportEmailXLSX(report)
		if err != nil {
			log.Printf("[notify] no se pudo generar Excel de novedades (éxito) file_id=%s err=%v", input.FileID, err)
		} else if len(b) > 0 {
			out = append(out, emailAttachment{
				data:     b,
				filename: validationReportAttachmentFilename(input.FileName, input.FileID, ".xlsx"),
				mime:     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			})
		}
	}

	archivePath := strings.TrimSpace(input.ArchivePath)
	if archivePath == "" {
		log.Printf("[notify] PROCESSED sin archive_path file_id=%s — correo saldrá sin el archivo original", input.FileID)
		return out, hasNovedades, nil
	}
	originalBytes, err := os.ReadFile(archivePath)
	if err != nil {
		// El archivo local puede haberse perdido (cwd cambió, se movió, cleanup). No bloquear
		// el correo por eso: enviarlo sin adjunto original y dejar constancia en el log.
		log.Printf("[notify] no se pudo leer archivo archivado file_id=%s path=%q err=%v — correo saldrá sin el archivo original",
			input.FileID, archivePath, err)
		return out, hasNovedades, nil
	}
	if len(originalBytes) > 0 {
		cands := originalFileAttachmentCandidates(input.FileName, originalBytes)
		if len(cands) > 0 {
			out = append(out, cands[0])
		}
	}
	return out, hasNovedades, nil
}

func validationReportHasNovedades(r store.FileValidationReport) bool {
	return r.TotalInformativeValidations > 0 || r.TotalPendingValidations > 0
}

func (n *sendGridNotifier) notifyProcessingError(input FileEmailInput) error {
	client := sendgrid.NewSendClient(n.apiKey)

	var xlsxBytes []byte
	if report, ok := parseValidationReport(input.ValidationReportJSON); ok {
		b, err := store.ValidationReportEmailXLSX(report)
		if err != nil {
			log.Printf("[notify] no se pudo generar Excel de novedades (correo) file_id=%s err=%v", input.FileID, err)
		} else if len(b) > 0 {
			xlsxBytes = b
		}
	}
	if len(xlsxBytes) == 0 {
		b, err := store.ValidationReportXLSXForErrorEmail(input.FileName, input.FileID, input.ProductID, input.ErrorReason)
		if err != nil {
			log.Printf("[notify] no se pudo generar Excel resumen file_id=%s err=%v", input.FileID, err)
		} else {
			xlsxBytes = b
		}
	}
	downloadURL := validationReportDownloadURL(input.FileID)

	send := func(attach *emailAttachment, bodyMentionsReport bool) (statusCode int, body string, err error) {
		msg := n.composeErrorMail(input, attach, bodyMentionsReport, downloadURL)
		r, e := client.Send(msg)
		if e != nil {
			return 0, "", e
		}
		return r.StatusCode, r.Body, nil
	}

	tryAttach := func(att emailAttachment, label string) (ok bool, status int, body string, err error) {
		if len(att.data) == 0 {
			return false, 0, "", nil
		}
		if len(att.data) > maxEmailAttachmentBytes {
			log.Printf("[notify] adjunto %s demasiado grande file_id=%s bytes=%d max=%d", label, input.FileID, len(att.data), maxEmailAttachmentBytes)
			return false, 0, "", nil
		}
		st, b, e := send(&att, true)
		if e != nil {
			return false, st, b, e
		}
		if st >= 200 && st < 300 {
			log.Printf("[notify] sendgrid aceptó adjunto %s file_id=%s status=%d bytes=%d destinatarios=%d",
				label, input.FileID, st, len(att.data), len(n.recipients))
			return true, st, b, nil
		}
		return false, st, b, nil
	}

	if len(xlsxBytes) > 0 {
		for _, att := range emailAttachmentCandidates(input.FileName, input.FileID, xlsxBytes) {
			label := "xlsx"
			if strings.HasSuffix(strings.ToLower(att.filename), ".zip") {
				label = "zip"
			}
			ok, st, body, err := tryAttach(att, label)
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
	}

	if len(xlsxBytes) > 0 {
		log.Printf("[notify] ADVERTENCIA: no se pudo adjuntar Excel file_id=%s bytes=%d; reintento correo sin adjunto", input.FileID, len(xlsxBytes))
	}

	log.Printf("[notify] reintentando sin adjunto file_id=%s", input.FileID)
	status2, body2, err2 := send(nil, len(xlsxBytes) > 0)
	if err2 != nil {
		return err2
	}
	if status2 >= 200 && status2 < 300 {
		if len(xlsxBytes) > 0 {
			if downloadURL != "" {
				log.Printf("[notify] correo enviado SIN adjunto (descargue: %s) file_id=%s", downloadURL, input.FileID)
			} else {
				log.Printf("[notify] correo enviado SIN adjunto (configure API_PUBLIC_BASE_URL) file_id=%s", input.FileID)
			}
		} else {
			log.Printf("[notify] correo enviado sin adjunto file_id=%s", input.FileID)
		}
		return nil
	}
	return fmt.Errorf("sendgrid (reintento sin adjunto) status=%d body=%s", status2, strings.TrimSpace(truncateForLog(body2, 1800)))
}

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

func archiveDownloadURL(fileID string) string {
	return publicFileURL("/api/v1/files/download?file_id=", fileID)
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

func (n *sendGridNotifier) composeErrorMail(input FileEmailInput, attach *emailAttachment, bodyMentionsReport bool, downloadURL string) *sgmail.SGMailV3 {
	from := sgmail.NewEmail("Busk Seguros", n.from)
	subject := fmt.Sprintf("[Busk Seguros] Novedades en archivo %s", strings.TrimSpace(input.FileName))

	message := sgmail.NewV3Mail()
	message.SetFrom(from)
	message.Subject = subject
	p := sgmail.NewPersonalization()
	for _, to := range n.recipients {
		p.AddTos(sgmail.NewEmail("", to))
	}
	message.AddPersonalizations(p)

	if attach != nil && len(attach.data) > 0 {
		att := sgmail.NewAttachment()
		att.SetContent(base64.StdEncoding.EncodeToString(attach.data))
		att.SetType(attach.mime)
		att.SetFilename(attach.filename)
		att.SetDisposition("attachment")
		message.AddAttachment(att)
	}

	adjunto := attach != nil && len(attach.data) > 0
	plainText := buildPlainBody(input, adjunto, bodyMentionsReport, downloadURL)
	htmlBody := buildHTMLBody(input, adjunto, bodyMentionsReport, downloadURL)
	message.AddContent(sgmail.NewContent("text/plain", plainText))
	message.AddContent(sgmail.NewContent("text/html", htmlBody))
	return message
}

func (n *sendGridNotifier) composeSuccessMail(input FileEmailInput, attachments []emailAttachment, hasNovedades bool, archiveDownloadURL, reportDownloadURL string) *sgmail.SGMailV3 {
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

	plainText := buildSuccessPlainBody(input, len(attachments) > 0, hasNovedades, archiveDownloadURL, reportDownloadURL)
	htmlBody := buildSuccessHTMLBody(input, len(attachments) > 0, hasNovedades, archiveDownloadURL, reportDownloadURL)
	message.AddContent(sgmail.NewContent("text/plain", plainText))
	message.AddContent(sgmail.NewContent("text/html", htmlBody))
	return message
}

type reportEmailData struct {
	FileName                string
	ProductID               string
	Estado                  string
	ProcesadoEn             string
	ResumenOperacion        string
	MensajePrincipal        string
	FilasRevisadas          int
	FilasConIncidencias     int
	CreditosDuplicados      int
	FilasDuplicadas         int
	TieneInforme            bool
	AdjuntoExcel            bool
	DescargaInformeURL    string
	DetalleCarga            string
}

func buildReportEmailData(input FileEmailInput, adjuntoExcel bool, downloadURL string) reportEmailData {
	data := reportEmailData{
		FileName:           strings.TrimSpace(input.FileName),
		ProductID:          emptyFallback(input.ProductID, "No identificado"),
		Estado:             etiquetaEstadoArchivo(input.Status),
		ProcesadoEn:        "No disponible",
		ResumenOperacion:   "El archivo no pudo completar el procesamiento automático.",
		MensajePrincipal:   "Se registró un error durante el procesamiento. Revise el detalle indicado más abajo.",
		AdjuntoExcel:       adjuntoExcel,
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

func buildPlainBody(input FileEmailInput, adjuntoExcel, tieneInforme bool, downloadURL string) string {
	d := buildReportEmailData(input, adjuntoExcel, downloadURL)
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
	if adjuntoExcel {
		b.WriteString("Adjunto: Excel (.xlsx o .zip) con hojas Incidencias e Informes.\n\n")
	} else if tieneInforme && d.DescargaInformeURL != "" {
		b.WriteString("El adjunto no pudo enviarse por tamaño. Descargue el informe Excel aquí:\n")
		b.WriteString(d.DescargaInformeURL + "\n\n")
	} else if tieneInforme {
		b.WriteString("El adjunto no pudo enviarse. Solicite el Excel al equipo técnico con file_id=" + strings.TrimSpace(input.FileID) + ".\n\n")
	}
	b.WriteString("— Busk Seguros · Procesamiento automático de inclusiones\n")
	return strings.TrimSpace(b.String())
}

func buildHTMLBody(input FileEmailInput, adjuntoExcel, tieneInforme bool, downloadURL string) string {
	data := buildReportEmailData(input, adjuntoExcel, downloadURL)
	tpl, err := template.New("error-email").Parse(errorEmailTemplate)
	if err != nil {
		return strings.ReplaceAll(buildPlainBody(input, adjuntoExcel, tieneInforme, downloadURL), "\n", "<br>")
	}
	var b bytes.Buffer
	if err := tpl.Execute(&b, data); err != nil {
		return strings.ReplaceAll(buildPlainBody(input, adjuntoExcel, tieneInforme, downloadURL), "\n", "<br>")
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

func originalFileAttachmentCandidates(fileName string, data []byte) []emailAttachment {
	safeName := sanitizeAttachmentFilename(fileName)
	if safeName == "" {
		safeName = "archivo.xlsx"
	}
	fileAtt := emailAttachment{
		data:     data,
		filename: safeName,
		mime:     mimeForFileName(safeName),
	}
	out := []emailAttachment{fileAtt}
	zipBytes, zipName, err := zipEmailAttachment(safeName, data)
	if err != nil || len(zipBytes) == 0 {
		return out
	}
	zipAtt := emailAttachment{data: zipBytes, filename: zipName, mime: "application/zip"}
	if len(data) >= emailZipPreferMinBytes {
		return []emailAttachment{zipAtt, fileAtt}
	}
	return []emailAttachment{fileAtt, zipAtt}
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

func mimeForFileName(fileName string) string {
	switch strings.ToLower(filepath.Ext(fileName)) {
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

func buildSuccessPlainBody(input FileEmailInput, adjuntos bool, hasNovedades bool, archiveDownloadURL, reportDownloadURL string) string {
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
		if hasNovedades {
			b.WriteString("Adjuntos: informe Excel de novedades y archivo original recibido.\n\n")
		} else {
			b.WriteString("Adjunto: archivo original recibido, sin modificaciones.\n\n")
		}
	} else {
		if hasNovedades && strings.TrimSpace(reportDownloadURL) != "" {
			b.WriteString("Descargue el informe de novedades:\n" + reportDownloadURL + "\n\n")
		}
		if strings.TrimSpace(archiveDownloadURL) != "" {
			b.WriteString("Descargue el archivo original:\n" + archiveDownloadURL + "\n\n")
		}
	}
	b.WriteString("— Busk Seguros · Procesamiento automático de inclusiones\n")
	return strings.TrimSpace(b.String())
}

func buildSuccessHTMLBody(input FileEmailInput, adjuntos bool, hasNovedades bool, archiveDownloadURL, reportDownloadURL string) string {
	tpl, err := template.New("success-email").Parse(successEmailTemplate)
	if err != nil {
		return strings.ReplaceAll(buildSuccessPlainBody(input, adjuntos, hasNovedades, archiveDownloadURL, reportDownloadURL), "\n", "<br>")
	}
	data := struct {
		FileName             string
		ProductID            string
		ProcesadoEn          string
		HasNovedades         bool
		Adjuntos             bool
		DescargaArchivoURL   string
		DescargaInformeURL   string
	}{
		FileName:           strings.TrimSpace(input.FileName),
		ProductID:          emptyFallback(input.ProductID, "No identificado"),
		HasNovedades:       hasNovedades,
		Adjuntos:           adjuntos,
		DescargaArchivoURL: strings.TrimSpace(archiveDownloadURL),
		DescargaInformeURL: strings.TrimSpace(reportDownloadURL),
	}
	if !input.ProcessedAt.IsZero() {
		data.ProcesadoEn = input.ProcessedAt.UTC().Format("02/01/2006 15:04") + " (UTC)"
	} else {
		data.ProcesadoEn = "No disponible"
	}
	var b bytes.Buffer
	if err := tpl.Execute(&b, data); err != nil {
		return strings.ReplaceAll(buildSuccessPlainBody(input, adjuntos, hasNovedades, archiveDownloadURL, reportDownloadURL), "\n", "<br>")
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
                <strong>Acción requerida:</strong> revise el Excel adjunto (hojas Incidencias e Informes).
                {{else if .DescargaInformeURL}}
                <strong>Acción requerida:</strong> descargue el informe Excel desde el enlace indicado más abajo (el adjunto no pudo enviarse por tamaño).
                {{else}}
                <strong>Acción requerida:</strong> revise el archivo en el panel de administración o vuelva a procesar cuando esté corregido.
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
                <strong>Informe adjunto (Excel):</strong> solo hojas Incidencias e Informes (no incluye Datos archivo).
              </p>
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
                {{if .HasNovedades}}
                <strong>Adjuntos:</strong> informe Excel de novedades y archivo original recibido.
                {{else}}
                <strong>Adjunto:</strong> archivo original recibido, sin modificaciones.
                {{end}}
              </p>
            </div>
            {{else if .DescargaInformeURL}}
            <div style="background:#eef9f1;padding:16px;border-radius:6px;margin-bottom:8px;">
              <p style="margin:0;font-size:14px;line-height:1.5;color:#0d6b3a;">
                <strong>Descargar informe de novedades:</strong>
                <a href="{{.DescargaInformeURL}}" style="color:#0d6b3a;">{{.DescargaInformeURL}}</a>
              </p>
            </div>
            {{end}}
            {{if and (not .Adjuntos) .DescargaArchivoURL}}
            <div style="background:#eef9f1;padding:16px;border-radius:6px;margin-bottom:8px;">
              <p style="margin:0;font-size:14px;line-height:1.5;color:#0d6b3a;">
                <strong>Descargar archivo original:</strong>
                <a href="{{.DescargaArchivoURL}}" style="color:#0d6b3a;">{{.DescargaArchivoURL}}</a>
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
