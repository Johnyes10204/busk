package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/buskseguros-design/services/api/internal/config"
	"github.com/buskseguros-design/services/api/internal/model"
	"github.com/buskseguros-design/services/api/internal/processor"
	"github.com/buskseguros-design/services/api/internal/store"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

type policyResponseItem struct {
	ID              string         `json:"id"`
	FileID          string         `json:"file_id"`
	ProductID       string         `json:"product_id"`
	FileName        string         `json:"file_name"`
	RowNumber       int            `json:"row_number"`
	Status          string         `json:"status"`
	DocumentNumber  string         `json:"document_number,omitempty"`
	CreditNumber    string         `json:"credit_number,omitempty"`
	CustomerData    map[string]any `json:"customer_data"`
	FinancialData   map[string]any `json:"financial_data"`
	ReferenceData   map[string]any `json:"reference_data"`
	ValidationData  map[string]any `json:"validation_data"`
	RawData         map[string]any `json:"raw_data,omitempty"`
	ValidationNotes []string       `json:"validation_notes,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
}

type allowedPremiumUpsertRequest struct {
	ProductID string    `json:"product_id"`
	Premiums  []float64 `json:"premiums"`
}

type allowedPremiumAddRequest struct {
	ProductID string  `json:"product_id"`
	Premium   float64 `json:"premium"`
}

type productFormatsUpsertRequest struct {
	ID         string             `json:"id"`
	ProductID  string             `json:"product_id"`
	Name       string             `json:"name"`
	FilePrefix string             `json:"file_prefix"`
	SheetName  string             `json:"sheet_name"`
	HeaderRow  int                `json:"header_row"`
	Priority   int                `json:"priority"`
	Active     *bool              `json:"active"`
	Mappings   []model.FieldMap   `json:"mappings"`
	Rules      []model.RuleConfig `json:"rules"`
}

type productFormatActiveRequest struct {
	ID     string `json:"id"`
	Active bool   `json:"active"`
}

type formatMatchTestRequest struct {
	FileName  string   `json:"file_name"`
	ProductID string   `json:"product_id"`
	Headers   []string `json:"headers"`
}

func loadDotEnv() {
	config.LoadAndApply()
	candidates := []string{
		".env",
		filepath.Join("services", "api", ".env"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := godotenv.Load(p); err != nil {
			log.Printf("[env] no se pudo cargar %s: %v", p, err)
			continue
		}
		log.Printf("[env] variables cargadas desde %s", p)
		return
	}
	log.Printf("[env] sin archivo .env (opcional); usando entorno del sistema o valores por defecto")
}

func main() {
	log.SetFlags(0)
	loadDotEnv()
	st, err := store.NewMySQLFromEnv()
	if err != nil {
		log.Fatalf("init mysql store: %v", err)
	}
	proc := processor.New(st)

	// Inicia escaneo automático cada 1 hora
	startAutoScanScheduler(proc)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "time": time.Now().UTC().Format(time.RFC3339)})
	})

	mux.HandleFunc("/api/v1/products", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var p model.Product
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if p.ID == "" || p.Code == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id y code son obligatorios"})
				return
			}
			// El prefijo ahora pertenece a los formatos/archivos, no al producto.
			p.FilePrefix = ""
			if len(p.Mappings) > 0 {
				if missing := validateProductConfig(p); len(missing) > 0 {
					writeJSON(w, http.StatusBadRequest, map[string]any{
						"error":          "configuración de producto incompleta",
						"missing_fields": missing,
					})
					return
				}
			}
			writeJSON(w, http.StatusCreated, st.UpsertProduct(p))
		case http.MethodGet:
			writeJSON(w, http.StatusOK, st.ListProducts())
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/product-formats", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			productID := strings.TrimSpace(r.URL.Query().Get("product_id"))
			writeJSON(w, http.StatusOK, st.ListProductFormats(productID))
		case http.MethodPost:
			var req productFormatsUpsertRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.ProductID) == "" || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.FilePrefix) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id, product_id, name y file_prefix son obligatorios"})
				return
			}
			active := true
			if req.Active != nil {
				active = *req.Active
			}
			format := model.ProductFormat{
				ID:         strings.TrimSpace(req.ID),
				ProductID:  strings.TrimSpace(req.ProductID),
				Name:       strings.TrimSpace(req.Name),
				FilePrefix: strings.TrimSpace(req.FilePrefix),
				SheetName:  strings.TrimSpace(req.SheetName),
				HeaderRow:  req.HeaderRow,
				Priority:   req.Priority,
				Active:     active,
				Mappings:   req.Mappings,
				Rules:      req.Rules,
			}
			writeJSON(w, http.StatusCreated, st.UpsertProductFormat(format))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/product-formats/active", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req productFormatActiveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if strings.TrimSpace(req.ID) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id es obligatorio"})
			return
		}
		if err := st.SetProductFormatActive(req.ID, req.Active); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":     strings.TrimSpace(req.ID),
			"active": req.Active,
		})
	})

	mux.HandleFunc("/api/v1/product-formats/match-test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req formatMatchTestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if strings.TrimSpace(req.FileName) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file_name es obligatorio"})
			return
		}
		if len(req.Headers) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "headers es obligatorio"})
			return
		}

		candidates := st.FindProductFormatCandidates(req.FileName)
		if strings.TrimSpace(req.ProductID) != "" {
			filtered := make([]model.Product, 0, len(candidates))
			for _, c := range candidates {
				if strings.EqualFold(strings.TrimSpace(c.ID), strings.TrimSpace(req.ProductID)) {
					filtered = append(filtered, c)
				}
			}
			candidates = filtered
		}
		if len(candidates) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{
				"file_name":    req.FileName,
				"product_id":   strings.TrimSpace(req.ProductID),
				"matched":      false,
				"reason":       "sin formatos candidatos para ese file_name/product_id",
				"candidates":   []any{},
				"winner":       nil,
				"header_count": len(req.Headers),
			})
			return
		}

		headerSet := make(map[string]struct{}, len(req.Headers))
		for _, h := range req.Headers {
			key := strings.ToUpper(strings.TrimSpace(h))
			if key != "" {
				headerSet[key] = struct{}{}
			}
		}
		results := make([]map[string]any, 0, len(candidates))
		bestScore := -1
		bestIdx := -1
		for i, c := range candidates {
			requiredTotal := 0
			requiredMatched := 0
			totalMapped := 0
			for _, m := range c.Mappings {
				h := strings.ToUpper(strings.TrimSpace(m.SourceHeader))
				if h == "" {
					continue
				}
				if m.Required {
					requiredTotal++
				}
				if _, ok := headerSet[h]; ok {
					totalMapped++
					if m.Required {
						requiredMatched++
					}
				}
			}
			requiredOK := requiredTotal == requiredMatched
			score := -1
			if requiredOK {
				score = totalMapped
				if score > bestScore {
					bestScore = score
					bestIdx = i
				}
			}
			results = append(results, map[string]any{
				"product_id":         c.ID,
				"product_code":       c.Code,
				"file_prefix":        c.FilePrefix,
				"sheet_name":         c.SheetName,
				"header_row":         c.HeaderRow,
				"required_total":     requiredTotal,
				"required_matched":   requiredMatched,
				"required_ok":        requiredOK,
				"mapped_match_count": totalMapped,
				"score":              score,
			})
		}

		var winner any = nil
		if bestIdx >= 0 {
			w := candidates[bestIdx]
			winner = map[string]any{
				"product_id":   w.ID,
				"product_code": w.Code,
				"file_prefix":  w.FilePrefix,
				"sheet_name":   w.SheetName,
				"header_row":   w.HeaderRow,
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"file_name":    req.FileName,
			"product_id":   strings.TrimSpace(req.ProductID),
			"matched":      bestIdx >= 0,
			"candidates":   results,
			"winner":       winner,
			"header_count": len(req.Headers),
		})
	})

	mux.HandleFunc("/api/v1/products/allowed-premiums", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			productID := strings.TrimSpace(r.URL.Query().Get("product_id"))
			if productID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query param product_id es obligatorio"})
				return
			}
			items := st.GetAllowedPremiums(productID)
			writeJSON(w, http.StatusOK, map[string]any{
				"product_id": productID,
				"count":      len(items),
				"premiums":   items,
			})
		case http.MethodPut:
			var req allowedPremiumUpsertRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if strings.TrimSpace(req.ProductID) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "product_id es obligatorio"})
				return
			}
			if err := st.UpsertAllowedPremiums(req.ProductID, req.Premiums); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"product_id": req.ProductID,
				"count":      len(st.GetAllowedPremiums(req.ProductID)),
				"premiums":   st.GetAllowedPremiums(req.ProductID),
			})
		case http.MethodPost:
			var req allowedPremiumAddRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if strings.TrimSpace(req.ProductID) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "product_id es obligatorio"})
				return
			}
			if err := st.AddAllowedPremium(req.ProductID, req.Premium); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{
				"product_id": req.ProductID,
				"count":      len(st.GetAllowedPremiums(req.ProductID)),
				"premiums":   st.GetAllowedPremiums(req.ProductID),
			})
		case http.MethodDelete:
			productID := strings.TrimSpace(r.URL.Query().Get("product_id"))
			if productID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query param product_id es obligatorio"})
				return
			}
			rawPremium := strings.TrimSpace(r.URL.Query().Get("premium"))
			if rawPremium == "" {
				if err := st.ClearAllowedPremiums(productID); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
			} else {
				v, err := strconv.ParseFloat(rawPremium, 64)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "premium inválida"})
					return
				}
				if err := st.DeleteAllowedPremium(productID, v); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"product_id": productID,
				"count":      len(st.GetAllowedPremiums(productID)),
				"premiums":   st.GetAllowedPremiums(productID),
			})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/process/scan", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		enqueued, err := proc.ScanAndEnqueue()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":   "queued",
			"enqueued": enqueued,
			"message":  "archivos escaneados y encolados para procesamiento asíncrono con workers",
		})
	})

	mux.HandleFunc("/api/v1/process/scan-local", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		dir := strings.TrimSpace(r.URL.Query().Get("dir"))
		if dir == "" {
			dir = strings.TrimSpace(os.Getenv("FILES_ARCHIVE_DIR"))
			if dir == "" {
				dir = "./data/files-archive"
			}
		}
		enqueued, err := proc.ScanAndEnqueueLocal(dir)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":   "queued",
			"enqueued": enqueued,
			"dir":      dir,
			"message":  "archivos locales encolados para procesamiento (sin SFTP)",
		})
	})

	mux.HandleFunc("/api/v1/files", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, st.ListFileRecords())
	})

	mux.HandleFunc("/api/v1/files/retry", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		fileID := strings.TrimSpace(r.URL.Query().Get("file_id"))
		if fileID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query param file_id es obligatorio"})
			return
		}
		newFileID, fileName, err := proc.RetryFileByID(fileID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":      "queued",
			"message":     "archivo reencolado para reprocesamiento",
			"source_id":   fileID,
			"new_file_id": newFileID,
			"file_name":   fileName,
		})
	})

	mux.HandleFunc("/api/v1/process/progress", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": proc.SnapshotProgress(),
		})
	})

	mux.HandleFunc("/api/v1/policies/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		credit := strings.TrimSpace(r.URL.Query().Get("credit_number"))
		doc := strings.TrimSpace(r.URL.Query().Get("document_number"))
		productID := strings.TrimSpace(r.URL.Query().Get("product_id"))
		// Permitir búsqueda paginada solo por producto.
		// Si no viene producto, exigir al menos credit_number o document_number.
		if productID == "" && credit == "" && doc == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "indique product_id o al menos uno: credit_number o document_number",
			})
			return
		}
		page := 1
		if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page inválido (entero >= 1)"})
				return
			}
			page = n
		}
		pageSize := 50
		if raw := strings.TrimSpace(r.URL.Query().Get("page_size")); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page_size inválido (entero >= 1)"})
				return
			}
			pageSize = n
		}
		const maxPageSize = 200
		if pageSize > maxPageSize {
			pageSize = maxPageSize
		}
		includeRaw := parseBoolQuery(r.URL.Query().Get("include_raw"))

		items, total := st.SearchPoliciesPage(productID, credit, doc, page, pageSize)
		totalPages := 0
		if total > 0 {
			totalPages = (total + pageSize - 1) / pageSize
		}
		hasNext := page < totalPages
		hasPrev := page > 1 && total > 0

		writeJSON(w, http.StatusOK, map[string]any{
			"product_id":        productID,
			"credit_number":     credit,
			"document_number":   doc,
			"page":              page,
			"page_size":         pageSize,
			"total":             total,
			"total_pages":       totalPages,
			"has_next_page":     hasNext,
			"has_previous_page": hasPrev,
			"count":             len(items),
			"items":             toPolicyResponseItems(items, includeRaw),
		})
	})

	mux.HandleFunc("/api/v1/policies", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		productID := strings.TrimSpace(r.URL.Query().Get("product_id"))
		if productID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query param product_id es obligatorio"})
			return
		}
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		limit := 0
		if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
			n, err := strconv.Atoi(rawLimit)
			if err != nil || n < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit inválido"})
				return
			}
			limit = n
		}
		includeRaw := parseBoolQuery(r.URL.Query().Get("include_raw"))

		items := st.ListPoliciesByProduct(productID, status, limit)
		writeJSON(w, http.StatusOK, map[string]any{
			"product_id": productID,
			"status":     status,
			"count":      len(items),
			"items":      toPolicyResponseItems(items, includeRaw),
		})
	})

	mux.HandleFunc("/api/v1/files/summary", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		fileID := strings.TrimSpace(r.URL.Query().Get("file_id"))
		if fileID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query param file_id es obligatorio"})
			return
		}
		sum, err := st.GetFileQualitySummary(fileID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file_id no encontrado"})
			return
		}
		writeJSON(w, http.StatusOK, sum)
	})

	mux.HandleFunc("/api/v1/files/validation-report", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		fileID := strings.TrimSpace(r.URL.Query().Get("file_id"))
		if fileID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query param file_id es obligatorio"})
			return
		}
		report, err := st.GetFileValidationReport(fileID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file_id no encontrado"})
			return
		}
		writeJSON(w, http.StatusOK, report)
	})

	mux.HandleFunc("/api/v1/files/validation-csv", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		fileID := strings.TrimSpace(r.URL.Query().Get("file_id"))
		if fileID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query param file_id es obligatorio"})
			return
		}
		report, err := st.GetFileValidationReport(fileID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file_id no encontrado"})
			return
		}
		content, err := store.ValidationReportClientCSV(report)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no se pudo construir CSV de validaciones"})
			return
		}
		baseName := strings.TrimSpace(report.FileName)
		if baseName == "" {
			baseName = fileID
		}
		baseName = strings.ReplaceAll(baseName, `"`, "")
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="novedades_`+baseName+`.csv"`)
		_, _ = w.Write(content)
	})

	mux.HandleFunc("/api/v1/files/validation-xlsx", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		fileID := strings.TrimSpace(r.URL.Query().Get("file_id"))
		if fileID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query param file_id es obligatorio"})
			return
		}
		// Preferimos servir el XLSX espejo archivado a disco: es la fuente autorizada
		// del reporte y es lo mismo que el correo intenta adjuntar. Además evita
		// re-generar Excel para archivos grandes cuando el JSON del reporte no cabe
		// en MySQL (max_allowed_packet).
		if rec, ok := st.GetFileRecordByID(fileID); ok && strings.TrimSpace(rec.ReportArchivePath) != "" {
			if fi, err := os.Stat(rec.ReportArchivePath); err == nil && !fi.IsDir() && fi.Size() > 0 {
				baseName := strings.TrimSpace(rec.FileName)
				if baseName == "" {
					baseName = fileID
				}
				baseName = strings.ReplaceAll(baseName, `"`, "")
				w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
				w.Header().Set("Content-Disposition", `attachment; filename="novedades_`+baseName+`.xlsx"`)
				http.ServeFile(w, r, rec.ReportArchivePath)
				return
			}
		}
		// Fallback: regenerar desde la BD (útil para archivos pequeños o cuando el
		// XLSX en disco no existe).
		report, err := st.GetFileValidationReport(fileID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file_id no encontrado"})
			return
		}
		content, err := store.ValidationReportClientXLSX(report)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no se pudo construir Excel de validaciones"})
			return
		}
		baseName := strings.TrimSpace(report.FileName)
		if baseName == "" {
			baseName = fileID
		}
		baseName = strings.ReplaceAll(baseName, `"`, "")
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="novedades_`+baseName+`.xlsx"`)
		_, _ = w.Write(content)
	})

	mux.HandleFunc("/api/v1/files/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		fileID := strings.TrimSpace(r.URL.Query().Get("file_id"))
		if fileID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query param file_id es obligatorio"})
			return
		}
		rec, ok := st.GetFileRecordByID(fileID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file_id no encontrado"})
			return
		}
		if strings.TrimSpace(rec.ArchivePath) == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "archivo no disponible para descarga (archive_path vacío)"})
			return
		}
		if _, err := os.Stat(rec.ArchivePath); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "archivo archivado no existe en disco"})
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+rec.FileName+`"`)
		http.ServeFile(w, r, rec.ArchivePath)
	})

	mux.HandleFunc("/api/v1/bootstrap/sample-products", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		seed(st)
		writeJSON(w, http.StatusCreated, map[string]string{"status": "seeded"})
	})

	addr := ":80"
	log.Printf("API escuchando en %s", addr)
	log.Fatal(http.ListenAndServe(addr, logging(mux)))
}

func startAutoScanScheduler(proc *processor.Service) {
	runScan := func() {
		log.Printf("[AUTO-SCAN] iniciando escaneo automático del SFTP")
		enqueued, err := proc.ScanAndEnqueue()
		if err != nil {
			log.Printf("[AUTO-SCAN] error: %v", err)
		} else {
			log.Printf("[AUTO-SCAN] completado: %d archivos encolados", enqueued)
		}
	}
	go func() {
		runScan()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			runScan()
		}
	}()
	log.Printf("[AUTO-SCAN] scheduler iniciado (frecuencia: cada 5 minutos, primer escaneo al arranque)")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func validateProductConfig(p model.Product) []string {
	mapped := map[string]bool{}
	for _, m := range p.Mappings {
		f := strings.TrimSpace(strings.ToLower(m.CanonicalField))
		if f != "" {
			mapped[f] = true
		}
	}

	required := []string{
		"document_number",
		"credit_number",
		"monthly_premium",
	}

	code := strings.ToUpper(strings.TrimSpace(p.Code))
	switch {
	case strings.HasPrefix(code, "MAPFRE"):
		required = append(required,
			"birth_date",
			"activation_date",
			"coverage_start_date",
			"coverage_end_date",
		)
		if code == codeMapfreInclusionVidaVoluntario {
			required = append(required, "initial_term_months")
		}
	case strings.HasPrefix(code, "BOLIVAR"):
		required = append(required,
			"birth_date",
			"activation_date",
			"initial_debt_amount",
			"rate_percent",
			"loan_award_date",
			"loan_due_date_current",
		)
	}

	missing := make([]string, 0)
	for _, f := range required {
		if !mapped[f] {
			missing = append(missing, f)
		}
	}
	return missing
}

func toPolicyResponseItems(items []model.PolicyRecord, includeRaw bool) []policyResponseItem {
	out := make([]policyResponseItem, 0, len(items))
	for _, p := range items {
		raw := map[string]any{}
		if strings.TrimSpace(p.RawDataJSON) != "" {
			_ = json.Unmarshal([]byte(p.RawDataJSON), &raw)
		}
		notes := []string{}
		if strings.TrimSpace(p.ValidationJSON) != "" {
			_ = json.Unmarshal([]byte(p.ValidationJSON), &notes)
		}
		premium, _ := parseFlexibleNumber(getFirstString(raw, "monthly_premium", "PRIMA MENSUAL", "PRIMAMENSUALPERIODO", "PRIMA"))
		resp := policyResponseItem{
			ID:             fmt.Sprintf("%s:%d", p.FileID, p.RowNumber),
			FileID:         p.FileID,
			ProductID:      p.ProductID,
			FileName:       p.FileName,
			RowNumber:      p.RowNumber,
			Status:         p.PolicyStatus,
			DocumentNumber: p.DocumentNumber,
			CreditNumber:   p.CreditNumber,
			CustomerData: compactMap(map[string]any{
				"id_number":   firstNonEmpty(p.DocumentNumber, getFirstString(raw, "document_number", "COD DOCUM", "IDENTIFICACIONAFILIADO", "IDENTIFICACION")),
				"id_type":     getFirstString(raw, "id_type", "TIPOIDENTIFICACION", "TIPO DOCUMENTO", "COD CEDULA", "TIP DOCUMENTOB1", "TIP DOCUMENTO"),
				"full_name":   joinName(getFirstString(raw, "full_name", "NOMBRES", "NOMBRE AP"), getFirstString(raw, "last_name_1", "APELLIDO PATERNO", "APELLIDO1 AP"), getFirstString(raw, "last_name_2", "APELLIDO MATERNO", "APELLIDO2 AP")),
				"gender":      getFirstString(raw, "gender", "GENERO", "SEXO"),
				"birth_date":  getFirstString(raw, "birth_date", "FECHA NAC", "FECHA DE NACIMIENTO", "FECHANACIMIENTO"),
				"phone":       getFirstString(raw, "phone", "Telefono", "TELEFONO", "CELULAR"),
				"email":       getFirstString(raw, "email", "E-mail", "CORREO"),
				"office":      getFirstString(raw, "office", "OFICINA"),
				"department":  getFirstString(raw, "DEPARTAMENTO"),
				"address":     getFirstString(raw, "DIRECCION", "DIRECCIÓN"),
				"postal_code": getFirstString(raw, "POSTAL"),
			}),
			FinancialData: compactMap(map[string]any{
				"base_value":    parseNumberOrZero(getFirstString(raw, "initial_debt_amount", "DEUDA INICIAL", "VALOR ASEGURADO")),
				"premium_value": premium,
				"total_value":   premium,
				"currency":      "COP",
				"applied_rate":  parseNumberOrNil(getFirstString(raw, "rate_percent", "%")),
			}),
			ReferenceData: compactMap(map[string]any{
				"reference_number": firstNonEmpty(p.CreditNumber, getFirstString(raw, "credit_number", "NUMERO DE CREDITO", "NUMEROPRESTAMO", "OP BT")),
				// MAPFRE: coverage_start_date / coverage_end_date. Bolívar: loan_award_date / loan_due_date_current.
				"start_date": getFirstString(raw, "coverage_start_date", "activation_date", "loan_award_date", "FECHA INICIO DE VIGENCIA", "FECHA ADJUDICACION"),
				"end_date":   getFirstString(raw, "coverage_end_date", "loan_due_date_current", "FECHAFINVIGENCIADERIESGO REAL", "FECHA VENCIMIENTO ACTUAL"),
				"additional_system_info": compactMap(map[string]any{
					"plan_code":    getFirstString(raw, "plan_code", "PLAN", "CODIGO SEGURO"),
					"plan_name":    getFirstString(raw, "plan_name", "PLAN", "PRODUCTO"),
					"loan_due":     getFirstString(raw, "loan_due_date_current", "FECHA VENCIMIENTO ACTUAL"),
					"loan_award":   getFirstString(raw, "loan_award_date", "FECHA ADJUDICACION"),
					"term_months":  getFirstString(raw, "calculated_term", "PLAZO CRÉDITO", "initial_term_months", "PLAZO INICIAL", "PLAZO INICIAL FINAL"),
					"file_name":    p.FileName,
					"product_code": p.ProductID,
				}),
			}),
			ValidationData: map[string]any{
				"is_valid":               p.PolicyStatus != "MANUAL_REVIEW",
				"alerts":                 notes,
				"requires_manual_action": p.PolicyStatus == "MANUAL_REVIEW",
			},
			CreatedAt: p.CreatedAt,
		}
		if includeRaw {
			resp.RawData = raw
		}
		resp.ValidationNotes = notes
		out = append(out, resp)
	}
	return out
}

func parseBoolQuery(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "si", "sí":
		return true
	default:
		return false
	}
}

// compactMap elimina del mapa las claves cuyo valor es string vacío o cero numérico,
// para no enviar campos sin datos en la respuesta JSON.
func compactMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case string:
			if strings.TrimSpace(val) != "" {
				out[k] = v
			}
		case float64:
			if val != 0 {
				out[k] = v
			}
		case int:
			if val != 0 {
				out[k] = v
			}
		case map[string]any:
			if inner := compactMap(val); len(inner) > 0 {
				out[k] = inner
			}
		default:
			if v != nil {
				out[k] = v
			}
		}
	}
	return out
}

func getFirstString(raw map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := raw[k]; ok {
			s := strings.TrimSpace(fmt.Sprintf("%v", v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		s := strings.TrimSpace(v)
		if s != "" {
			return s
		}
	}
	return ""
}

func joinName(parts ...string) string {
	items := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			items = append(items, s)
		}
	}
	return strings.Join(items, " ")
}

func parseNumberOrZero(raw string) float64 {
	n, _ := parseFlexibleNumber(raw)
	return n
}

func parseNumberOrNil(raw string) any {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	n, _ := parseFlexibleNumber(raw)
	return n
}

func parseFlexibleNumber(raw string) (float64, error) {
	s := strings.TrimSpace(strings.ReplaceAll(raw, ",", ""))
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return f, nil
}

// mapfreInclusionDateMappings: edad de ingreso en fecha de activación (= inicio de vigencia en inclusiones).
func mapfreInclusionDateMappings() []model.FieldMap {
	return []model.FieldMap{
		{CanonicalField: "activation_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: true},
		{CanonicalField: "coverage_start_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: true},
	}
}

// mapfreStockDateMappings: archivos con FECHAACTIVACION explícita (stock / algunos VF).
func mapfreStockDateMappings() []model.FieldMap {
	return []model.FieldMap{
		{CanonicalField: "activation_date", SourceHeader: "FECHAACTIVACION", Required: true},
		{CanonicalField: "coverage_start_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: false},
	}
}

// bolivarBancoMappings columnas del layout Deudores Banco (Anexo 4 / MICRO_BANCO_*).
// La columna "%" (sin espacio) es la tasa de prima; "% " es otra tasa (valor servicio).
func bolivarBancoMappings() []model.FieldMap {
	return []model.FieldMap{
		{CanonicalField: "document_number", SourceHeader: "IDENTIFICACION", Required: true},
		{CanonicalField: "birth_date", SourceHeader: "FECHA DE NACIMIENTO", Required: true},
		{CanonicalField: "credit_number", SourceHeader: "OP BT", Required: true},
		{CanonicalField: "initial_debt_amount", SourceHeader: "DEUDA INICIAL", Required: true},
		{CanonicalField: "monthly_premium", SourceHeader: "PRIMA MENSUAL", Required: true},
		{CanonicalField: "rate_percent", SourceHeader: "%", Required: true},
		{CanonicalField: "activation_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
		{CanonicalField: "loan_award_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
		{CanonicalField: "loan_due_date_current", SourceHeader: "FECHA VENCIMIENTO ACTUAL", Required: true},
		{CanonicalField: "calculated_term", SourceHeader: "PLAZO CRÉDITO", Required: false},
		{CanonicalField: "plan_code", SourceHeader: "CODIGO SEGURO", Required: false},
		{CanonicalField: "office", SourceHeader: "OFICINA", Required: false},
		{CanonicalField: "email", SourceHeader: "E-mail", Required: false},
		{CanonicalField: "phone", SourceHeader: "Telefono", Required: false},
		{CanonicalField: "full_name", SourceHeader: "NOMBRES", Required: false},
		{CanonicalField: "last_name_1", SourceHeader: "APELLIDO PATERNO", Required: false},
		{CanonicalField: "last_name_2", SourceHeader: "APELLIDO MATERNO", Required: false},
		// MICRO_BANCO usa "TIPO DOCUMENTO"; Pyme_BANCO y ESAL usan "TIPOIDENTIFICACION".
		{CanonicalField: "id_type", SourceHeader: "TIPOIDENTIFICACION", Aliases: []string{"TIPO DOCUMENTO"}, Required: false},
		{CanonicalField: "gender", SourceHeader: "GENERO", Required: false},
		{CanonicalField: "observacion", SourceHeader: "OBSERVACIONES ABRIL 2026", Required: false},
	}
}

func optionalPersonMappings() []model.FieldMap {
	return []model.FieldMap{
		// Nombres: ACC MEN y Cáncer usan "NOMBRE AP"; Vida VOL usa "NOMBRES" y apellidos sin espacio.
		{CanonicalField: "full_name", SourceHeader: "NOMBRE AP", Aliases: []string{"NOMBRES"}, Required: false},
		{CanonicalField: "last_name_1", SourceHeader: "APELLIDO1 AP", Aliases: []string{"APELLIDOPATERNO"}, Required: false},
		{CanonicalField: "last_name_2", SourceHeader: "APELLIDO2 AP", Aliases: []string{"APELLIDOMATERNO"}, Required: false},
		// Tipo de doc: Cáncer="COD CEDULA", ACC MEN="TIPO DOC", Vida VOL="TIPOIDENTIFICACIONAFILIADO".
		{CanonicalField: "id_type", SourceHeader: "COD CEDULA", Aliases: []string{"TIPO DOC", "TIPOIDENTIFICACIONAFILIADO"}, Required: false},
		{CanonicalField: "gender", SourceHeader: "GENERO", Aliases: []string{"SEXO"}, Required: false},
		{CanonicalField: "email", SourceHeader: "CORREO", Required: false},
		// Teléfono: ACC MEN y Cáncer="TELEFONO"; Vida VOL="CELULAR".
		{CanonicalField: "phone", SourceHeader: "TELEFONO", Aliases: []string{"CELULAR"}, Required: false},
		{CanonicalField: "address", SourceHeader: "DIRECCION", Aliases: []string{"DIRECCIÓN"}, Required: false},
		{CanonicalField: "department", SourceHeader: "DEPARTAMENTO", Required: false},
		{CanonicalField: "postal_code", SourceHeader: "POSTAL", Required: false},
	}
}

// Prefijos de archivo (match: UPPER(nombre) LIKE %prefijo%).
// Lote SFTP abril 2026 (tools/sftpconnect/downloads): prefijo numérico = contrato/producto MAPFRE.
// Cabezotes MAPFRE (Requerimiento conexiones API VF Nov 2025):
//
//	108 · Anexo 1 · Vida voluntaria
//	114 · Anexo 2 · AP Cáncer
//	110 · Anexo 3 · AP Menores
const (
	contractMapfreVida   = "5024424900103" // cabezote 108 · Anexo 1 · VIDA_VOL RM-INCLUSION
	contractMapfreAccMen = "5024524900101" // cabezote 110 · Anexo 3 · ACC MEN RM-INCLUSION
	contractMapfreCancer = "5024524900103" // cabezote 114 · Anexo 2 · CANCER RM-INCLUSION

	prefixMapfreInclusionVida        = "INCLUSION-VIDA"   // legado INCLUSION-VIDA-MAPFRE.xlsx
	prefixMapfreInclusionVidaVF      = "VOL RM-INCLUSION" // VF con FECHAACTIVACION
	prefixMapfreInclusionVidaAbril   = "VIDA_VOL RM-INCLUSION"
	prefixMapfreInclusionAccMen      = "INCLUSION-ACCIDEMENOR"
	prefixMapfreInclusionAccMenVF    = "ACC MEN RM-INCLUSION"
	prefixMapfreInclusionCancer      = "INCLUSION-CANCER"
	prefixMapfreInclusionCancerVF    = "CANCER RM-INCLUSION"
	prefixMapfreStock                = "STOCK_MAPFRE"
	prefixMapfreAnulacionMasiva      = "Anulacion masiva" // cancelaciones MAPFRE (plantilla; no confundir con Anexo 4 Bolívar)
	prefixMapfreAnulacionMasivaTilde = "Anulación masiva" // variante con tilde (algunos archivos MAPFRE la usan)

	prefixBolivarBancoMicro   = "MICRO_BANCO" // Anexo 4 · Deudores_Banco_Bolivar_MICRO_BANCO_*
	prefixBolivarBancoPyme    = "Pyme_BANCO"  // Anexo 4 · Deudores_Banco_Bolivar__Pyme_BANCO_*
	prefixBolivarEsalMicro    = "micro_ESAL"  // Anexo 5 · Deudores_ESAL_Bolivar_micro_ESAL_*
	prefixBolivarEsalPyme     = "Pyme_ESAL"   // Anexo 5 · Deudores_ESAL_Bolivar_Pyme_ESAL_*
	prefixBolivarStock        = "STOCK-"      // legado; no matchea STOCK_MAPFRE
	prefixBolivarStockFebrero = "STOCK-FEBRE-BOLIVAR"
)

// IDs y códigos de producto (BD + informes). Alineados con docs/Requerimiento conexiones API_VF_Nov_25.docx.
const (
	pidMapfreInclusionVidaVoluntario  = "mapfre_inclusion_vida_voluntario" // cabezote 108 · Anexo 1
	codeMapfreInclusionVidaVoluntario = "MAPFRE_INCLUSION_VIDA_VOLUNTARIO" // cabezote 108 · Anexo 1

	pidMapfreInclusionAPMenores  = "mapfre_inclusion_ap_menores" // cabezote 110 · Anexo 3
	codeMapfreInclusionAPMenores = "MAPFRE_INCLUSION_AP_MENORES" // cabezote 110 · Anexo 3

	pidMapfreInclusionAPCancer  = "mapfre_inclusion_ap_cancer" // cabezote 114 · Anexo 2
	codeMapfreInclusionAPCancer = "MAPFRE_INCLUSION_AP_CANCER" // cabezote 114 · Anexo 2

	pidMapfreStockCartera  = "mapfre_stock_cartera" // stock MAPFRE (diagrama operativo; fuera de Etapa 1 voluntario)
	codeMapfreStockCartera = "MAPFRE_STOCK_CARTERA"

	pidMapfreAnulacionMasiva  = "mapfre_anulacion_masiva" // cancelaciones MAPFRE (plantilla Anulacion masiva)
	codeMapfreAnulacionMasiva = "MAPFRE_ANULACION_MASIVA"

	pidBolivarInclusionDeudoresBanco  = "bolivar_inclusion_deudores_banco" // Anexo 4 · Deudores_Banco_Bolivar
	codeBolivarInclusionDeudoresBanco = "BOLIVAR_INCLUSION_DEUDORES_BANCO" // Anexo 4

	// Stock Bolívar mensual + inclusiones ESAL (Anexo 5): mismo producto, distintos formatos.
	pidBolivarDeudoresStockEsal  = "bolivar_deudores_stock_esal" // Anexo 5 + stock mensual
	codeBolivarDeudoresStockEsal = "BOLIVAR_DEUDORES_STOCK_ESAL" // Anexo 5 + stock mensual
)

// Primas permitidas en BD (catálogo + coherencia con tarifario en mapfre_plan.go).
// SDD borrador (docs/sdd.md): VOLUNTARIO Plan1=6600; en archivos reales y diagrama API usan 8600.
var (
	// Voluntario (Vida) y Stock MAPFRE comparten tarifario Plan 1 / Plan 2.
	mapfreVidaStockAllowedPremiums = []float64{8600, 17100}
	mapfreAccMenAllowedPremiums    = []float64{7800, 7410, 10600, 10070}
	mapfreCancerAllowedPremiums    = []float64{8500, 8075, 12000, 12350, 13000}
)

func mapfreInclusionBaseRules() []model.RuleConfig {
	return []model.RuleConfig{
		{Type: "required_not_empty", Field: "document_number"},
		{Type: "required_not_empty", Field: "birth_date"},
		{Type: "required_not_empty", Field: "credit_number"},
		{Type: "number_gte", Field: "monthly_premium", Params: map[string]float64{"min": 0}},
	}
}

func bolivarBancoRules() []model.RuleConfig {
	return []model.RuleConfig{
		{Type: "required_not_empty", Field: "credit_number"},
		{Type: "number_gte", Field: "initial_debt_amount", Params: map[string]float64{"min": 0}},
		{Type: "number_gte", Field: "monthly_premium", Params: map[string]float64{"min": 0}},
		{Type: "freeze_on_zero_premium", Field: "monthly_premium"},
	}
}

func seedBolivarRuleParams(st *store.Store, productID string) {
	_ = st.UpsertProductRuleParam(productID, "debt_manual_threshold", "20000000")
	_ = st.UpsertProductRuleParam(productID, "bolivar_prima_calc_tolerance", "1")
	_ = st.UpsertProductRuleParam(productID, "bolivar_plazo_dias_tolerance", "31")
	_ = st.UpsertProductRuleParam(productID, "bolivar_due_reference_month_offset", "-1")
	_ = st.UpsertProductRuleParam(productID, "bolivar_validate_due_month", "1")
	_ = st.UpsertProductRuleParam(productID, "age_min", "18")
	_ = st.UpsertProductRuleParam(productID, "age_max", "75.997")
	_ = st.UpsertProductRuleParam(productID, "age_max_days_before_birthday", "1")
	_ = st.UpsertProductRuleParam(productID, "required_valid_date_fields_csv", "birth_date,activation_date,loan_award_date,loan_due_date_current")
}

func mapfreAccMenInclusionMappings() []model.FieldMap {
	return append([]model.FieldMap{
		{CanonicalField: "document_number", SourceHeader: "NUM DOCUM", Required: true},
		{CanonicalField: "birth_date", SourceHeader: "FECHA NAC", Required: true},
		{CanonicalField: "monthly_premium", SourceHeader: "PRIMA", Required: true},
		{CanonicalField: "credit_number", SourceHeader: "NUMERO DE CREDITO", Required: true},
		{CanonicalField: "activation_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: true},
		{CanonicalField: "coverage_start_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: true},
		{CanonicalField: "coverage_end_date", SourceHeader: "FECHAFINVIGENCIADERIESGO REAL", Required: true},
		{CanonicalField: "initial_term_months", SourceHeader: "PLAZO", Required: true},
		{CanonicalField: "plan_name", SourceHeader: "PLAN", Required: false},
		{CanonicalField: "plan_code", SourceHeader: "COD PLAN", Required: false},
		{CanonicalField: "insured_amount", SourceHeader: "VALOR ASEGURADO", Required: false},
	}, optionalPersonMappings()...)
}

func mapfreCancerInclusionMappings() []model.FieldMap {
	return append([]model.FieldMap{
		{CanonicalField: "document_number", SourceHeader: "COD CEDULA", Required: true},
		{CanonicalField: "birth_date", SourceHeader: "FECHA NAC", Required: true},
		{CanonicalField: "monthly_premium", SourceHeader: "PRIMA", Required: true},
		{CanonicalField: "credit_number", SourceHeader: "NUMERO DE CREDITO", Required: true},
		{CanonicalField: "activation_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: true},
		{CanonicalField: "coverage_start_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: true},
		{CanonicalField: "coverage_end_date", SourceHeader: "FECHAFINVIGENCIADERIESGO REAL", Required: true},
		{CanonicalField: "initial_term_months", SourceHeader: "PLAZO", Required: true},
		{CanonicalField: "plan_name", SourceHeader: "PLAN", Required: false},
		{CanonicalField: "plan_code", SourceHeader: "PLAN", Required: false},
		{CanonicalField: "insured_amount", SourceHeader: "VALOR ASEGURADO", Required: false},
	}, optionalPersonMappings()...)
}

// bolivarEsalMappings: Deudores ESAL (FECHA VENCIMIENTO sin "ACTUAL").
func bolivarEsalMappings() []model.FieldMap {
	return []model.FieldMap{
		{CanonicalField: "document_number", SourceHeader: "IDENTIFICACION", Required: true},
		{CanonicalField: "birth_date", SourceHeader: "FECHA DE NACIMIENTO", Required: true},
		{CanonicalField: "credit_number", SourceHeader: "OP BT", Required: true},
		{CanonicalField: "initial_debt_amount", SourceHeader: "DEUDA INICIAL", Required: true},
		{CanonicalField: "monthly_premium", SourceHeader: "PRIMA MENSUAL", Required: true},
		{CanonicalField: "rate_percent", SourceHeader: "%", Required: true},
		{CanonicalField: "activation_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
		{CanonicalField: "loan_award_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
		{CanonicalField: "loan_due_date_current", SourceHeader: "FECHA VENCIMIENTO", Required: true},
		{CanonicalField: "calculated_term", SourceHeader: "PLAZO CRÉDITO", Required: false},
		{CanonicalField: "plan_code", SourceHeader: "CODIGO SEGURO", Required: false},
		{CanonicalField: "office", SourceHeader: "OFICINA", Required: false},
		{CanonicalField: "email", SourceHeader: "E-mail", Required: false},
		{CanonicalField: "phone", SourceHeader: "Telefono", Required: false},
		{CanonicalField: "full_name", SourceHeader: "NOMBRES", Required: false},
		{CanonicalField: "last_name_1", SourceHeader: "APELLIDO PATERNO", Required: false},
		{CanonicalField: "last_name_2", SourceHeader: "APELLIDO MATERNO", Required: false},
		{CanonicalField: "id_type", SourceHeader: "TIPOIDENTIFICACION", Required: false},
		{CanonicalField: "gender", SourceHeader: "GENERO", Required: false},
	}
}

// mapfreVidaInclusionVFMappings: inclusiones con FECHAACTIVACION (febrero/abril VF, no solo inicio vigencia).
func mapfreVidaInclusionVFMappings() []model.FieldMap {
	return append([]model.FieldMap{
		{CanonicalField: "document_number", SourceHeader: "IDENTIFICACIONAFILIADO", Required: true},
		{CanonicalField: "birth_date", SourceHeader: "FECHANACIMIENTO", Required: true},
		{CanonicalField: "monthly_premium", SourceHeader: "PRIMAMENSUALPERIODO", Required: true},
		{CanonicalField: "credit_number", SourceHeader: "NUMEROPRESTAMO", Required: true},
		{CanonicalField: "activation_date", SourceHeader: "FECHAACTIVACION", Required: true},
		{CanonicalField: "coverage_start_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: false},
		{CanonicalField: "coverage_end_date", SourceHeader: "FECHAFINVIGENCIADERIESGO REAL", Required: true},
		{CanonicalField: "initial_term_months", SourceHeader: "PLAZO INICIAL", Required: true},
		{CanonicalField: "plan_name", SourceHeader: "NOMBREPLAN", Required: false},
		{CanonicalField: "plan_code", SourceHeader: "CODIGO SEGURO", Required: false},
		{CanonicalField: "insured_amount", SourceHeader: "VALORARECONOCER", Required: false},
		{CanonicalField: "office", SourceHeader: "OFICINA", Required: false},
	}, optionalPersonMappings()...)
}

func seed(st *store.Store) {
	_ = st.UpsertGlobalRuleParam("date_layouts_csv", "2006-01-02,02/01/2006,2/1/2006,02-01-2006,2006/01/02,02-01-06,2-1-06,02/01/06,2/1/06")
	_ = st.UpsertGlobalRuleParam("mapfre_cancel_keywords_csv", "ANUL,TERMINAD,SINIEST")

	st.UpsertProduct(model.Product{
		ID:         pidMapfreInclusionVidaVoluntario,
		Code:       codeMapfreInclusionVidaVoluntario,
		Insurer:    "MAPFRE",
		FilePrefix: prefixMapfreInclusionVida,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Mappings: append([]model.FieldMap{
			{CanonicalField: "document_number", SourceHeader: "IDENTIFICACIONAFILIADO", Required: true},
			{CanonicalField: "birth_date", SourceHeader: "FECHANACIMIENTO", Required: true},
			{CanonicalField: "monthly_premium", SourceHeader: "PRIMAMENSUALPERIODO", Required: true},
			{CanonicalField: "credit_number", SourceHeader: "NUMEROPRESTAMO", Required: true},
		}, append(append(mapfreInclusionDateMappings(), []model.FieldMap{
			{CanonicalField: "coverage_end_date", SourceHeader: "FECHAFINVIGENCIADERIESGO REAL", Required: true},
			{CanonicalField: "initial_term_months", SourceHeader: "PLAZO INICIAL", Required: true},
			{CanonicalField: "plan_name", SourceHeader: "NOMBREPLAN", Required: false},
			{CanonicalField: "plan_code", SourceHeader: "CODIGO SEGURO", Required: false},
			{CanonicalField: "insured_amount", SourceHeader: "VALORARECONOCER", Required: false},
		}...), optionalPersonMappings()...)...),
		Rules: mapfreInclusionBaseRules(),
	})
	_ = st.UpsertAllowedPremiums(pidMapfreInclusionVidaVoluntario, mapfreVidaStockAllowedPremiums)
	_ = st.UpsertProductRuleParam(pidMapfreInclusionVidaVoluntario, "age_min", "18")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionVidaVoluntario, "age_max", "75.997")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionVidaVoluntario, "mapfre_require_current_month", "1")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionVidaVoluntario, "mapfre_date_tolerance_days", "2")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionVidaVoluntario, "age_max_days_before_birthday", "1")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionVidaVoluntario, "required_valid_date_fields_csv", "birth_date,activation_date,coverage_start_date,coverage_end_date")
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "mapfre_vida_fmt_default",
		ProductID:  pidMapfreInclusionVidaVoluntario,
		Name:       "Anexo 1 · cabezote 108 · default",
		FilePrefix: prefixMapfreInclusionVida,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Priority:   100,
		Active:     true,
		Mappings: append([]model.FieldMap{
			{CanonicalField: "document_number", SourceHeader: "IDENTIFICACIONAFILIADO", Required: true},
			{CanonicalField: "birth_date", SourceHeader: "FECHANACIMIENTO", Required: true},
			{CanonicalField: "monthly_premium", SourceHeader: "PRIMAMENSUALPERIODO", Required: true},
			{CanonicalField: "credit_number", SourceHeader: "NUMEROPRESTAMO", Required: true},
			{CanonicalField: "activation_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: true},
			{CanonicalField: "coverage_start_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: true},
			{CanonicalField: "coverage_end_date", SourceHeader: "FECHAFINVIGENCIADERIESGO REAL", Required: true},
			{CanonicalField: "initial_term_months", SourceHeader: "PLAZO INICIAL", Required: true},
			{CanonicalField: "plan_name", SourceHeader: "NOMBREPLAN", Required: false},
			{CanonicalField: "plan_code", SourceHeader: "CODIGO SEGURO", Required: false},
			{CanonicalField: "insured_amount", SourceHeader: "VALORARECONOCER", Required: false},
		}, optionalPersonMappings()...),
		Rules: mapfreInclusionBaseRules(),
	})
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "mapfre_vida_fmt_inclusion_febrero_2026_vf",
		ProductID:  pidMapfreInclusionVidaVoluntario,
		Name:       "Anexo 1 · cabezote 108 · VF (FECHAACTIVACION)",
		FilePrefix: prefixMapfreInclusionVidaVF,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Priority:   130,
		Active:     true,
		Mappings:   mapfreVidaInclusionVFMappings(),
		Rules:      mapfreInclusionBaseRules(),
	})
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "mapfre_vida_fmt_contract",
		ProductID:  pidMapfreInclusionVidaVoluntario,
		Name:       "Anexo 1 · cabezote 108 · contrato 5024424900103",
		FilePrefix: contractMapfreVida,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Priority:   150,
		Active:     true,
		Mappings:   mapfreVidaInclusionVFMappings(),
		Rules:      mapfreInclusionBaseRules(),
	})
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "mapfre_vida_fmt_inclusion_abril_2026",
		ProductID:  pidMapfreInclusionVidaVoluntario,
		Name:       "Anexo 1 · cabezote 108 · inclusion abril 2026 (VIDA_VOL)",
		FilePrefix: prefixMapfreInclusionVidaAbril,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Priority:   135,
		Active:     true,
		Mappings:   mapfreVidaInclusionVFMappings(),
		Rules:      mapfreInclusionBaseRules(),
	})

	st.UpsertProduct(model.Product{
		ID:         pidMapfreInclusionAPMenores,
		Code:       codeMapfreInclusionAPMenores,
		Insurer:    "MAPFRE",
		FilePrefix: prefixMapfreInclusionAccMen,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Mappings:   mapfreAccMenInclusionMappings(),
		Rules:      mapfreInclusionBaseRules(),
	})
	_ = st.UpsertAllowedPremiums(pidMapfreInclusionAPMenores, mapfreAccMenAllowedPremiums)
	_ = st.UpsertProductRuleParam(pidMapfreInclusionAPMenores, "age_min", "18")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionAPMenores, "age_max", "65.997")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionAPMenores, "mapfre_require_current_month", "1")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionAPMenores, "mapfre_date_tolerance_days", "2")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionAPMenores, "age_max_days_before_birthday", "1")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionAPMenores, "required_valid_date_fields_csv", "birth_date,activation_date,coverage_start_date,coverage_end_date")
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "mapfre_acc_men_fmt_default",
		ProductID:  pidMapfreInclusionAPMenores,
		Name:       "Anexo 3 · cabezote 110 · default",
		FilePrefix: prefixMapfreInclusionAccMen,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Priority:   100,
		Active:     true,
		Mappings:   mapfreAccMenInclusionMappings(),
		Rules:      mapfreInclusionBaseRules(),
	})
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "mapfre_acc_men_fmt_inclusion_febrero_2026_vf",
		ProductID:  pidMapfreInclusionAPMenores,
		Name:       "Anexo 3 · cabezote 110 · inclusion VF ACC MEN",
		FilePrefix: prefixMapfreInclusionAccMenVF,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Priority:   130,
		Active:     true,
		Mappings: append(mapfreAccMenInclusionMappings(), model.FieldMap{
			CanonicalField: "office", SourceHeader: "OFICINA", Required: false,
		}),
		Rules: mapfreInclusionBaseRules(),
	})
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "mapfre_acc_men_fmt_contract",
		ProductID:  pidMapfreInclusionAPMenores,
		Name:       "Anexo 3 · cabezote 110 · contrato 5024524900101",
		FilePrefix: contractMapfreAccMen,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Priority:   150,
		Active:     true,
		Mappings:   mapfreAccMenInclusionMappings(),
		Rules:      mapfreInclusionBaseRules(),
	})

	st.UpsertProduct(model.Product{
		ID:         pidMapfreInclusionAPCancer,
		Code:       codeMapfreInclusionAPCancer,
		Insurer:    "MAPFRE",
		FilePrefix: prefixMapfreInclusionCancer,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Mappings:   mapfreCancerInclusionMappings(),
		Rules:      mapfreInclusionBaseRules(),
	})
	_ = st.UpsertAllowedPremiums(pidMapfreInclusionAPCancer, mapfreCancerAllowedPremiums)
	_ = st.UpsertProductRuleParam(pidMapfreInclusionAPCancer, "age_min", "18")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionAPCancer, "age_max", "65.997")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionAPCancer, "mapfre_require_current_month", "1")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionAPCancer, "mapfre_date_tolerance_days", "2")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionAPCancer, "age_max_days_before_birthday", "1")
	_ = st.UpsertProductRuleParam(pidMapfreInclusionAPCancer, "required_valid_date_fields_csv", "birth_date,activation_date,coverage_start_date,coverage_end_date")
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "mapfre_cancer_fmt_default",
		ProductID:  pidMapfreInclusionAPCancer,
		Name:       "Anexo 2 · cabezote 114 · default",
		FilePrefix: prefixMapfreInclusionCancer,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Priority:   100,
		Active:     true,
		Mappings:   mapfreCancerInclusionMappings(),
		Rules:      mapfreInclusionBaseRules(),
	})
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "mapfre_cancer_fmt_inclusion_abril",
		ProductID:  pidMapfreInclusionAPCancer,
		Name:       "Anexo 2 · cabezote 114 · inclusion CANCER RM abril",
		FilePrefix: prefixMapfreInclusionCancerVF,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Priority:   130,
		Active:     true,
		Mappings:   mapfreCancerInclusionMappings(),
		Rules:      mapfreInclusionBaseRules(),
	})
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "mapfre_cancer_fmt_contract",
		ProductID:  pidMapfreInclusionAPCancer,
		Name:       "Anexo 2 · cabezote 114 · contrato 5024524900103",
		FilePrefix: contractMapfreCancer,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Priority:   150,
		Active:     true,
		Mappings:   mapfreCancerInclusionMappings(),
		Rules:      mapfreInclusionBaseRules(),
	})

	st.UpsertProduct(model.Product{
		ID:         pidMapfreStockCartera,
		Code:       codeMapfreStockCartera,
		Insurer:    "MAPFRE",
		FilePrefix: prefixMapfreStock,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Mappings: append([]model.FieldMap{
			{CanonicalField: "document_number", SourceHeader: "IDENTIFICACIONAFILIADO", Required: true},
			{CanonicalField: "birth_date", SourceHeader: "FECHANACIMIENTO", Required: true},
			{CanonicalField: "monthly_premium", SourceHeader: "PRIMAMENSUALPERIODO", Required: true},
			{CanonicalField: "credit_number", SourceHeader: "NUMEROPRESTAMO", Required: true},
			{CanonicalField: "activation_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: true},
			{CanonicalField: "coverage_start_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: true},
			{CanonicalField: "coverage_end_date", SourceHeader: "FECHAFINVIGENCIADERIESGO REAL", Required: true},
			{CanonicalField: "plan_name", SourceHeader: "NOMBREPLAN", Required: false},
			{CanonicalField: "plan_code", SourceHeader: "CODIGO SEGURO", Required: false},
			{CanonicalField: "insured_amount", SourceHeader: "VALOR ASEGURADO", Required: false},
			{CanonicalField: "office", SourceHeader: "OFICINA", Required: false},
		}, optionalPersonMappings()...),
		Rules: []model.RuleConfig{
			{Type: "required_not_empty", Field: "document_number"},
			{Type: "required_not_empty", Field: "birth_date"},
			{Type: "required_not_empty", Field: "credit_number"},
			{Type: "number_gte", Field: "monthly_premium", Params: map[string]float64{"min": 0}},
		},
	})
	_ = st.UpsertAllowedPremiums(pidMapfreStockCartera, mapfreVidaStockAllowedPremiums)
	_ = st.UpsertProductRuleParam(pidMapfreStockCartera, "age_min", "18")
	_ = st.UpsertProductRuleParam(pidMapfreStockCartera, "age_max", "75.997")
	_ = st.UpsertProductRuleParam(pidMapfreStockCartera, "mapfre_require_current_month", "1")
	_ = st.UpsertProductRuleParam(pidMapfreStockCartera, "mapfre_date_tolerance_days", "2")
	_ = st.UpsertProductRuleParam(pidMapfreStockCartera, "age_max_days_before_birthday", "1")
	_ = st.UpsertProductRuleParam(pidMapfreStockCartera, "required_valid_date_fields_csv", "birth_date,activation_date,coverage_start_date,coverage_end_date")
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "mapfre_stock_fmt_default",
		ProductID:  pidMapfreStockCartera,
		Name:       "default",
		FilePrefix: prefixMapfreStock,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Priority:   100,
		Active:     true,
		Mappings: append([]model.FieldMap{
			{CanonicalField: "document_number", SourceHeader: "IDENTIFICACIONAFILIADO", Required: true},
			{CanonicalField: "birth_date", SourceHeader: "FECHANACIMIENTO", Required: true},
			{CanonicalField: "monthly_premium", SourceHeader: "PRIMAMENSUALPERIODO", Required: true},
			{CanonicalField: "credit_number", SourceHeader: "NUMEROPRESTAMO", Required: true},
			{CanonicalField: "activation_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: true},
			{CanonicalField: "coverage_start_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: true},
			{CanonicalField: "coverage_end_date", SourceHeader: "FECHAFINVIGENCIADERIESGO REAL", Required: true},
			{CanonicalField: "plan_name", SourceHeader: "NOMBREPLAN", Required: false},
			{CanonicalField: "plan_code", SourceHeader: "CODIGO SEGURO", Required: false},
			{CanonicalField: "insured_amount", SourceHeader: "VALOR ASEGURADO", Required: false},
			{CanonicalField: "office", SourceHeader: "OFICINA", Required: false},
		}, optionalPersonMappings()...),
		Rules: []model.RuleConfig{
			{Type: "required_not_empty", Field: "document_number"},
			{Type: "required_not_empty", Field: "birth_date"},
			{Type: "required_not_empty", Field: "credit_number"},
			{Type: "number_gte", Field: "monthly_premium", Params: map[string]float64{"min": 0}},
		},
	})
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "mapfre_stock_fmt_vf2_reporte",
		ProductID:  pidMapfreStockCartera,
		Name:       "stock vf2 hoja Reporte",
		FilePrefix: prefixMapfreStock,
		SheetName:  "Reporte",
		HeaderRow:  1,
		Priority:   120,
		Active:     true,
		Mappings: append([]model.FieldMap{
			{CanonicalField: "document_number", SourceHeader: "COD DOCUM", Required: true},
			{CanonicalField: "birth_date", SourceHeader: "FECHA NAC", Required: true},
			{CanonicalField: "monthly_premium", SourceHeader: "PRIMA MENSUAL", Required: true},
			{CanonicalField: "credit_number", SourceHeader: "NUMERO DE CREDITO", Required: true},
			{CanonicalField: "activation_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: true},
			{CanonicalField: "coverage_start_date", SourceHeader: "FECHA INICIO DE VIGENCIA", Required: true},
			{CanonicalField: "coverage_end_date", SourceHeader: "FECHAFINVIGENCIADERIESGO REAL CORRECTA FINAL", Required: true},
			{CanonicalField: "plan_name", SourceHeader: "PLAN", Required: false},
			{CanonicalField: "plan_code", SourceHeader: "CODIGO SEGURO", Required: false},
			{CanonicalField: "insured_amount", SourceHeader: "VALOR ASEGURADO", Required: false},
			{CanonicalField: "office", SourceHeader: "OFICINA", Required: false},
		}, optionalPersonMappings()...),
		Rules: mapfreInclusionBaseRules(),
	})

	st.UpsertProduct(model.Product{
		ID:         pidBolivarDeudoresStockEsal,
		Code:       codeBolivarDeudoresStockEsal,
		Insurer:    "BOLIVAR",
		FilePrefix: prefixBolivarStock,
		HeaderRow:  1,
		Mappings: append([]model.FieldMap{
			{CanonicalField: "document_number", SourceHeader: "IDENTIFICACION", Required: true},
			{CanonicalField: "birth_date", SourceHeader: "FECHA DE NACIMIENTO", Required: true},
			{CanonicalField: "credit_number", SourceHeader: "OP BT", Required: true},
			{CanonicalField: "initial_debt_amount", SourceHeader: "DEUDA INICIAL", Required: true},
			{CanonicalField: "monthly_premium", SourceHeader: "PRIMA MENSUAL", Required: true},
			{CanonicalField: "rate_percent", SourceHeader: "%", Required: true},
			{CanonicalField: "activation_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
			{CanonicalField: "loan_award_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
			{CanonicalField: "loan_due_date_current", SourceHeader: "FECHA VENCIMIENTO ACTUAL", Required: true},
			{CanonicalField: "observacion", SourceHeader: "OBSERVACION", Required: false},
		}, optionalPersonMappings()...),
		Rules: bolivarBancoRules(),
	})
	seedBolivarRuleParams(st, pidBolivarDeudoresStockEsal)
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "bolivar_stock_fmt_default",
		ProductID:  pidBolivarDeudoresStockEsal,
		Name:       "stock Bolívar genérico (STOCK-)",
		FilePrefix: prefixBolivarStock,
		HeaderRow:  1,
		Priority:   100,
		Active:     true,
		Mappings: append([]model.FieldMap{
			{CanonicalField: "document_number", SourceHeader: "IDENTIFICACION", Required: true},
			{CanonicalField: "birth_date", SourceHeader: "FECHA DE NACIMIENTO", Required: true},
			{CanonicalField: "credit_number", SourceHeader: "OP BT", Required: true},
			{CanonicalField: "initial_debt_amount", SourceHeader: "DEUDA INICIAL", Required: true},
			{CanonicalField: "monthly_premium", SourceHeader: "PRIMA MENSUAL", Required: true},
			{CanonicalField: "rate_percent", SourceHeader: "%", Required: true},
			{CanonicalField: "activation_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
			{CanonicalField: "loan_award_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
			{CanonicalField: "loan_due_date_current", SourceHeader: "FECHA VENCIMIENTO ACTUAL", Required: true},
			{CanonicalField: "observacion", SourceHeader: "OBSERVACION", Required: false},
		}, optionalPersonMappings()...),
		Rules: bolivarBancoRules(),
	})
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "bolivar_stock_fmt_febre_2026_xls",
		ProductID:  pidBolivarDeudoresStockEsal,
		Name:       "stock febre bolivar 2026",
		FilePrefix: prefixBolivarStockFebrero,
		SheetName:  "FACTURACION FEBRERO 2026",
		HeaderRow:  1,
		Priority:   130,
		Active:     true,
		Mappings: append([]model.FieldMap{
			{CanonicalField: "document_number", SourceHeader: "IDENTIFICACION", Required: true},
			{CanonicalField: "birth_date", SourceHeader: "FECHA DE NACIMIENTO", Required: true},
			{CanonicalField: "credit_number", SourceHeader: "OP BT", Required: true},
			{CanonicalField: "initial_debt_amount", SourceHeader: "DEUDA INICIAL", Required: true},
			{CanonicalField: "monthly_premium", SourceHeader: "PRIMA MENSUAL", Required: true},
			{CanonicalField: "rate_percent", SourceHeader: "%", Required: true},
			{CanonicalField: "activation_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
			{CanonicalField: "loan_award_date", SourceHeader: "FECHA ADJUDICACION", Required: true},
			{CanonicalField: "loan_due_date_current", SourceHeader: "FECHA VENCIMIENTO ACTUAL", Required: true},
			{CanonicalField: "plan_code", SourceHeader: "CODIGO SEGURO", Required: false},
			{CanonicalField: "insured_amount", SourceHeader: "VALOR ASEGURADO", Required: false},
			{CanonicalField: "office", SourceHeader: "OFICINA", Required: false},
			{CanonicalField: "observacion", SourceHeader: "OBSERVACION", Required: false},
		}, optionalPersonMappings()...),
		Rules: bolivarBancoRules(),
	})
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "bolivar_deudores_fmt_esal_micro",
		ProductID:  pidBolivarDeudoresStockEsal,
		Name:       "Anexo 5 · deudores ESAL micro",
		FilePrefix: prefixBolivarEsalMicro,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Priority:   200,
		Active:     true,
		Mappings:   bolivarEsalMappings(),
		Rules:      bolivarBancoRules(),
	})
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "bolivar_deudores_fmt_esal_pyme",
		ProductID:  pidBolivarDeudoresStockEsal,
		Name:       "Anexo 5 · deudores ESAL pyme",
		FilePrefix: prefixBolivarEsalPyme,
		SheetName:  "Hoja1",
		HeaderRow:  1,
		Priority:   190,
		Active:     true,
		Mappings:   bolivarEsalMappings(),
		Rules:      bolivarBancoRules(),
	})

	// MAPFRE anulaciones masivas (hoja Plantilla; reglas i–iii en mapfre_cancel.go).
	st.UpsertProduct(model.Product{
		ID:         pidMapfreAnulacionMasiva,
		Code:       codeMapfreAnulacionMasiva,
		Insurer:    "MAPFRE",
		FilePrefix: prefixMapfreAnulacionMasiva,
		SheetName:  "Plantilla_Anulaciones Abril26",
		HeaderRow:  1,
		Mappings: []model.FieldMap{
			{CanonicalField: "group_policy_number", SourceHeader: "NUMER PÓLIZA GRUPO", Required: false},
			{CanonicalField: "policy_number", SourceHeader: "# PÓLIZA ACTUAL", Required: false},
			{CanonicalField: "document_number", SourceHeader: "CEDULA CLIENTE", Required: true},
			{CanonicalField: "credit_number", SourceHeader: "OPERACIÓN BANTOTAL", Required: true},
			{CanonicalField: "cancellation_date", SourceHeader: "FEC_ANULA", Required: false},
			{CanonicalField: "activation_date", SourceHeader: "FECHA DE ACTIVACIÓN", Required: true},
			{CanonicalField: "coverage_end_date", SourceHeader: "FECHA PROYECTADA FIN DE VIGENCIA", Required: true},
			{CanonicalField: "observacion", SourceHeader: "OBS", Required: false},
		},
		Rules: []model.RuleConfig{
			{Type: "required_not_empty", Field: "document_number"},
			{Type: "required_not_empty", Field: "credit_number"},
			{Type: "required_not_empty", Field: "activation_date"},
			{Type: "required_not_empty", Field: "coverage_end_date"},
		},
	})

	mapfreAnulacionMappings := []model.FieldMap{
		{CanonicalField: "group_policy_number", SourceHeader: "NUMER PÓLIZA GRUPO", Required: false},
		{CanonicalField: "policy_number", SourceHeader: "# PÓLIZA ACTUAL", Required: false},
		{CanonicalField: "document_number", SourceHeader: "CEDULA CLIENTE", Required: true},
		{CanonicalField: "credit_number", SourceHeader: "OPERACIÓN BANTOTAL", Required: true},
		{CanonicalField: "cancellation_date", SourceHeader: "FEC_ANULA", Required: false},
		{CanonicalField: "activation_date", SourceHeader: "FECHA DE ACTIVACIÓN", Required: true},
		{CanonicalField: "coverage_end_date", SourceHeader: "FECHA PROYECTADA FIN DE VIGENCIA", Required: true},
		{CanonicalField: "observacion", SourceHeader: "OBS", Required: false},
	}
	mapfreAnulacionRules := []model.RuleConfig{
		{Type: "required_not_empty", Field: "document_number"},
		{Type: "required_not_empty", Field: "credit_number"},
		{Type: "required_not_empty", Field: "activation_date"},
		{Type: "required_not_empty", Field: "coverage_end_date"},
	}

	st.UpsertProductFormat(model.ProductFormat{
		ID:         "mapfre_anulacion_fmt_abril",
		ProductID:  pidMapfreAnulacionMasiva,
		Name:       "cancelaciones MAPFRE · plantilla anulaciones abril",
		FilePrefix: prefixMapfreAnulacionMasiva,
		SheetName:  "Plantilla_Anulaciones Abril26",
		HeaderRow:  1,
		Priority:   100,
		Active:     true,
		Mappings:   mapfreAnulacionMappings,
		Rules:      mapfreAnulacionRules,
	})

	// Variante con tilde: mismos mappings/rules, el selector de hoja resuelve la plantilla real.
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "mapfre_anulacion_fmt_abril_tilde",
		ProductID:  pidMapfreAnulacionMasiva,
		Name:       "cancelaciones MAPFRE · plantilla anulaciones (con tilde)",
		FilePrefix: prefixMapfreAnulacionMasivaTilde,
		SheetName:  "Plantilla_Anulaciones Abril26",
		HeaderRow:  1,
		Priority:   100,
		Active:     true,
		Mappings:   mapfreAnulacionMappings,
		Rules:      mapfreAnulacionRules,
	})

	st.UpsertProduct(model.Product{
		ID:         pidBolivarInclusionDeudoresBanco,
		Code:       codeBolivarInclusionDeudoresBanco,
		Insurer:    "BOLIVAR",
		FilePrefix: prefixBolivarBancoMicro,
		HeaderRow:  1,
		Mappings:   bolivarBancoMappings(),
		Rules:      bolivarBancoRules(),
	})
	seedBolivarRuleParams(st, pidBolivarInclusionDeudoresBanco)
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "bolivar_banco_fmt_micro",
		ProductID:  pidBolivarInclusionDeudoresBanco,
		Name:       "Anexo 4 · deudores banco micro (MICRO_BANCO)",
		FilePrefix: prefixBolivarBancoMicro,
		HeaderRow:  1,
		Priority:   200,
		Active:     true,
		Mappings:   bolivarBancoMappings(),
		Rules:      bolivarBancoRules(),
	})
	st.UpsertProductFormat(model.ProductFormat{
		ID:         "bolivar_banco_fmt_pyme",
		ProductID:  pidBolivarInclusionDeudoresBanco,
		Name:       "Anexo 4 · deudores banco pyme (Pyme_BANCO)",
		FilePrefix: prefixBolivarBancoPyme,
		HeaderRow:  1,
		Priority:   190,
		Active:     true,
		Mappings:   bolivarBancoMappings(),
		Rules:      bolivarBancoRules(),
	})
}
