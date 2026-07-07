package processor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/buskseguros-design/services/api/internal/model"
	"github.com/buskseguros-design/services/api/internal/notify"
	sftpclient "github.com/buskseguros-design/services/api/internal/sftp"
	"github.com/buskseguros-design/services/api/internal/store"
	"github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
)

type Service struct {
	store      *store.Store
	notifier   notify.FileNotifier
	jobs       chan queuedJob
	once       sync.Once
	workers    int
	progressMu sync.RWMutex
	progress   map[string]ProgressInfo
}

type queuedJob struct {
	ID       string
	FileName string
}

type ProgressInfo struct {
	FileID    string    `json:"file_id"`
	FileName  string    `json:"file_name"`
	Status    string    `json:"status"`
	Percent   int       `json:"percent"`
	Step      string    `json:"step"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LastError string    `json:"last_error,omitempty"`
	ProductID string    `json:"product_id,omitempty"`
}

var errDuplicateFileHash = errors.New("duplicate file hash already processed")

type ruleRuntimeConfig struct {
	DateLayouts                []string
	MapfreCancelKeywords       []string
	RequiredValidDateFields    []string
	AllowedPremiums            []float64
	AgeMin                     float64
	AgeMax                     float64
	HasAgeLimits               bool
	BolivarDebtManualThreshold  float64
	BolivarPrimaCalcTolerance   float64
	BolivarPlazoDiasTolerance        int
	BolivarDueReferenceMonthOffset   int  // mes mínimo vs calendario: -1 = mes vencido (en mayo se carga abril)
	BolivarValidateDueMonth          bool // E.10: vencimiento < mes facturación → revisar prima; prima 0 → cancelación sin incidencia por fecha
	MapfreRequireCurrentMonth    bool
	MapfreDateToleranceDays      int // tolerancia vigencias (plazo); no confundir con edad
	AgeMaxDaysBeforeBirthday     int // días antes del cumpleaños tope (75.997 → 1 = hasta el día anterior al 76)
}

func New(st *store.Store) *Service {
	workers := processorWorkersFromEnv()
	s := &Service{
		store:    st,
		notifier: notify.NewFileNotifierFromEnv(),
		jobs:     make(chan queuedJob, 256),
		workers:  workers,
		progress: make(map[string]ProgressInfo),
	}
	log.Printf("[processor] inicializado workers=%d queue_capacity=%d", workers, cap(s.jobs))
	log.Printf("[processor] PROCESSOR_READ_FULL_FILE_ON_ROW_ERRORS=%t", processorReadFullFileOnRowErrorsFromEnv())
	s.startWorker()
	return s
}

func (s *Service) startWorker() {
	s.once.Do(func() {
		for i := 0; i < s.workers; i++ {
			workerID := i + 1
			go func(id int) {
				log.Printf("[processor] worker_%d listo", id)
				for job := range s.jobs {
					log.Printf("[processor] worker_%d tomó file_id=%s file=%s", id, job.ID, job.FileName)
					s.updateProgress(job.ID, job.FileName, "PROCESSING", 0, "iniciando procesamiento", "", "")
					s.store.AddFileRecord(model.FileProcessRecord{
						ID:          job.ID,
						FileName:    job.FileName,
						Status:      model.FileStatusProcessing,
						RemotePath:  job.FileName,
						ProcessedAt: time.Now().UTC(),
					})
					rec := s.processByName(job)
					s.store.AddFileRecord(rec)
					s.notifyFileProcessing(rec)
					finalErr := rec.ErrorReason
					s.updateProgress(job.ID, job.FileName, string(rec.Status), 100, "finalizado", rec.ProductID, finalErr)
					log.Printf("[processor] worker_%d finalizó file_id=%s status=%s", id, job.ID, rec.Status)
				}
			}(workerID)
		}
	})
}

func (s *Service) notifyFileProcessing(rec model.FileProcessRecord) {
	if rec.Status != model.FileStatusProcessed && rec.Status != model.FileStatusError {
		return
	}
	err := s.notifier.NotifyFileProcessing(notify.FileEmailInput{
		FileID:               rec.ID,
		FileName:             rec.FileName,
		ProductID:            rec.ProductID,
		Status:               string(rec.Status),
		ErrorReason:          rec.ErrorReason,
		ValidationReportJSON: rec.ValidationReportJSON,
		ArchivePath:          rec.ArchivePath,
		ProcessedAt:          rec.ProcessedAt,
	})
	if err != nil {
		log.Printf("[notify] fallo envio correo file_id=%s status=%s err=%v", rec.ID, rec.Status, err)
		if setErr := s.store.SetFileEmailError(rec.ID, err.Error()); setErr != nil {
			log.Printf("[notify] no se pudo guardar email_error file_id=%s err=%v", rec.ID, setErr)
		}
		return
	}
	if rec.Status == model.FileStatusProcessed {
		log.Printf("[notify] correo de éxito enviado file_id=%s", rec.ID)
	} else {
		log.Printf("[notify] correo de error enviado file_id=%s", rec.ID)
	}
}

// ScanAndEnqueue lee el SFTP y encola archivos para proceso asíncrono con pool de workers.
func (s *Service) ScanAndEnqueue() (int, error) {
	cfg, err := sftpclient.NewFromEnv()
	if err != nil {
		return 0, err
	}
	c, err := sftpclient.Connect(cfg)
	if err != nil {
		return 0, err
	}
	defer c.Close()

	entries, err := c.ListRootFiles()
	if err != nil {
		return 0, err
	}
	log.Printf("[processor] scan encontró entradas=%d", len(entries))

	candidates := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isSpreadsheet(name) {
			continue
		}
		candidates = append(candidates, name)
	}

	// Prioriza bases de stock según diagrama (procesarlas primero).
	sort.SliceStable(candidates, func(i, j int) bool {
		pi := filePriority(candidates[i])
		pj := filePriority(candidates[j])
		if pi == pj {
			return candidates[i] < candidates[j]
		}
		return pi > pj
	})

	enqueued := 0
	for _, name := range candidates {
		job := queuedJob{
			ID:       fmt.Sprintf("file_%d", time.Now().UnixNano()),
			FileName: name,
		}
		s.updateProgress(job.ID, job.FileName, "QUEUED", 0, "encolado", "", "")
		s.store.AddFileRecord(model.FileProcessRecord{
			ID:          job.ID,
			FileName:    job.FileName,
			Status:      model.FileStatusQueued,
			RemotePath:  job.FileName,
			ProcessedAt: time.Now().UTC(),
		})
		select {
		case s.jobs <- job:
			enqueued++
			log.Printf("[processor] encolado file_id=%s file=%s cola_ocupada=%d", job.ID, job.FileName, len(s.jobs))
		default:
			log.Printf("[processor] cola llena, se omite archivo=%s", name)
			s.store.AddFileRecord(model.FileProcessRecord{
				ID:          job.ID,
				FileName:    job.FileName,
				Status:      model.FileStatusError,
				ErrorReason: "cola de procesamiento llena",
				RemotePath:  job.FileName,
				ProcessedAt: time.Now().UTC(),
			})
			s.updateProgress(job.ID, job.FileName, "ERROR", 100, "cola llena", "", "cola de procesamiento llena")
		}
	}
	return enqueued, nil
}

// RetryFileByID re-encola un archivo específico (normalmente en ERROR) para reprocesarlo.
// Crea un nuevo file_id para mantener trazabilidad de intentos.
func (s *Service) RetryFileByID(fileID string) (string, string, error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return "", "", fmt.Errorf("file_id es obligatorio")
	}
	rec, ok := s.store.GetFileRecordByID(fileID)
	if !ok {
		return "", "", fmt.Errorf("file_id no encontrado")
	}
	if rec.Status != model.FileStatusError {
		return "", "", fmt.Errorf("solo se permite reintentar archivos en estado ERROR")
	}

	job := queuedJob{
		ID:       fmt.Sprintf("file_%d", time.Now().UnixNano()),
		FileName: rec.FileName,
	}
	s.updateProgress(job.ID, job.FileName, "QUEUED", 0, "reintento encolado", rec.ProductID, "")
	s.store.AddFileRecord(model.FileProcessRecord{
		ID:          job.ID,
		FileName:    job.FileName,
		ProductID:   rec.ProductID,
		Status:      model.FileStatusQueued,
		RemotePath:  rec.RemotePath,
		ProcessedAt: time.Now().UTC(),
	})

	select {
	case s.jobs <- job:
		return job.ID, job.FileName, nil
	default:
		s.store.AddFileRecord(model.FileProcessRecord{
			ID:          job.ID,
			FileName:    job.FileName,
			ProductID:   rec.ProductID,
			Status:      model.FileStatusError,
			ErrorReason: "cola de procesamiento llena",
			RemotePath:  rec.RemotePath,
			ProcessedAt: time.Now().UTC(),
		})
		s.updateProgress(job.ID, job.FileName, "ERROR", 100, "cola llena", rec.ProductID, "cola de procesamiento llena")
		return "", "", fmt.Errorf("cola de procesamiento llena")
	}
}

func (s *Service) processByName(job queuedJob) model.FileProcessRecord {
	start := time.Now()
	log.Printf("[processor] iniciando file_id=%s file=%s", job.ID, job.FileName)
	cfg, err := sftpclient.NewFromEnv()
	if err != nil {
		log.Printf("[processor] sin conexión SFTP (config): file=%q permanece en raíz remota hasta corregir env", job.FileName)
		return model.FileProcessRecord{
			ID:          job.ID,
			FileName:    job.FileName,
			Status:      model.FileStatusError,
			ErrorReason: "config sftp inválida: " + err.Error(),
			RemotePath:  job.FileName,
			ProcessedAt: time.Now().UTC(),
		}
	}
	c, err := sftpclient.Connect(cfg)
	if err != nil {
		log.Printf("[processor] sin conexión SFTP: file=%q permanece en raíz remota err=%v", job.FileName, err)
		return model.FileProcessRecord{
			ID:          job.ID,
			FileName:    job.FileName,
			Status:      model.FileStatusError,
			ErrorReason: "no se pudo conectar a sftp: " + err.Error(),
			RemotePath:  job.FileName,
			ProcessedAt: time.Now().UTC(),
		}
	}
	defer c.Close()
	s.updateProgress(job.ID, job.FileName, "PROCESSING", 5, "conectado a sftp", "", "")
	rec := s.processOne(c, job)
	log.Printf("[processor] fin file_id=%s status=%s elapsed_ms=%d", job.ID, rec.Status, time.Since(start).Milliseconds())
	return rec
}

// moveRemoteFile renombia el archivo desde la raíz configurada del SFTP a la subcarpeta ERROR o PROCESSED.
// Si falla (permisos, carpeta, archivo ya movido, etc.), devuelve "" y deja trazado en log para operación manual.
func moveRemoteFile(c *sftpclient.Client, fileName, folder string) string {
	dst, err := c.MoveToFolder(fileName, folder)
	if err != nil {
		log.Printf("[processor] sftp MOVE falló file=%q dest_folder=%q err=%v", fileName, folder, err)
		return ""
	}
	log.Printf("[processor] sftp MOVE OK file=%q -> %s", fileName, dst)
	return dst
}

func policyRowHasBlockingIssues(p *model.PolicyRecord) bool {
	if strings.EqualFold(strings.TrimSpace(p.PolicyStatus), "MANUAL_REVIEW") {
		return true
	}
	var notes []string
	if strings.TrimSpace(p.ValidationJSON) != "" {
		_ = json.Unmarshal([]byte(p.ValidationJSON), &notes)
	}
	for _, n := range notes {
		if noteIsBlocking(n) {
			return true
		}
	}
	return false
}

func policiesRowSetHasBlockingIssues(policies []model.PolicyRecord) bool {
	for i := range policies {
		if policyRowHasBlockingIssues(&policies[i]) {
			return true
		}
	}
	return false
}

func countPoliciesBlockingIssues(policies []model.PolicyRecord) int {
	n := 0
	for i := range policies {
		if policyRowHasBlockingIssues(&policies[i]) {
			n++
		}
	}
	return n
}

func (s *Service) processOne(c *sftpclient.Client, job queuedJob) model.FileProcessRecord {
	fileName := job.FileName
	rec := model.FileProcessRecord{
		ID:          job.ID,
		FileName:    fileName,
		Status:      model.FileStatusPending,
		RemotePath:  fileName,
		ProcessedAt: time.Now().UTC(),
	}

	products := s.store.FindProductFormatCandidates(fileName)
	if len(products) == 0 {
		rec.Status = model.FileStatusError
		rec.ErrorReason = "no existe producto configurado para el prefijo del archivo"
		rec.ProcessedPath = moveRemoteFile(c, fileName, "ERROR")
		if rec.ProcessedPath == "" {
			rec.ErrorReason += " (no se pudo mover a carpeta ERROR/ en SFTP)"
		}
		return rec
	}
	s.updateProgress(job.ID, job.FileName, "PROCESSING", 10, "producto/formato identificado", "", "")

	reader, err := c.OpenRelative(fileName)
	if err != nil {
		rec.Status = model.FileStatusError
		rec.ErrorReason = "no se pudo abrir archivo remoto: " + err.Error()
		rec.ProcessedPath = moveRemoteFile(c, fileName, "ERROR")
		if rec.ProcessedPath == "" {
			rec.ErrorReason += " (no se pudo mover a carpeta ERROR/ en SFTP)"
		}
		return rec
	}
	defer reader.Close()
	s.updateProgress(job.ID, job.FileName, "PROCESSING", 15, "leyendo archivo", "", "")

	policies, fileHash, archivePath, selectedProductID, err := validateFile(reader, rec.ID, fileName, products, s)
	if err != nil {
		if errors.Is(err, errDuplicateFileHash) {
			rec.Status = model.FileStatusSkipped
			rec.ErrorReason = "archivo omitido: mismo contenido (SHA-256) ya procesado u omitido"
			rec.FileHash = fileHash
			rec.ArchivePath = archivePath
			rec.ProcessedPath = moveRemoteFile(c, fileName, "PROCESSED")
			if rec.ProcessedPath == "" {
				rec.ErrorReason += " | SFTP: no se pudo mover el archivo a PROCESSED/"
			}
			return rec
		}
		rec.Status = model.FileStatusError
		rec.ErrorReason = err.Error()
		rec.FileHash = fileHash
		rec.ArchivePath = archivePath
		rec.ProcessedPath = moveRemoteFile(c, fileName, "ERROR")
		if rec.ProcessedPath == "" {
			rec.ErrorReason += " | SFTP: no se pudo mover el archivo a ERROR/"
		}
		return rec
	}
	rec.ProductID = selectedProductID
	rec.FileHash = fileHash
	rec.ArchivePath = archivePath
	rec.ProcessedAt = time.Now().UTC()
	rec.ValidationReportJSON = ""

	if policiesRowSetHasBlockingIssues(policies) {
		nIssue := countPoliciesBlockingIssues(policies)
		summary := fmt.Sprintf(
			"carga omitida: %d filas con incidencias; ninguna póliza persistida (informe en validation_report)",
			nIssue,
		)
		report := store.BuildFileValidationReportFromPolicies(
			rec.ID,
			fileName,
			selectedProductID,
			string(model.FileStatusError),
			summary,
			rec.ProcessedAt.UTC().Format(time.RFC3339Nano),
			policies,
		)
		b, marshErr := json.Marshal(report)
		if marshErr != nil {
			rec.Status = model.FileStatusError
			rec.ErrorReason = "no se pudo serializar informe de validación: " + marshErr.Error()
			rec.ProcessedPath = moveRemoteFile(c, fileName, "ERROR")
			if rec.ProcessedPath == "" {
				rec.ErrorReason += " | SFTP: no se pudo mover el archivo a ERROR/"
			}
			return rec
		}
		rec.ValidationReportJSON = string(b)
		rec.ReportArchivePath = saveValidationReportArchive(report, rec.ID, fileName)
		rec.Status = model.FileStatusError
		rec.ErrorReason = summary
		rec.ProcessedPath = moveRemoteFile(c, fileName, "ERROR")
		if rec.ProcessedPath == "" {
			rec.ErrorReason += " | SFTP: no se pudo mover el archivo a ERROR/"
		}
		log.Printf("[processor] solo_informe file_id=%s filas_con_incidencias=%d", rec.ID, nIssue)
		return rec
	}

	if err := s.store.InsertPolicies(policies); err != nil {
		rec.Status = model.FileStatusError
		rec.ErrorReason = "no se pudieron registrar pólizas: " + err.Error()
		rec.ProcessedPath = moveRemoteFile(c, fileName, "ERROR")
		if rec.ProcessedPath == "" {
			rec.ErrorReason += " | SFTP: no se pudo mover el archivo a ERROR/"
		}
		return rec
	}

	// Regla de corte para archivos STOCK:
	// si una póliza histórica del producto no viene en el stock actual, se cancela.
	selectedProduct, ok := findProductByID(products, selectedProductID)
	if ok && strings.Contains(strings.ToUpper(selectedProduct.Code), "STOCK") {
		currentCredits := make([]string, 0, len(policies))
		for _, pol := range policies {
			if credit := strings.TrimSpace(pol.CreditNumber); credit != "" {
				currentCredits = append(currentCredits, credit)
			}
		}
		cancelled, err := s.store.CancelMissingStockPolicies(selectedProduct.ID, rec.ID, currentCredits)
		if err != nil {
			// La cancelación de faltantes no debe tumbar el proceso principal de carga.
			log.Printf("[processor] warning: no se pudo aplicar cancelación de stock producto=%s file_id=%s err=%v", selectedProduct.ID, rec.ID, err)
			if rec.ErrorReason == "" {
				rec.ErrorReason = "advertencia: no se pudo aplicar cancelación de stock, se cargó el archivo igualmente"
			} else {
				rec.ErrorReason += " | advertencia: no se pudo aplicar cancelación de stock"
			}
		}
		log.Printf("[processor] stock cancelaciones producto=%s file_id=%s canceladas=%d", selectedProduct.ID, rec.ID, cancelled)
	}
	if ok && isMapfreCancelacionProduct(selectedProduct.Code) {
		applied, err := s.applyMapfreCancellationsToStock(policies)
		if err != nil {
			log.Printf("[processor] warning: no se pudo aplicar anulaciones a stock file_id=%s err=%v", rec.ID, err)
			msg := "advertencia: no se pudo marcar cancelaciones en stock MAPFRE"
			if rec.ErrorReason == "" {
				rec.ErrorReason = msg
			} else {
				rec.ErrorReason += " | " + msg
			}
		} else if applied > 0 {
			log.Printf("[processor] anulacion_masiva file_id=%s filas_stock_canceladas=%d", rec.ID, applied)
		}
	}
	s.updateProgress(job.ID, job.FileName, "PROCESSING", 85, "validaciones OK", selectedProductID, "")

	rec.ValidationReportJSON, rec.ReportArchivePath = validationReportFromPolicies(
		rec.ID,
		fileName,
		selectedProductID,
		string(model.FileStatusProcessed),
		rec.ErrorReason,
		rec.ProcessedAt,
		policies,
	)
	if rec.ValidationReportJSON != "" {
		log.Printf("[processor] informe_novedades generado file_id=%s (carga OK con avisos/informes)", rec.ID)
	}

	rec.Status = model.FileStatusProcessed
	rec.ProcessedPath = moveRemoteFile(c, fileName, "PROCESSED")
	if rec.ProcessedPath == "" {
		msg := "advertencia: pólizas registradas pero el archivo no se pudo mover a PROCESSED/ en SFTP (revisar permisos o mover manualmente)"
		if rec.ErrorReason != "" {
			rec.ErrorReason += " | " + msg
		} else {
			rec.ErrorReason = msg
		}
	}
	log.Printf("[processor] terminado OK file_id=%s file=%s dst=%s", rec.ID, rec.FileName, rec.ProcessedPath)
	return rec
}

// validationReportFromPolicies construye el informe del archivo, lo guarda en disco para
// auditoría (carpeta REPORTS_ARCHIVE_DIR) y devuelve el JSON serializado (vacío cuando no hay novedades)
// junto con la ruta donde quedó el XLSX (vacía si la escritura falló).
func validationReportFromPolicies(
	fileID, fileName, productID, fileStatus, errorReason string,
	processedAt time.Time,
	policies []model.PolicyRecord,
) (string, string) {
	report := store.BuildFileValidationReportFromPolicies(
		fileID,
		fileName,
		productID,
		fileStatus,
		errorReason,
		processedAt.UTC().Format(time.RFC3339Nano),
		policies,
	)
	archivePath := saveValidationReportArchive(report, fileID, fileName)
	if report.TotalInformativeValidations == 0 && report.TotalPendingValidations == 0 {
		return "", archivePath
	}
	b, err := json.Marshal(report)
	if err != nil {
		log.Printf("[processor] no se pudo serializar informe de novedades file_id=%s err=%v", fileID, err)
		return "", archivePath
	}
	return string(b), archivePath
}

func validateFile(r io.Reader, fileID, fileName string, candidates []model.Product, svc *Service) ([]model.PolicyRecord, string, string, string, error) {
	tmp, err := os.CreateTemp("", "sftp-validate-*"+filepath.Ext(fileName))
	if err != nil {
		return nil, "", "", "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	archivePath, err := buildArchivePath(fileID, fileName)
	if err != nil {
		_ = tmp.Close()
		return nil, "", "", "", err
	}
	archive, err := os.Create(archivePath)
	if err != nil {
		_ = tmp.Close()
		return nil, "", "", "", fmt.Errorf("create archive file: %w", err)
	}
	defer archive.Close()
	// Hash del fichero = SHA-256 de todos los octetos del stream (contenido real). Cambio de contenido → hash distinto → no se omite por duplicado.
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, archive, hasher), r); err != nil {
		_ = tmp.Close()
		_ = os.Remove(archivePath)
		return nil, "", "", "", fmt.Errorf("copy remote to temp/archive: %w", err)
	}
	fileHash := hex.EncodeToString(hasher.Sum(nil))
	log.Printf("[processor] hash calculado file_id=%s file=%s sha256=%s", fileID, fileName, fileHash)
	if err := tmp.Close(); err != nil {
		return nil, fileHash, archivePath, "", fmt.Errorf("close temp file: %w", err)
	}
	if err := archive.Close(); err != nil {
		return nil, fileHash, archivePath, "", fmt.Errorf("close archive file: %w", err)
	}
	// Primer control: deduplicación por hash de contenido, antes de parsear workbook.
	if svc.store.FileHashAlreadyProcessed("", fileHash) {
		log.Printf("[processor] hash duplicado detectado file_id=%s file=%s sha256=%s", fileID, fileName, fileHash)
		return nil, fileHash, archivePath, "", errDuplicateFileHash
	}

	p, header, rows, err := selectProductCandidateFromWorkbook(tmpPath, candidates)
	if err != nil {
		return nil, fileHash, archivePath, "", err
	}
	headerIdx := make(map[string]int)
	for i, h := range header {
		headerIdx[strings.ToUpper(strings.TrimSpace(h))] = i
	}

	fieldToCol := make(map[string]int)
	for _, m := range p.Mappings {
		col, ok := columnIndexForFieldMap(header, m)
		if !ok {
			if m.Required {
				return nil, fileHash, archivePath, p.ID, fmt.Errorf("falta columna requerida: %s", m.SourceHeader)
			}
			continue
		}
		fieldToCol[m.CanonicalField] = col
	}

	policies := make([]model.PolicyRecord, 0)
	seenCredits := map[string]struct{}{}
	ruleCfg := svc.buildRuleRuntimeConfig(p)
	hasFreezeOnZeroPolicy := productFreezesOnZeroPremium(p.Code, p.Rules)
	inFileCreditCounts := make(map[string]int)
	inFileCreditRows := make(map[string][]int)
	inFileDuplicateCreditKeys := 0
	inFileDuplicateRows := 0
	if creditCol, ok := fieldToCol["credit_number"]; ok {
		codeKeyPrefix := strings.ToUpper(strings.TrimSpace(p.Code)) + "::"
		for i := p.HeaderRow; i < len(rows); i++ {
			row := rows[i]
			if rowEmpty(row) || creditCol >= len(row) {
				continue
			}
			credit := strings.TrimSpace(row[creditCol])
			if credit == "" {
				continue
			}
			key := codeKeyPrefix + credit
			inFileCreditCounts[key]++
			inFileCreditRows[key] = append(inFileCreditRows[key], i+1)
		}
		for _, c := range inFileCreditCounts {
			if c > 1 {
				inFileDuplicateCreditKeys++
				inFileDuplicateRows += c - 1
			}
		}
		if inFileDuplicateRows > 0 {
			duplicateSamples := make([]string, 0, inFileDuplicateCreditKeys)
			for key, c := range inFileCreditCounts {
				if c <= 1 {
					continue
				}
				credit := key
				if idx := strings.Index(key, "::"); idx >= 0 && idx+2 < len(key) {
					credit = key[idx+2:]
				}
				duplicateSamples = append(duplicateSamples, fmt.Sprintf("%s(x%d)", credit, c))
			}
			sort.Strings(duplicateSamples)
			log.Printf(
				"[processor] archivo=%s duplicados_credito_en_archivo creditos_duplicados=%d filas_duplicadas=%d duplicados=%v",
				fileName, inFileDuplicateCreditKeys, inFileDuplicateRows, duplicateSamples,
			)
		}
	}
	frozenCount := 0
	cancelledZeroPremiumCount := 0
	readFullFileOnRowErrors := processorReadFullFileOnRowErrorsFromEnv()
	// Por cada fila de datos: (1) todas las reglas declaradas del producto/formato en p.Rules
	// vía runRules; (2) reglas de negocio por aseguradora y parámetros de BD vía applyDiagramRules.
	for i := p.HeaderRow; i < len(rows); i++ {
		row := rows[i]
		if rowEmpty(row) {
			continue
		}
		values := make(map[string]string)
		// Incluye toda la fila por encabezado original para exponer el recurso completo
		// (ej. nombres, apellidos, oficina y demás columnas no canónicas).
		excelColOrder := make([]string, 0, len(header))
		for _, h := range header {
			if h = strings.TrimSpace(h); h != "" {
				excelColOrder = append(excelColOrder, h)
			}
		}
		for col, h := range header {
			if col >= len(row) {
				continue
			}
			key := strings.TrimSpace(h)
			if key == "" {
				continue
			}
			val := strings.TrimSpace(row[col])
			// Si hay encabezados repetidos, guardamos también una llave por posición
			// para no perder información de ninguna columna.
			if _, exists := values[key]; !exists {
				values[key] = val
				continue
			}
			values[fmt.Sprintf("%s__col_%d", key, col+1)] = val
		}
		for field, col := range fieldToCol {
			if col < len(row) {
				values[field] = strings.TrimSpace(row[col])
			}
		}
		attachFieldHeaders(values, p.Mappings)
		if len(excelColOrder) > 0 {
			if b, err := json.Marshal(excelColOrder); err == nil {
				values["_excel_column_order"] = string(b)
			}
		}
		values["_file_name"] = fileName
		values["product_id"] = p.ID
		frozen, ruleViolations := runRules(values, p.Rules)
		if !frozen && productFreezesOnZeroPremium(p.Code, p.Rules) {
			if prem, _ := parseFlexibleNumber(values["monthly_premium"]); prem == 0 {
				frozen = true
			}
		}
		status := "ACTIVE"
		notes := make([]string, 0)
		for _, rv := range ruleViolations {
			notes = append(notes, noteIncidencia(rv))
		}
		if len(ruleViolations) > 0 {
			if !readFullFileOnRowErrors {
				return nil, fileHash, archivePath, p.ID, fmt.Errorf("fila %d: %s", i+1, strings.Join(ruleViolations, "; "))
			}
			status = "MANUAL_REVIEW"
		} else if frozen {
			frozenCount++
			status = "FROZEN"
			notes = append(notes, noteInformativo(mensajePolizaCongeladaPrimaCero()))
		}
		// Si la prima es 0 y el producto no define política de congelamiento,
		// la póliza debe cancelarse.
		if !hasFreezeOnZeroPolicy {
			if prem, _ := parseFlexibleNumber(values["monthly_premium"]); prem == 0 {
				cancelledZeroPremiumCount++
				status = "CANCELLED"
				// Cancelación no es incidencia: no se guarda en validation_json ni en el informe Excel/CSV.
			}
		}
		// Reglas duras y de negocio del diagrama por producto (se acumulan todos los fallos de la fila).
		diagramHards, rowNotes := applyDiagramRules(
			p.Code,
			values,
			seenCredits,
			inFileCreditCounts,
			inFileCreditRows,
			ruleCfg,
			svc,
		)
		for _, msg := range rowNotes {
			notes = append(notes, noteInformativo(msg))
		}
		for _, msg := range diagramHards {
			notes = append(notes, noteIncidencia(msg))
		}
		if len(diagramHards) > 0 {
			if !readFullFileOnRowErrors {
				return nil, fileHash, archivePath, p.ID, fmt.Errorf("fila %d: %s", i+1, strings.Join(diagramHards, "; "))
			}
			if status == "ACTIVE" {
				status = "MANUAL_REVIEW"
			}
		}
		if isMapfreCancelacionProduct(p.Code) && status == "ACTIVE" && len(ruleViolations) == 0 && len(diagramHards) == 0 {
			status = "CANCELLED"
		}

		rawJSONBytes, _ := json.Marshal(values)
		noteJSONBytes, _ := json.Marshal(notes)
		policies = append(policies, model.PolicyRecord{
			FileID:         fileID,
			ProductID:      p.ID,
			FileName:       fileName,
			RowNumber:      i + 1,
			DocumentNumber: values["document_number"],
			CreditNumber:   values["credit_number"],
			PolicyStatus:   status,
			RawDataJSON:    string(rawJSONBytes),
			ValidationJSON: string(noteJSONBytes),
			CreatedAt:      time.Now().UTC(),
		})
	}
	annotateInFileDuplicateRowNotes(policies, p.Code)
	if frozenCount > 0 {
		log.Printf("[processor] archivo=%s filas_congeladas=%d (prima en 0 según política por producto)", fileName, frozenCount)
	}
	if cancelledZeroPremiumCount > 0 {
		log.Printf("[processor] archivo=%s filas_canceladas_prima_0_sin_politica_freeze=%d", fileName, cancelledZeroPremiumCount)
	}
	return policies, fileHash, archivePath, p.ID, nil
}

// annotateInFileDuplicateRowNotes añade en cada fila afectada una nota NOVEDAD cuando el mismo
// credit_number aparece en más de una fila del archivo (la primera fila del grupo no la recibe
// solo desde applyDiagramRules). Así el informe Excel y pending_validations listan el duplicado en todas las filas.
func annotateInFileDuplicateRowNotes(policies []model.PolicyRecord, productCode string) {
	code := strings.ToUpper(strings.TrimSpace(productCode))
	groups := make(map[string][]int)
	for i := range policies {
		c := strings.TrimSpace(policies[i].CreditNumber)
		if c == "" {
			continue
		}
		key := code + "::" + c
		groups[key] = append(groups[key], i)
	}
	for _, idxs := range groups {
		if len(idxs) < 2 {
			continue
		}
		rowNums := make([]int, 0, len(idxs))
		for _, xi := range idxs {
			rowNums = append(rowNums, policies[xi].RowNumber)
		}
		sort.Ints(rowNums)
		filasTxt := joinIntsForNote(rowNums)
		credit := strings.TrimSpace(policies[idxs[0]].CreditNumber)
		msg := noteIncidencia(mensajeCreditoDuplicadoEnArchivoPorFila(credit, filasTxt))
		for _, xi := range idxs {
			notes := decodeValidationNotesJSON(policies[xi].ValidationJSON)
			if notesContainInFileDuplicateSignal(notes) {
				continue
			}
			notes = append(notes, msg)
			b, _ := json.Marshal(notes)
			policies[xi].ValidationJSON = string(b)
		}
	}
}

func decodeValidationNotesJSON(raw string) []string {
	var notes []string
	if strings.TrimSpace(raw) == "" {
		return notes
	}
	_ = json.Unmarshal([]byte(raw), &notes)
	return notes
}

func notesContainInFileDuplicateSignal(notes []string) bool {
	for _, n := range notes {
		lower := strings.ToLower(n)
		if strings.Contains(lower, "duplicidad") ||
			strings.Contains(lower, "duplicado") ||
			strings.Contains(lower, "duplicate") ||
			strings.Contains(lower, "aparece más de una vez") ||
			strings.Contains(lower, "se repite en este mismo archivo") {
			return true
		}
	}
	return false
}

func joinIntsForNote(nums []int) string {
	if len(nums) == 0 {
		return ""
	}
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, "; ")
}

func findProductByID(items []model.Product, id string) (model.Product, bool) {
	for _, p := range items {
		if p.ID == id {
			return p, true
		}
	}
	return model.Product{}, false
}

func selectProductCandidate(rows [][]string, candidates []model.Product) (model.Product, []string, error) {
	bestScore := -1
	var best model.Product
	var bestHeader []string
	for _, c := range candidates {
		if len(rows) < c.HeaderRow || c.HeaderRow <= 0 {
			continue
		}
		header := rows[c.HeaderRow-1]
		if !hasAllRequiredHeaders(header, c.Mappings) {
			continue
		}
		score := countMappedHeaders(header, c.Mappings)
		if score > bestScore {
			bestScore = score
			best = c
			bestHeader = header
		}
	}
	if bestScore < 0 {
		return model.Product{}, nil, fmt.Errorf("ningún formato coincide con encabezados requeridos del archivo")
	}
	return best, bestHeader, nil
}

func hasAllRequiredHeaders(header []string, mappings []model.FieldMap) bool {
	hidx := make(map[string]struct{}, len(header))
	for _, h := range header {
		hidx[strings.ToUpper(strings.TrimSpace(h))] = struct{}{}
	}
	for _, m := range mappings {
		if !m.Required {
			continue
		}
		if _, ok := hidx[strings.ToUpper(strings.TrimSpace(m.SourceHeader))]; !ok {
			return false
		}
	}
	return true
}

func countMappedHeaders(header []string, mappings []model.FieldMap) int {
	hidx := make(map[string]struct{}, len(header))
	for _, h := range header {
		hidx[strings.ToUpper(strings.TrimSpace(h))] = struct{}{}
	}
	n := 0
	for _, m := range mappings {
		if _, ok := hidx[strings.ToUpper(strings.TrimSpace(m.SourceHeader))]; ok {
			n++
		}
	}
	return n
}

// productFreezesOnZeroPremium: Bolívar siempre congela prima 0 (aunque rules_json en BD esté desactualizado).
func productFreezesOnZeroPremium(productCode string, rules []model.RuleConfig) bool {
	if hasRuleType(rules, "freeze_on_zero_premium") {
		return true
	}
	code := strings.ToUpper(strings.TrimSpace(productCode))
	return strings.HasPrefix(code, "BOLIVAR")
}

func hasRuleType(rules []model.RuleConfig, ruleType string) bool {
	target := strings.ToLower(strings.TrimSpace(ruleType))
	if target == "" {
		return false
	}
	for _, r := range rules {
		if strings.ToLower(strings.TrimSpace(r.Type)) == target {
			return true
		}
	}
	return false
}

func buildArchivePath(fileID, fileName string) (string, error) {
	baseDir := strings.TrimSpace(os.Getenv("FILES_ARCHIVE_DIR"))
	if baseDir == "" {
		baseDir = "./data/files-archive"
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir archive dir: %w", err)
	}
	safeName := strings.ReplaceAll(fileName, string(filepath.Separator), "_")
	fullPath := filepath.Join(baseDir, fmt.Sprintf("%s_%s", fileID, safeName))
	return fullPath, nil
}

func (s *Service) buildRuleRuntimeConfig(p model.Product) ruleRuntimeConfig {
	cfg := ruleRuntimeConfig{
		DateLayouts:                defaultDateLayouts(),
		MapfreCancelKeywords:       []string{"ANUL", "TERMINAD", "SINIEST"},
		RequiredValidDateFields:    []string{},
		AllowedPremiums:            s.store.GetAllowedPremiums(p.ID),
		BolivarDebtManualThreshold: 20000000,
		BolivarPrimaCalcTolerance:  1.0,
		BolivarPlazoDiasTolerance:        5,
		BolivarDueReferenceMonthOffset:   -1,
		BolivarValidateDueMonth:          true,
		MapfreRequireCurrentMonth:  true,
		MapfreDateToleranceDays:    2,
	}
	if v, ok := s.store.GetGlobalRuleParam("date_layouts_csv"); ok {
		layouts := splitCSV(v)
		if len(layouts) > 0 {
			cfg.DateLayouts = mergeDateLayouts(cfg.DateLayouts, layouts)
		}
	}
	if v, ok := s.store.GetGlobalRuleParam("mapfre_cancel_keywords_csv"); ok {
		kw := splitCSV(v)
		if len(kw) > 0 {
			cfg.MapfreCancelKeywords = kw
		}
	}
	if v, ok := s.store.GetProductRuleParam(p.ID, "age_min"); ok {
		if n, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			cfg.AgeMin = n
			cfg.HasAgeLimits = true
		}
	}
	if v, ok := s.store.GetProductRuleParam(p.ID, "age_max"); ok {
		if n, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			cfg.AgeMax = n
			cfg.HasAgeLimits = true
		}
	}
	if v, ok := s.store.GetProductRuleParam(p.ID, "debt_manual_threshold"); ok {
		if n, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			cfg.BolivarDebtManualThreshold = n
		}
	}
	if v, ok := s.store.GetProductRuleParam(p.ID, "bolivar_prima_calc_tolerance"); ok {
		if n, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && n >= 0 {
			cfg.BolivarPrimaCalcTolerance = n
		}
	}
	if v, ok := s.store.GetProductRuleParam(p.ID, "bolivar_plazo_dias_tolerance"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			cfg.BolivarPlazoDiasTolerance = n
		}
	}
	if v, ok := s.store.GetProductRuleParam(p.ID, "bolivar_due_reference_month_offset"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			cfg.BolivarDueReferenceMonthOffset = n
		}
	}
	if v, ok := s.store.GetProductRuleParam(p.ID, "bolivar_validate_due_month"); ok {
		cfg.BolivarValidateDueMonth = strings.TrimSpace(v) == "1"
	}
	if v, ok := s.store.GetProductRuleParam(p.ID, "mapfre_require_current_month"); ok {
		cfg.MapfreRequireCurrentMonth = strings.TrimSpace(v) != "0"
	}
	if v, ok := s.store.GetProductRuleParam(p.ID, "mapfre_date_tolerance_days"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			cfg.MapfreDateToleranceDays = n
		}
	}
	if v, ok := s.store.GetProductRuleParam(p.ID, "age_max_days_before_birthday"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			cfg.AgeMaxDaysBeforeBirthday = n
		}
	}
	if v, ok := s.store.GetProductRuleParam(p.ID, "required_valid_date_fields_csv"); ok {
		cfg.RequiredValidDateFields = splitCSV(v)
	}
	return cfg
}

func readWorkbookRows(tmpPath, sheetName string) ([][]string, error) {
	ext := strings.ToLower(filepath.Ext(tmpPath))

	// Algunos archivos llegan con extensión .xls pero contenido .xlsx.
	// Intentamos primero con excelize cuando ext es xlsx o xls.
	if ext == ".xlsx" || ext == ".xls" {
		if rows, err := readRowsWithExcelize(tmpPath, sheetName); err == nil {
			return rows, nil
		}
	}
	if ext == ".xls" {
		return readRowsWithXLS(tmpPath)
	}
	return nil, fmt.Errorf("extensión no soportada: %s", ext)
}

func readRowsWithExcelize(tmpPath, sheetName string) ([][]string, error) {
	// ShortDatePattern fuerza a excelize a formatear las celdas de fecha (numFmtID 14, etc.) como
	// día/mes/año, evitando que las reescriba con el default US (mm-dd-yy) y altere los datos.
	f, err := excelize.OpenFile(tmpPath, excelize.Options{ShortDatePattern: "dd/mm/yyyy"})
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("sin hojas en workbook")
	}
	// Requisito operativo: siempre la primera hoja del libro (inclusiones Bolívar/MAPFRE).
	sheet := sheets[0]
	if want := strings.TrimSpace(sheetName); want != "" && !strings.EqualFold(want, sheet) {
		log.Printf("[processor] ignorando sheet_name=%q; se usa la primera hoja %q", want, sheet)
	}

	rows, err := f.GetRows(sheet, excelize.Options{ShortDatePattern: "dd/mm/yyyy", RawCellValue: false})
	if err != nil {
		return nil, fmt.Errorf("read rows %s: %w", sheet, err)
	}
	return rows, nil
}

// selectProductCandidateFromWorkbook prueba cada formato contra todas las hojas del libro
// y elige la mejor combinación por score de encabezados mapeados. Si el candidato define
// SheetName y una hoja del libro coincide, se prioriza esa hoja en desempate.
func selectProductCandidateFromWorkbook(tmpPath string, candidates []model.Product) (model.Product, []string, [][]string, error) {
	sheets, err := readAllSheetsFromWorkbook(tmpPath)
	if err != nil {
		return model.Product{}, nil, nil, err
	}
	if len(sheets) == 0 {
		return model.Product{}, nil, nil, fmt.Errorf("sin hojas en workbook")
	}

	bestScore := -1
	bestSheetPreferred := false
	var best model.Product
	var bestHeader []string
	var bestRows [][]string
	var bestSheetName string

	for _, c := range candidates {
		if c.HeaderRow <= 0 {
			continue
		}
		for _, sh := range sheets {
			if len(sh.rows) < c.HeaderRow {
				continue
			}
			header := sh.rows[c.HeaderRow-1]
			if !hasAllRequiredHeaders(header, c.Mappings) {
				continue
			}
			score := countMappedHeaders(header, c.Mappings)
			preferred := strings.TrimSpace(c.SheetName) != "" && strings.EqualFold(c.SheetName, sh.name)
			if score > bestScore || (score == bestScore && preferred && !bestSheetPreferred) {
				bestScore = score
				bestSheetPreferred = preferred
				best = c
				bestHeader = header
				bestRows = sh.rows
				bestSheetName = sh.name
			}
		}
	}
	if bestScore < 0 {
		return model.Product{}, nil, nil, fmt.Errorf("ningún formato coincide con encabezados requeridos del archivo")
	}
	log.Printf("[processor] hoja seleccionada file=%q producto=%q hoja=%q score=%d", filepath.Base(tmpPath), best.Code, bestSheetName, bestScore)
	return best, bestHeader, bestRows, nil
}

type workbookSheet struct {
	name string
	rows [][]string
}

// readAllSheetsFromWorkbook devuelve todas las hojas del libro con sus filas, en el orden
// declarado por el archivo. Prueba primero excelize (xlsx y algunos xls disfrazados) y cae
// a xls legacy sólo si la extensión es .xls y excelize falla.
func readAllSheetsFromWorkbook(tmpPath string) ([]workbookSheet, error) {
	ext := strings.ToLower(filepath.Ext(tmpPath))
	if ext == ".xlsx" || ext == ".xls" {
		if sheets, err := readAllSheetsWithExcelize(tmpPath); err == nil {
			return sheets, nil
		}
	}
	if ext == ".xls" {
		return readAllSheetsWithXLS(tmpPath)
	}
	return nil, fmt.Errorf("extensión no soportada: %s", ext)
}

func readAllSheetsWithExcelize(tmpPath string) ([]workbookSheet, error) {
	f, err := excelize.OpenFile(tmpPath, excelize.Options{ShortDatePattern: "dd/mm/yyyy"})
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	names := f.GetSheetList()
	if len(names) == 0 {
		return nil, fmt.Errorf("sin hojas en workbook")
	}
	out := make([]workbookSheet, 0, len(names))
	for _, name := range names {
		rows, err := f.GetRows(name, excelize.Options{ShortDatePattern: "dd/mm/yyyy", RawCellValue: false})
		if err != nil {
			log.Printf("[processor] no se pudo leer hoja %q en %q: %v (se ignora)", name, filepath.Base(tmpPath), err)
			continue
		}
		out = append(out, workbookSheet{name: name, rows: rows})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ninguna hoja legible en workbook")
	}
	return out, nil
}

func readAllSheetsWithXLS(tmpPath string) ([]workbookSheet, error) {
	wb, err := xls.Open(tmpPath, "utf-8")
	if err != nil {
		return nil, fmt.Errorf("open xls: %w", err)
	}
	nSheets := wb.NumSheets()
	if nSheets == 0 {
		return nil, fmt.Errorf("sin hojas en xls")
	}
	out := make([]workbookSheet, 0, nSheets)
	for idx := 0; idx < nSheets; idx++ {
		sheet := wb.GetSheet(idx)
		if sheet == nil {
			continue
		}
		rows := make([][]string, 0, int(sheet.MaxRow)+1)
		for i := 0; i <= int(sheet.MaxRow); i++ {
			r := sheet.Row(i)
			if r == nil {
				rows = append(rows, []string{})
				continue
			}
			cols := r.LastCol()
			row := make([]string, cols)
			for c := 0; c < cols; c++ {
				row[c] = strings.TrimSpace(r.Col(c))
			}
			rows = append(rows, row)
		}
		out = append(out, workbookSheet{name: sheet.Name, rows: rows})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ninguna hoja legible en xls")
	}
	return out, nil
}

func readRowsWithXLS(tmpPath string) ([][]string, error) {
	wb, err := xls.Open(tmpPath, "utf-8")
	if err != nil {
		return nil, fmt.Errorf("open xls: %w", err)
	}
	sheet := wb.GetSheet(0)
	if sheet == nil {
		return nil, fmt.Errorf("sin hojas en xls")
	}

	rows := make([][]string, 0, int(sheet.MaxRow)+1)
	for i := 0; i <= int(sheet.MaxRow); i++ {
		r := sheet.Row(i)
		if r == nil {
			rows = append(rows, []string{})
			continue
		}
		cols := r.LastCol()
		row := make([]string, cols)
		for c := 0; c < cols; c++ {
			row[c] = strings.TrimSpace(r.Col(c))
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// runRules ejecuta todas las reglas del producto y devuelve incidencias en lenguaje de negocio (español).
func runRules(values map[string]string, rules []model.RuleConfig) (frozen bool, violations []string) {
	for _, r := range rules {
		tipo := strings.ToLower(strings.TrimSpace(r.Type))
		if tipo == "freeze_on_zero_premium" {
			n, _ := parseFlexibleNumber(strings.TrimSpace(values[r.Field]))
			if n == 0 {
				frozen = true
			}
			continue
		}
		violations = append(violations, mensajeReglaFormato(r, values, frozen)...)
	}
	return frozen, violations
}

// normalizeDashesForNumber convierte guiones tipográficos y menos Unicode al guión ASCII
// que el resto del parser ya trata como "sin valor numérico" → 0.
func normalizeDashesForNumber(s string) string {
	return strings.NewReplacer(
		"\u2013", "-", // en dash
		"\u2014", "-", // em dash
		"\u2212", "-", // minus sign
		"\u2010", "-", // hyphen
		"\u2011", "-", // non-breaking hyphen
		"\uFE58", "-", // small em dash
		"\uFE63", "-", // small hyphen-minus
		"\u00A0", "", // nbsp (evita que "1 234" quede raro tras trim parcial)
	).Replace(s)
}

// parseFlexibleNumber interpreta montos con coma/punto como miles o decimales.
// Si no hay dígitos válidos o el parse falla, devuelve 0 (sin error), para que
// reglas como number_gte y freeze_on_zero_premium traten "-" y celdas no numéricas como prima 0.
func parseFlexibleNumber(raw string) (float64, error) {
	s := strings.TrimSpace(normalizeDashesForNumber(raw))
	if s == "" || s == "-" {
		return 0, nil
	}

	// Limpia símbolos comunes y deja solo dígitos + separadores.
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) || r == ',' || r == '.' || r == '-' {
			b.WriteRune(r)
		}
	}
	s = b.String()
	if s == "" || s == "-" {
		return 0, nil
	}

	lastComma := strings.LastIndex(s, ",")
	lastDot := strings.LastIndex(s, ".")

	parseFinal := func(s string) (float64, error) {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, nil
		}
		return f, nil
	}

	// Si tiene ambos separadores, asumimos que el último es decimal.
	if lastComma >= 0 && lastDot >= 0 {
		if lastComma > lastDot {
			// 12.345,67 -> 12345.67
			s = strings.ReplaceAll(s, ".", "")
			s = strings.ReplaceAll(s, ",", ".")
		} else {
			// 12,345.67 -> 12345.67
			s = strings.ReplaceAll(s, ",", "")
		}
		return parseFinal(s)
	}

	// Solo coma: puede ser miles (8,600) o decimal (8,6)
	if lastComma >= 0 {
		parts := strings.Split(s, ",")
		if len(parts) > 1 {
			last := parts[len(parts)-1]
			// Si el último bloque tiene 3 dígitos, tratamos coma como miles.
			if len(last) == 3 {
				s = strings.ReplaceAll(s, ",", "")
			} else {
				// Si no, tratamos coma como decimal.
				s = strings.ReplaceAll(s, ",", ".")
			}
		}
		return parseFinal(s)
	}

	// Solo punto: puede ser miles (20.000.000) o decimal (20.5)
	if lastDot >= 0 {
		parts := strings.Split(s, ".")
		if len(parts) > 1 {
			last := parts[len(parts)-1]
			if len(last) == 3 {
				s = strings.ReplaceAll(s, ".", "")
			}
		}
	}

	return parseFinal(s)
}

func applyDiagramRules(
	productCode string,
	values map[string]string,
	seenCredits map[string]struct{},
	inFileCreditCounts map[string]int,
	inFileCreditRows map[string][]int,
	cfg ruleRuntimeConfig,
	svc *Service,
) (hardViolations []string, softNotes []string) {
	code := strings.ToUpper(productCode)
	softNotes = make([]string, 0)

	if isMapfreCancelacionProduct(code) {
		credit := strings.TrimSpace(values["credit_number"])
		if credit != "" {
			key := code + "::" + credit
			if _, ok := seenCredits[key]; ok {
				repeats := inFileCreditCounts[key]
				if repeats < 2 {
					repeats = 2
				}
				hardViolations = append(hardViolations, mensajeCreditoDuplicadoEnArchivo(
					credit, repeats, joinIntsForNote(inFileCreditRows[key]),
				))
			} else {
				seenCredits[key] = struct{}{}
			}
		}
		hardViolations = append(hardViolations, mapfreCancelacionViolaciones(values, cfg, svc.store)...)
		return hardViolations, softNotes
	}

	hardViolations = append(hardViolations, mensajesFechasRequeridas(values, cfg, code)...)

	if cfg.HasAgeLimits {
		mapfreSheet := strings.HasPrefix(code, "MAPFRE")
		det := evaluarEdadDetalle(values, cfg, mapfreSheet, code)
		if strings.HasPrefix(code, "BOLIVAR") {
			hardViolations = append(hardViolations, bolivarViolacionesEdad(values, cfg, det)...)
		} else {
			hardViolations = append(hardViolations, violacionesEdadGenerico(code, cfg, det, mapfreSheet)...)
		}
	}

	// OP BT / crédito duplicado intra-lote e histórico.
	credit := strings.TrimSpace(values["credit_number"])
	if credit != "" {
		key := code + "::" + credit
		if _, ok := seenCredits[key]; ok {
			repeats := inFileCreditCounts[key]
			if repeats < 2 {
				repeats = 2
			}
			hardViolations = append(hardViolations, mensajeCreditoDuplicadoEnArchivo(
				credit, repeats, joinIntsForNote(inFileCreditRows[key]),
			))
		} else {
			seenCredits[key] = struct{}{}
			if svc != nil && svc.store != nil &&
				(svc.store.PolicyCreditExists(values["product_id"], credit) || svc.store.PolicyCreditExists(codeToProductID(code), credit)) {
				hardViolations = append(hardViolations, mensajeCreditoDuplicadoHistorico())
			}
		}
	}

	if strings.HasPrefix(code, "BOLIVAR") {
		bh, bs := applyBolivarDiagramRules(values, cfg)
		hardViolations = append(hardViolations, bh...)
		softNotes = append(softNotes, bs...)
		return hardViolations, softNotes
	}

	// MAPFRE reglas duras: edad ingreso/permanencia, vigencias y planes (no aplica a anulaciones masivas).
	if strings.HasPrefix(code, "MAPFRE") && code != "MAPFRE_ANULACION" && code != "MAPFRE_ANULACION_MASIVA" {
		// Catálogo de primas permitidas configurable por producto en BD.
		hardViolations = append(hardViolations, validarPlanMapfre(code, values)...)

		if cfg.MapfreRequireCurrentMonth && !mapfreInicioVigenciaEnMesTrabajo(values["coverage_start_date"], cfg.DateLayouts) {
			hardViolations = append(hardViolations, mensajeInicioVigenciaFueraMesTrabajo(values["coverage_start_date"], time.Now().UTC()))
		}
		if diffDays, coherente := mapfreFinVigenciaPlazoCoherente(
			values["coverage_start_date"],
			values["initial_term_months"],
			values["coverage_end_date"],
			cfg.DateLayouts,
			cfg.MapfreDateToleranceDays,
		); !coherente {
			hardViolations = append(hardViolations, mensajeFinVigenciaIncoherentePlazo(
				values["coverage_start_date"],
				values["initial_term_months"],
				values["coverage_end_date"],
				diffDays,
				cfg.MapfreDateToleranceDays,
				cfg.DateLayouts,
			))
		}
	}

	nameUpper := strings.ToUpper(values["_file_name"])
	if strings.HasPrefix(code, "MAPFRE") {
		for _, kw := range cfg.MapfreCancelKeywords {
			if strings.Contains(nameUpper, strings.ToUpper(strings.TrimSpace(kw))) {
				softNotes = append(softNotes, mensajeFlujoCancelacionesMapfre())
				break
			}
		}
	}

	return hardViolations, softNotes
}

func parseDate(raw string) time.Time {
	return parseDateWithLayouts(raw, defaultDateLayouts())
}

// numericDatePattern: día/mes/año o mes/día/año con los mismos separadores (/, -, .).
var numericDatePattern = regexp.MustCompile(`^(\d{1,2})[/.-](\d{1,2})[/.-](\d{2,4})$`)

var dateTimeSuffixPattern = regexp.MustCompile(`^(.+?)(?:\s+\d{1,2}:\d{2}(?::\d{2})?)?\s*$`)

type dateFieldOrder int

const (
	dateOrderDMY dateFieldOrder = iota // día-mes-año (primero)
	dateOrderMDY                       // mes-día-año (si falla la validación con DMY)
)

// parseDateWithLayouts → calendario canónico (nacimiento / fechas generales).
func parseDateWithLayouts(raw string, layouts []string) time.Time {
	return parseDateField(raw, layouts, dateYearContextBirth)
}

func parseDateWithLayoutsOrder(raw string, layouts []string, order dateFieldOrder) time.Time {
	return parseDateFieldOrder(raw, layouts, order, dateYearContextBirth)
}

func parseVigenciaDateWithLayoutsOrder(raw string, layouts []string, order dateFieldOrder) time.Time {
	return parseDateFieldOrder(raw, layouts, order, dateYearContextVigencia)
}

func dateParsedWithOrder(raw string, layouts []string, order dateFieldOrder) bool {
	return !parseDateFieldOrder(raw, layouts, order, dateYearContextBirth).IsZero()
}

func fechaNacimientoParseable(raw string, layouts []string, mapfreSheet bool) bool {
	return !parseBirthDate(raw, layouts, mapfreSheet).IsZero()
}

func fechaNacimientoParseableProducto(productCode, raw string, layouts []string, mapfreSheet bool) bool {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(productCode)), "BOLIVAR") {
		return fechaNacimientoParseableBolivar(raw, layouts)
	}
	return fechaNacimientoParseable(raw, layouts, mapfreSheet)
}

func fechaNacimientoParseableBolivar(raw string, layouts []string) bool {
	return len(bolivarBirthDateLecturas(raw, layouts)) > 0
}

// bolivarBirthDateLecturas: única lectura de nacimiento como día/mes/año.
func bolivarBirthDateLecturas(raw string, layouts []string) []time.Time {
	t := parseDateField(raw, layouts, dateYearContextBirth)
	if t.IsZero() {
		return nil
	}
	return []time.Time{t}
}

// edadCumpleRangoBolivarDual: si alguna lectura de nacimiento cumple el rango, la póliza es válida (sin novedad de edad).
func edadCumpleRangoBolivarDual(
	birthRaw string,
	layouts []string,
	ref time.Time,
	ageMin, ageMax float64,
	diasAntesMax int,
) (cumple bool, edadReportada int) {
	minYears := ageLimitMinInt(ageMin)
	maxBirthdayYear := ageLimitMaxBirthdayYear(ageMax)
	type lectura struct {
		birth time.Time
		age   int
	}
	var lecturas []lectura
	for _, b := range bolivarBirthDateLecturas(birthRaw, layouts) {
		lecturas = append(lecturas, lectura{birth: b, age: completedYearsBetween(b, ref)})
	}
	if len(lecturas) == 0 {
		return false, -1
	}
	edadPeor := lecturas[0].age
	mejorEdad := -1
	for _, l := range lecturas {
		if l.age > edadPeor {
			edadPeor = l.age
		}
		if !edadEnRangoCalendario(l.birth, ref, minYears, maxBirthdayYear, diasAntesMax) {
			continue
		}
		if mejorEdad < 0 || l.age < mejorEdad {
			mejorEdad = l.age
		}
	}
	if mejorEdad >= 0 {
		return true, mejorEdad
	}
	return false, edadPeor
}

// mapfreInicioVigenciaEnMesTrabajo: el archivo cargado en mes N debe traer INICIO VIGENCIA del mes N-1
// (regla operativa: las inclusiones de mayo se cargan en junio, las de junio en julio, etc.).
// Solo se compara el mes/año; el día puede ser cualquiera dentro del mes esperado.
func mapfreInicioVigenciaEnMesTrabajo(startRaw string, layouts []string) bool {
	start := parseDateField(startRaw, layouts, dateYearContextVigencia)
	if start.IsZero() {
		return false
	}
	expectedYear, expectedMonth := mesAnteriorAlCargue(time.Now().UTC())
	return start.Year() == expectedYear && start.Month() == expectedMonth
}

// mesAnteriorAlCargue devuelve año/mes del mes inmediatamente anterior al mes de cargue (rollover de año en enero).
func mesAnteriorAlCargue(now time.Time) (int, time.Month) {
	prev := now.AddDate(0, -1, 0)
	return prev.Year(), prev.Month()
}

// mapfreFinVigenciaPlazoCoherente valida inicio+plazo vs fin de vigencia (no calcula edad).
// Si no hay plazo o las fechas no se interpretan (diff 0), se omite la regla: la fila sigue
// como válida (p. ej. fin de vigencia ajustado por cancelación en otro archivo/momento).
func mapfreFinVigenciaPlazoCoherente(startRaw, termRaw, endRaw string, layouts []string, tolerance int) (diffDays int, ok bool) {
	term, _ := parseFlexibleNumber(termRaw)
	if term <= 0 {
		return 0, true
	}
	bestDiff := 0
	hadPair := false
	start := parseDateField(startRaw, layouts, dateYearContextVigencia)
	end := parseDateField(endRaw, layouts, dateYearContextVigencia)
	if !start.IsZero() && !end.IsZero() {
		hadPair = true
		expected := start.AddDate(0, int(term), 0)
		diff := int(end.Sub(expected).Hours() / 24)
		if diff >= -tolerance && diff <= tolerance {
			return diff, true
		}
		bestDiff = diff
	}
	if !hadPair {
		return 0, true
	}
	return bestDiff, false
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func isYearFirstDateLayout(layout string) bool {
	layout = strings.TrimSpace(layout)
	return strings.HasPrefix(layout, "2006")
}

func normalizeTwoDigitYearBirth(y int) int {
	return resolveYear(y, dateYearContextBirth)
}

func normalizeTwoDigitYearVigencia(y int) int {
	return resolveYear(y, dateYearContextVigencia)
}

func calendarDateUTC(month, day, year int) time.Time {
	if month < 1 || month > 12 || day < 1 || day > 31 || year < 1 {
		return time.Time{}
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Day() != day || int(t.Month()) != month {
		return time.Time{}
	}
	return t
}

func normalizeTwoDigitYear(y int) int {
	return normalizeTwoDigitYearBirth(y)
}

// normalizeDateRaw limpia valores típicos de Excel antes de parsear (espacios, hora, apóstrofo, serial).
func normalizeDateRaw(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = strings.TrimPrefix(s, "'")
	s = strings.ReplaceAll(s, "\u00a0", " ")
	for _, old := range []string{"∕", "⁄", "／"} {
		s = strings.ReplaceAll(s, old, "/")
	}
	// Guiones tipográficos de Excel (no son el ASCII '-' del patrón de fechas).
	for _, h := range []string{
		"\u2010", "\u2011", "\u2012", "\u2013", "\u2014", "\u2212", "−",
	} {
		s = strings.ReplaceAll(s, h, "-")
	}
	if m := dateTimeSuffixPattern.FindStringSubmatch(s); len(m) >= 2 {
		s = strings.TrimSpace(m[1])
	}
	// "17 / 11 / 1963" → "17/11/1963"
	s = strings.ReplaceAll(s, " ", "")
	if strings.Contains(s, ".") && !strings.ContainsAny(s, "/-") {
		if n, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64); err == nil && n >= 1 && n <= 60000 {
			return s // serial Excel; no quitar decimales
		}
	}
	if i := strings.Index(s, "."); i > 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// parseExcelSerialDateString convierte serial Excel (días desde 1899-12-30) cuando no hay separadores de fecha.
func parseExcelSerialDateString(s string) (time.Time, bool) {
	if strings.ContainsAny(s, "/-.") {
		return time.Time{}, false
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
	if err != nil || n < 1 || n > 60000 {
		return time.Time{}, false
	}
	days := int(n)
	frac := n - float64(days)
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	t := base.AddDate(0, 0, days).Add(time.Duration(frac * 24 * float64(time.Hour)))
	if t.Year() < 1900 || t.Year() > 2100 {
		return time.Time{}, false
	}
	return t, true
}

// activationDateRaw: fecha de activación para edad de ingreso (diagrama Bolívar: FECHA ADJUDICACION del crédito).
func activationDateRaw(values map[string]string, productCode string) string {
	for _, key := range []string{"activation_date", "loan_award_date", "coverage_start_date"} {
		if v := strings.TrimSpace(values[key]); v != "" {
			return v
		}
	}
	code := strings.ToUpper(strings.TrimSpace(productCode))
	if strings.HasPrefix(code, "BOLIVAR") {
		for key, val := range values {
			upper := strings.ToUpper(strings.TrimSpace(key))
			if strings.Contains(upper, "ADJUDIC") || strings.Contains(upper, "ACTIVAC") {
				if v := strings.TrimSpace(val); v != "" {
					return v
				}
			}
		}
	}
	return ""
}

// ageReferenceAtActivation devuelve la fecha de activación parseada (edad = nacimiento → activación).
func ageReferenceAtActivation(values map[string]string, layouts []string, mapfreSheet bool, productCode string) (time.Time, bool) {
	raw := activationDateRaw(values, productCode)
	if raw == "" {
		return time.Time{}, false
	}
	t := parseAgeReferenceDate(raw, layouts, mapfreSheet, productCode)
	return t, !t.IsZero()
}

// completedYearsBetween devuelve años cumplidos (enteros) entre nacimiento y referencia.
func completedYearsBetween(birth, ref time.Time) int {
	if birth.IsZero() || ref.IsZero() || birth.After(ref) {
		return -1
	}
	years := ref.Year() - birth.Year()
	if ref.Month() < birth.Month() || (ref.Month() == birth.Month() && ref.Day() < birth.Day()) {
		years--
	}
	return years
}

func edadCumplidaConOrden(birthRaw string, layouts []string, ref time.Time, order dateFieldOrder) int {
	birth := parseDateWithLayoutsOrder(birthRaw, layouts, order)
	return completedYearsBetween(birth, ref)
}

// edadValidacionDetalle reúne fechas y edades usadas en la validación (para el informe).
type edadValidacionDetalle struct {
	cumple          bool
	edadReportada   int
	birthRaw        string
	activacionRaw   string
	refValid        bool
	ref             time.Time
	refEtiqueta     string
	refValorArchivo string
}

func evaluarEdadDetalle(values map[string]string, cfg ruleRuntimeConfig, mapfreSheet bool, productCode string) edadValidacionDetalle {
	birthRaw := strings.TrimSpace(values["birth_date"])
	activacionRaw := activationDateRaw(values, productCode)
	layouts := cfg.DateLayouts
	ref, refOK := ageReferenceAtActivation(values, layouts, mapfreSheet, productCode)
	d := edadValidacionDetalle{
		birthRaw:        birthRaw,
		activacionRaw:   activacionRaw,
		refValid:        refOK,
		ref:             ref,
		refEtiqueta:     "fecha de activación",
		refValorArchivo: activacionRaw,
	}
	diasAntesEdad := ageDaysBeforeTopBirthday(cfg.AgeMaxDaysBeforeBirthday)
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(productCode)), "BOLIVAR") {
		d.cumple, d.edadReportada = edadCumpleRangoBolivarDual(birthRaw, layouts, ref, cfg.AgeMin, cfg.AgeMax, diasAntesEdad)
		return d
	}
	d.cumple, d.edadReportada = edadCumpleRangoEstricto(birthRaw, layouts, ref, cfg.AgeMin, cfg.AgeMax, diasAntesEdad, mapfreSheet)
	return d
}

// bolivarViolacionesEdad: edad fuera de rango; fechas inválidas las reportan otras reglas (sin duplicar).
func bolivarViolacionesEdad(values map[string]string, cfg ruleRuntimeConfig, det edadValidacionDetalle) []string {
	switch {
	case strings.TrimSpace(det.birthRaw) == "":
		return nil
	case det.activacionRaw == "":
		return []string{mensajeFechaActivacionObligatoriaBolivar()}
	case !det.refValid, !fechaNacimientoParseableBolivar(det.birthRaw, cfg.DateLayouts):
		return nil
	case !det.cumple && bolivarAplicaIncidenciaEdadFueraRango(values, cfg):
		return []string{mensajeEdadFueraDeRango("BOLIVAR", det, cfg.AgeMin, cfg.AgeMax, ageDaysBeforeTopBirthday(cfg.AgeMaxDaysBeforeBirthday))}
	}
	return nil
}

func violacionesEdadGenerico(code string, cfg ruleRuntimeConfig, det edadValidacionDetalle, mapfreSheet bool) []string {
	switch {
	case strings.TrimSpace(det.birthRaw) == "":
		return nil
	case det.activacionRaw == "":
		return nil
	case !det.refValid, !fechaNacimientoParseable(det.birthRaw, cfg.DateLayouts, mapfreSheet):
		return nil
	case !det.cumple:
		return []string{mensajeEdadFueraDeRango(code, det, cfg.AgeMin, cfg.AgeMax, ageDaysBeforeTopBirthday(cfg.AgeMaxDaysBeforeBirthday))}
	}
	return nil
}

func formatFechaCalendario(t time.Time) string {
	return FormatDateCanonical(t)
}

// edadCumpleRangoEstricto valida el rango con nacimiento y referencia en día/mes/año.
func edadCumpleRangoEstricto(birthRaw string, layouts []string, ref time.Time, ageMin, ageMax float64, diasAntesMax int, mapfreSheet bool) (cumple bool, edadReportada int) {
	_ = mapfreSheet
	minYears := ageLimitMinInt(ageMin)
	maxBirthdayYear := ageLimitMaxBirthdayYear(ageMax)
	birth := parseBirthDate(birthRaw, layouts, false)
	if birth.IsZero() {
		return false, -1
	}
	age := completedYearsBetween(birth, ref)
	if age >= maxBirthdayYear {
		return false, age
	}
	maxDate := fechaLimiteMaxEdad(birth, ageMax, diasAntesMax)
	if !maxDate.IsZero() && ref.After(maxDate) {
		return false, age
	}
	if edadEnRangoCalendario(birth, ref, minYears, maxBirthdayYear, diasAntesMax) {
		return true, age
	}
	return false, age
}

func sameCalendarDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// ageDaysBeforeTopBirthday: margen de edad (75 años 364 días = 1 día antes del cumpleaños 76). Vigencias siguen con mapfre_date_tolerance_days.
func ageDaysBeforeTopBirthday(cfgDays int) int {
	if cfgDays > 0 {
		return cfgDays
	}
	return 1
}

// edadEnRangoCalendario: activación >= cumpleaños minYears; activación <= último día antes del cumpleaños maxBirthdayYear.
func edadEnRangoCalendario(birth, ref time.Time, minYears, maxBirthdayYear, diasAntesTop int) bool {
	if birth.IsZero() || ref.IsZero() {
		return false
	}
	minDate := birth.AddDate(minYears, 0, 0)
	maxDate := fechaLimiteMaxEdadFromBirth(birth, maxBirthdayYear, diasAntesTop)
	return !ref.Before(minDate) && !ref.After(maxDate)
}

func fechaLimiteMaxEdadFromBirth(birth time.Time, maxBirthdayYear, diasAntesTop int) time.Time {
	if birth.IsZero() || maxBirthdayYear <= 0 {
		return time.Time{}
	}
	d := ageDaysBeforeTopBirthday(diasAntesTop)
	return birth.AddDate(maxBirthdayYear, 0, -d)
}

func fechaLimiteMaxEdad(birth time.Time, ageMax float64, diasAntesTop int) time.Time {
	return fechaLimiteMaxEdadFromBirth(birth, ageLimitMaxBirthdayYear(ageMax), diasAntesTop)
}

// ageLimitMaxBirthdayYear: 75.997 → tope en cumpleaños 76; 65.997 → 66.
func ageLimitMaxBirthdayYear(max float64) int {
	if max <= 0 {
		return 0
	}
	return int(math.Floor(max + 1.0 + 1e-9))
}

// edadCumpleRango (legacy tests): delega en la validación estricta (formato no MAPFRE).
func edadCumpleRango(birthRaw string, layouts []string, ref time.Time, ageMin, ageMax float64) (cumple bool, edad int) {
	return edadCumpleRangoEstricto(birthRaw, layouts, ref, ageMin, ageMax, 2, false)
}

func ageLimitMinInt(min float64) int {
	if min <= 0 {
		return 0
	}
	return int(math.Ceil(min - 1e-9))
}

func ageLimitMaxInt(max float64) int {
	if max <= 0 {
		return 0
	}
	return int(math.Floor(max + 1e-9))
}

func defaultDateLayouts() []string {
	// ISO (año-mes-día) y numérico día/mes/año. No se admiten layouts mes/día/año.
	return []string{
		"2006-01-02", "2006/01/02",
		"02/01/2006", "2/1/2006", "02-01-2006",
		"02-01-06", "2-1-06", "02/01/06", "2/1/06",
	}
}

func mergeDateLayouts(base, extra []string) []string {
	out := make([]string, 0, len(base)+len(extra))
	seen := make(map[string]struct{}, len(base)+len(extra))
	appendUnique := func(items []string) {
		for _, it := range items {
			v := strings.TrimSpace(it)
			if v == "" {
				continue
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	appendUnique(base)
	appendUnique(extra)
	return out
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		item := strings.TrimSpace(p)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func inAllowed(raw string, allowed []string) bool {
	v := strings.TrimSpace(strings.ToUpper(raw))
	if v == "" {
		return false
	}
	for _, a := range allowed {
		if strings.Contains(v, strings.ToUpper(a)) {
			return true
		}
	}
	return false
}

func numberInAllowed(raw string, allowed []float64) bool {
	n, err := parseFlexibleNumber(raw)
	if err != nil {
		return false
	}
	for _, a := range allowed {
		if n == a {
			return true
		}
	}
	return false
}

func codeToProductID(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "MAPFRE_VIDA", "MAPFRE_INCLUSION_VIDA_VOLUNTARIO":
		return "mapfre_inclusion_vida_voluntario"
	case "MAPFRE_ACC_MEN", "MAPFRE_INCLUSION_AP_MENORES":
		return "mapfre_inclusion_ap_menores"
	case "MAPFRE_CANCER", "MAPFRE_INCLUSION_AP_CANCER":
		return "mapfre_inclusion_ap_cancer"
	case "MAPFRE_STOCK", "MAPFRE_STOCK_CARTERA":
		return "mapfre_stock_cartera"
	case "BOLIVAR_STOCK", "BOLIVAR_ESAL", "BOLIVAR_DEUDORES_STOCK_ESAL":
		return "bolivar_deudores_stock_esal"
	case "BOLIVAR_BANCO", "BOLIVAR_INCLUSION_DEUDORES_BANCO":
		return "bolivar_inclusion_deudores_banco"
	case "MAPFRE_ANULACION", "MAPFRE_ANULACION_MASIVA":
		return "mapfre_anulacion_masiva"
	default:
		return strings.ToLower(code)
	}
}

func processorWorkersFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("PROCESSOR_WORKERS"))
	if raw == "" {
		return 2
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 2
	}
	if n > 8 {
		return 8
	}
	return n
}

// processorReadFullFileOnRowErrorsFromEnv controla el comportamiento ante error en una fila
// (reglas de formato runRules o reglas de negocio applyDiagramRules).
// true (defecto): se sigue leyendo el archivo y cada fila problemática se guarda con notas NOVEDAD y MANUAL_REVIEW cuando aplica.
// false: el primer error de fila aborta el archivo (sin insertar pólizas).
func processorReadFullFileOnRowErrorsFromEnv() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("PROCESSOR_READ_FULL_FILE_ON_ROW_ERRORS")))
	if raw == "" {
		return true
	}
	switch raw {
	case "0", "false", "no", "off", "fail_fast", "strict":
		return false
	default:
		return true
	}
}

func rowEmpty(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func isSpreadsheet(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".xlsx" || ext == ".xls" || ext == ".csv"
}

func filePriority(name string) int {
	n := strings.ToUpper(strings.TrimSpace(name))
	switch {
	case strings.Contains(n, "STOCK"):
		return 100
	case strings.Contains(n, "BASE") && strings.Contains(n, "STOCK"):
		return 100
	case strings.Contains(n, "INCLUSION"):
		return 50
	default:
		return 10
	}
}

func (s *Service) updateProgress(fileID, fileName, status string, pct int, step, productID, lastError string) {
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	now := time.Now().UTC()
	createdAt := now
	if prev, ok := s.progress[fileID]; ok && !prev.CreatedAt.IsZero() {
		createdAt = prev.CreatedAt
	}
	s.progress[fileID] = ProgressInfo{
		FileID:    fileID,
		FileName:  fileName,
		Status:    status,
		Percent:   pct,
		Step:      step,
		CreatedAt: createdAt,
		UpdatedAt: now,
		LastError: lastError,
		ProductID: productID,
	}
}

func (s *Service) SnapshotProgress() []ProgressInfo {
	s.progressMu.RLock()
	defer s.progressMu.RUnlock()
	out := make([]ProgressInfo, 0, len(s.progress))
	for _, p := range s.progress {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}
