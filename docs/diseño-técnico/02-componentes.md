# Componentes Técnicos Detallados

## 1. API Go HTTP

### 1.1 Estructura de Handlers

**Ubicación:** `services/api/main.go`

Handlers se definen inline en `main()` con `http.HandleFunc()`. No hay router externo (stdlib `http.ServeMux`).

#### **Health & Bootstrap**

```go
mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
  writeJSON(w, http.StatusOK, map[string]string{
    "status": "ok",
    "time": time.Now().UTC().Format(time.RFC3339),
  })
})

mux.HandleFunc("/api/v1/bootstrap/sample-products", func(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodPost {
    w.WriteHeader(http.StatusMethodNotAllowed)
    return
  }
  seed(st)  // Load hardcoded products + formats + rule params
  writeJSON(w, http.StatusCreated, map[string]string{"status": "seeded"})
})
```

#### **Catálogo de Productos**

```go
// POST /api/v1/products   - Crear/actualizar producto
// GET  /api/v1/products   - Listar todos

mux.HandleFunc("/api/v1/products", func(w http.ResponseWriter, r *http.Request) {
  switch r.Method {
  case http.MethodPost:
    var p model.Product
    json.NewDecoder(r.Body).Decode(&p)
    if p.ID == "" || p.Code == "" {
      writeJSON(w, http.StatusBadRequest, map[string]string{"error": "..."})
      return
    }
    // Validar mappings requeridos si existen
    if len(p.Mappings) > 0 {
      if missing := validateProductConfig(p); len(missing) > 0 {
        writeJSON(w, http.StatusBadRequest, map[string]any{
          "error":          "configuración incompleta",
          "missing_fields": missing,
        })
        return
      }
    }
    writeJSON(w, http.StatusCreated, st.UpsertProduct(p))
  case http.MethodGet:
    writeJSON(w, http.StatusOK, st.ListProducts())
  }
})
```

#### **Formatos (Nuevo Modelo)**

```go
// GET  /api/v1/product-formats?product_id=X     - Listar formatos
// POST /api/v1/product-formats                   - Crear/actualizar
// PATCH /api/v1/product-formats/active          - Toggle active
// POST /api/v1/product-formats/match-test        - Test matching

mux.HandleFunc("/api/v1/product-formats", func(w http.ResponseWriter, r *http.Request) {
  switch r.Method {
  case http.MethodGet:
    productID := strings.TrimSpace(r.URL.Query().Get("product_id"))
    writeJSON(w, http.StatusOK, st.ListProductFormats(productID))
  case http.MethodPost:
    var req productFormatsUpsertRequest
    json.NewDecoder(r.Body).Decode(&req)
    // Validar campos requeridos
    if req.ID == "" || req.ProductID == "" || req.FilePrefix == "" {
      writeJSON(w, http.StatusBadRequest, map[string]string{
        "error": "id, product_id, file_prefix son obligatorios",
      })
      return
    }
    format := model.ProductFormat{
      ID:         req.ID,
      ProductID:  req.ProductID,
      Name:       req.Name,
      FilePrefix: req.FilePrefix,
      SheetName:  req.SheetName,
      HeaderRow:  req.HeaderRow,
      Priority:   req.Priority,
      Active:     req.Active != nil ? *req.Active : true,
      Mappings:   req.Mappings,
      Rules:      req.Rules,
    }
    writeJSON(w, http.StatusCreated, st.UpsertProductFormat(format))
  }
})
```

#### **Primas Permitidas (Tunable)**

```go
// GET    /api/v1/products/allowed-premiums?product_id=X
// PUT    /api/v1/products/allowed-premiums                (reemplazar lista)
// POST   /api/v1/products/allowed-premiums                (agregar una)
// DELETE /api/v1/products/allowed-premiums?product_id=X&premium=N

// Ejemplo GET:
mux.HandleFunc("/api/v1/products/allowed-premiums", func(w http.ResponseWriter, r *http.Request) {
  if r.Method == http.MethodGet {
    productID := r.URL.Query().Get("product_id")
    items := st.GetAllowedPremiums(productID)
    writeJSON(w, http.StatusOK, map[string]any{
      "product_id": productID,
      "count": len(items),
      "premiums": items,
    })
  }
  // ... POST, PUT, DELETE cases
})
```

#### **Procesamiento**

```go
// POST /api/v1/process/scan - Escanear SFTP y encolar archivos
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
    "status": "queued",
    "enqueued": enqueued,
    "message": "archivos escaneados y encolados para procesamiento asíncrono",
  })
})

// GET /api/v1/process/progress - Monitoreo en tiempo real
mux.HandleFunc("/api/v1/process/progress", func(w http.ResponseWriter, r *http.Request) {
  writeJSON(w, http.StatusOK, map[string]any{
    "items": proc.SnapshotProgress(),  // map[fileID]ProgressInfo
  })
})
```

#### **Archivos**

```go
// GET  /api/v1/files                           - Listar registros
// POST /api/v1/files/retry?file_id=X           - Reintentar
// GET  /api/v1/files/summary?file_id=X         - Resumen calidad
// GET  /api/v1/files/validation-report?file_id=X  - Informe JSON
// GET  /api/v1/files/validation-csv?file_id=X     - CSV descargable
// GET  /api/v1/files/validation-xlsx?file_id=X    - XLSX descargable
// GET  /api/v1/files/download?file_id=X           - Archivo original

// Ejemplo: validation-xlsx prefiere servidor en disco, fallback a regenerar
mux.HandleFunc("/api/v1/files/validation-xlsx", func(w http.ResponseWriter, r *http.Request) {
  fileID := r.URL.Query().Get("file_id")
  // Intentar servir XLSX archivado en disco
  if rec, ok := st.GetFileRecordByID(fileID); ok && rec.ReportArchivePath != "" {
    if _, err := os.Stat(rec.ReportArchivePath); err == nil {
      w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
      w.Header().Set("Content-Disposition", `attachment; filename="novedades_...xlsx"`)
      http.ServeFile(w, r, rec.ReportArchivePath)
      return
    }
  }
  // Fallback: regenerar desde BD (para archivos pequeños)
  report, _ := st.GetFileValidationReport(fileID)
  content, _ := store.ValidationReportClientXLSX(report)
  w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
  w.Write(content)
})
```

#### **Pólizas**

```go
// GET /api/v1/policies?product_id=X&limit=50
// GET /api/v1/policies/search?product_id=X&credit_number=Y&page=1&page_size=50

mux.HandleFunc("/api/v1/policies/search", func(w http.ResponseWriter, r *http.Request) {
  credit := r.URL.Query().Get("credit_number")
  doc := r.URL.Query().Get("document_number")
  productID := r.URL.Query().Get("product_id")
  
  if productID == "" && credit == "" && doc == "" {
    writeJSON(w, http.StatusBadRequest, map[string]string{
      "error": "indique product_id o al menos credit_number/document_number",
    })
    return
  }
  
  page, pageSize := 1, 50
  // Parse page, page_size, maxPageSize=200
  
  items, total := st.SearchPoliciesPage(productID, credit, doc, page, pageSize)
  totalPages := (total + pageSize - 1) / pageSize
  
  writeJSON(w, http.StatusOK, map[string]any{
    "product_id": productID,
    "page": page,
    "page_size": pageSize,
    "total": total,
    "total_pages": totalPages,
    "count": len(items),
    "items": toPolicyResponseItems(items, false),  // include_raw flag
  })
})
```

### 1.2 Response Shape

Todas las respuestas usan `writeJSON()`:

```go
func writeJSON(w http.ResponseWriter, status int, v any) {
  w.Header().Set("Content-Type", "application/json")
  w.WriteHeader(status)
  json.NewEncoder(w).Encode(v)
}
```

Respuesta de póliza (compactada, sin campos vacíos):

```json
{
  "id": "file_xyz:15",
  "file_id": "file_xyz",
  "product_id": "mapfre_inclusion_vida_voluntario",
  "file_name": "INCLUSION-VIDA-MAPFRE.xlsx",
  "row_number": 15,
  "status": "ACTIVE",
  "document_number": "123456789",
  "credit_number": "98765",
  "customer_data": {
    "id_number": "123456789",
    "full_name": "Juan Pérez",
    "birth_date": "1980-05-15",
    "email": "juan@example.com"
  },
  "financial_data": {
    "premium_value": 8600,
    "currency": "COP"
  },
  "validation_data": {
    "is_valid": true,
    "alerts": [],
    "requires_manual_action": false
  },
  "created_at": "2025-08-04T12:34:56.789Z"
}
```

La función `compactMap()` elimina claves con valores vacíos o nulos.

---

## 2. Processor Service (Worker Pool)

### 2.1 Inicialización

```go
// services/api/internal/processor/processor.go

type Service struct {
  store      *store.Store
  notifier   notify.FileNotifier
  jobs       chan queuedJob          // buffered channel, 256 capacity
  once       sync.Once               // ensure workers start once
  workers    int                     // PROCESSOR_WORKERS env var
  progressMu sync.RWMutex
  progress   map[string]ProgressInfo // fileID → live status
}

func New(st *store.Store) *Service {
  workers := processorWorkersFromEnv()  // default 2
  s := &Service{
    store:    st,
    notifier: notify.NewFileNotifierFromEnv(),
    jobs:     make(chan queuedJob, 256),
    workers:  workers,
    progress: make(map[string]ProgressInfo),
  }
  s.startWorker()  // spin up workers
  return s
}

func (s *Service) startWorker() {
  s.once.Do(func() {
    for i := 0; i < s.workers; i++ {
      go func(id int) {
        for job := range s.jobs {
          s.runJob(id, job)
        }
      }(i + 1)
    }
  })
}
```

### 2.2 Job Processing Loop

```go
func (s *Service) runJob(workerID int, job queuedJob) {
  // Garantía: todo job termina en estado terminal
  terminalWritten := false
  defer func() {
    r := recover()
    if r == nil { return }
    
    // Panic capturado: pasar a ERROR si aún no se escribió estado terminal
    if !terminalWritten {
      stack := debug.Stack()
      log.Printf("[processor] worker_%d PANIC file_id=%s\n%s", workerID, job.ID, stack)
      s.store.AddFileRecord(model.FileProcessRecord{
        ID:          job.ID,
        Status:      model.FileStatusError,
        ErrorReason: fmt.Sprintf("panic: %v", r),
      })
    }
  }()
  
  s.updateProgress(job.ID, job.FileName, "PROCESSING", 0, "iniciando", "", "")
  s.store.AddFileRecord(model.FileProcessRecord{
    ID:       job.ID,
    FileName: job.FileName,
    Status:   model.FileStatusProcessing,
  })
  
  rec := s.processByName(job)
  s.store.AddFileRecord(rec)
  
  // Red de seguridad: UPDATE mínimo si AddFileRecord falló silenciosamente
  if err := s.store.FinalizeFileStatus(rec); err != nil {
    log.Printf("[processor] FinalizeFileStatus falló file_id=%s err=%v", job.ID, err)
  }
  terminalWritten = true
  
  // Notificar sólo si status es terminal
  s.notifyFileProcessing(rec)
  
  s.updateProgress(job.ID, job.FileName, string(rec.Status), 100, "finalizado", rec.ProductID, "")
}
```

### 2.3 Validación de Filas

**Ubicación:** `services/api/internal/processor/processor.go` línea ~822

```go
// Por cada fila:
for i := p.HeaderRow; i < len(rows); i++ {
  row := rows[i]
  if rowEmpty(row) { continue }
  
  // 1. Mapeo de valores canónicos
  values := make(map[string]string)
  for col, h := range header {
    if col >= len(row) { continue }
    key := strings.TrimSpace(h)
    val := strings.TrimSpace(row[col])
    if _, exists := values[key]; !exists {
      values[key] = val  // Usa encabezado original como clave
    }
  }
  for field, col := range fieldToCol {
    if col < len(row) {
      values[field] = strings.TrimSpace(row[col])  // Sobreescribe con canónico
    }
  }
  
  // 2. Reglas declaradas en formato
  frozen, ruleViolations := runRules(values, p.Rules)
  
  // 3. Reglas de negocio (aseguradora específica)
  diagramHards, rowNotes := applyDiagramRules(
    p.Code, values, seenCredits, inFileCreditCounts, 
    inFileCreditRows, ruleCfg, svc,
  )
  
  // 4. Determinar estado
  status := "ACTIVE"
  notes := []string{}
  for _, rv := range ruleViolations {
    notes = append(notes, noteIncidencia(rv))
  }
  if len(ruleViolations) > 0 {
    if !readFullFileOnRowErrors {
      return nil, fileHash, archivePath, p.ID, 
        fmt.Errorf("fila %d: %s", i+1, strings.Join(ruleViolations, "; "))
    }
    status = "MANUAL_REVIEW"
  }
  
  // Si prima es 0 y producto congela:
  if frozen {
    status = "FROZEN"
    notes = append(notes, noteInformativo("Póliza congelada por prima = 0"))
  }
  
  // Si prima es 0 y SIN política de congelamiento:
  if !hasFreezeOnZeroPolicy && premiun == 0 {
    status = "CANCELLED"
  }
  
  // Acumular diagram hards
  for _, msg := range diagramHards {
    notes = append(notes, noteIncidencia(msg))
  }
  if len(diagramHards) > 0 && status == "ACTIVE" {
    status = "MANUAL_REVIEW"
  }
  
  // Si es anulación MAPFRE:
  if isMapfreCancelacionProduct(p.Code) && status == "ACTIVE" {
    status = "CANCELLED"
  }
  
  // Crear registro de póliza
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
```

### 2.4 File-Level Gate (CRÍTICO)

```go
// Si alguna fila tiene blocking issues → NO se persisten pólizas
if policiesRowSetHasBlockingIssues(policies) {
  nIssue := countPoliciesBlockingIssues(policies)
  summary := fmt.Sprintf(
    "carga omitida: %d filas con incidencias; ninguna póliza persistida",
    nIssue,
  )
  report := store.BuildFileValidationReportFromPolicies(
    rec.ID, fileName, selectedProductID, 
    string(model.FileStatusError), summary, 
    rec.ProcessedAt.UTC().Format(time.RFC3339Nano), policies,
  )
  rec.ValidationReportJSON = string(b)
  rec.ReportArchivePath = saveValidationReportArchive(report, rec.ID, fileName)
  rec.Status = model.FileStatusError
  rec.ErrorReason = summary
  rec.ProcessedPath = moveRemoteFile(src, fileName, "ERROR")
  log.Printf("[processor] solo_informe file_id=%s filas_con_incidencias=%d", rec.ID, nIssue)
  return rec  // Exit aquí, NO insert
}

// De lo contrario, persistir
if err := s.store.InsertPolicies(policies); err != nil {
  rec.Status = model.FileStatusError
  rec.ErrorReason = "no se pudieron registrar pólizas: " + err.Error()
  return rec
}

// Stock: cancelar faltantes
if isStockProduct(...) {
  cancelled, err := s.store.CancelMissingStockPolicies(productID, rec.ID, currentCredits)
  // ...
}

// MAPFRE anulación masiva: marcar stock
if isMapfreCancelacionProduct(...) {
  applied, err := s.applyMapfreCancellationsToStock(policies)
  // ...
}
```

---

## 3. Store (MySQL + Queries)

### 3.1 Schema (Migraciones Automáticas)

```go
// services/api/internal/store/store.go

func (s *Store) runMigrations() error {
  migrations := map[string]string{
    "20250101_001_schema": `
      CREATE TABLE IF NOT EXISTS products (
        id VARCHAR(255) PRIMARY KEY,
        code VARCHAR(255),
        insurer VARCHAR(100),
        file_prefix VARCHAR(255),
        sheet_name VARCHAR(255),
        header_row INT DEFAULT 1,
        mappings_json LONGTEXT,
        rules_json LONGTEXT,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
      );
      
      CREATE TABLE IF NOT EXISTS product_formats (
        id VARCHAR(255) PRIMARY KEY,
        product_id VARCHAR(255),
        format_name VARCHAR(255),
        file_prefix VARCHAR(255),
        sheet_name VARCHAR(255),
        header_row INT DEFAULT 1,
        priority INT DEFAULT 100,
        active TINYINT DEFAULT 1,
        mappings_json LONGTEXT,
        rules_json LONGTEXT,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (product_id) REFERENCES products(id),
        INDEX idx_file_prefix (file_prefix)
      );
      
      CREATE TABLE IF NOT EXISTS policies (
        id INT AUTO_INCREMENT PRIMARY KEY,
        file_id VARCHAR(255),
        product_id VARCHAR(255),
        file_name VARCHAR(255),
        row_number INT,
        document_number VARCHAR(50),
        credit_number VARCHAR(50),
        policy_status VARCHAR(50) DEFAULT 'ACTIVE',
        raw_data_json LONGTEXT,
        validation_json LONGTEXT,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        INDEX idx_product_status (product_id, policy_status),
        INDEX idx_credit (credit_number),
        INDEX idx_doc (document_number),
        INDEX idx_file (file_id)
      );
      
      CREATE TABLE IF NOT EXISTS processed_files (
        id VARCHAR(255) PRIMARY KEY,
        file_name VARCHAR(255),
        product_id VARCHAR(255),
        file_hash VARCHAR(64),
        status VARCHAR(50) DEFAULT 'PENDING',
        error_reason LONGTEXT,
        email_error LONGTEXT,
        validation_report_json LONGTEXT,
        remote_path VARCHAR(500),
        processed_path VARCHAR(500),
        archive_path VARCHAR(500),
        report_archive_path VARCHAR(500),
        processed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        INDEX idx_status (status),
        INDEX idx_hash (file_hash)
      );
      
      CREATE TABLE IF NOT EXISTS product_allowed_premiums (
        id INT AUTO_INCREMENT PRIMARY KEY,
        product_id VARCHAR(255),
        premium_value DECIMAL(12, 2),
        active TINYINT DEFAULT 1,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        UNIQUE KEY uk_product_premium (product_id, premium_value)
      );
      
      CREATE TABLE IF NOT EXISTS product_rule_params (
        id INT AUTO_INCREMENT PRIMARY KEY,
        product_id VARCHAR(255),
        param_name VARCHAR(255),
        param_value VARCHAR(500),
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        UNIQUE KEY uk_product_param (product_id, param_name)
      );
      
      CREATE TABLE IF NOT EXISTS global_rule_params (
        id INT AUTO_INCREMENT PRIMARY KEY,
        param_name VARCHAR(255),
        param_value VARCHAR(500),
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        UNIQUE KEY uk_param (param_name)
      );
    `,
    // Más migrations con claves como "20250115_001_add_column_x"
  }
  
  for key, sql := range migrations {
    if alreadyRun := s.isMigrationRun(key); alreadyRun {
      continue
    }
    if _, err := s.db.Exec(sql); err != nil {
      return fmt.Errorf("migration %s: %w", key, err)
    }
    s.recordMigrationRun(key)
  }
  return nil
}
```

### 3.2 Key Operations

#### **InsertPolicies (Transacción)**

```go
func (s *Store) InsertPolicies(policies []model.PolicyRecord) error {
  if len(policies) == 0 { return nil }
  
  tx, err := s.db.Begin()
  if err != nil { return err }
  defer func() {
    if err != nil { tx.Rollback() }
  }()
  
  q := `INSERT INTO policies (
    file_id, product_id, file_name, row_number, document_number, 
    credit_number, policy_status, raw_data_json, validation_json, created_at
  ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
  
  for _, p := range policies {
    if _, err = tx.Exec(q,
      p.FileID, p.ProductID, p.FileName, p.RowNumber,
      p.DocumentNumber, p.CreditNumber, p.PolicyStatus,
      p.RawDataJSON, nullableString(p.ValidationJSON), p.CreatedAt,
    ); err != nil {
      return fmt.Errorf("insert policy row=%d: %w", p.RowNumber, err)
    }
  }
  return tx.Commit()
}
```

#### **FindProductFormatCandidates (Match Query)**

```go
func (s *Store) FindProductFormatCandidates(fileName string) []model.Product {
  name := strings.TrimSpace(fileName)
  if name == "" { return []model.Product{} }
  
  sqlFormats, argsFormats, _ := s.sb.
    Select(
      "p.id", "p.code", "p.insurer", "f.file_prefix", "f.sheet_name",
      "f.header_row", "f.mappings_json", "f.rules_json", "p.created_at",
    ).
    From("product_formats f").
    Join("products p ON p.id = f.product_id").
    Where(sq.Expr("f.active = 1")).
    Where(sq.Expr("UPPER(?) LIKE CONCAT('%', UPPER(f.file_prefix), '%')", name)).
    OrderBy("LENGTH(f.file_prefix) DESC", "f.priority DESC", "f.created_at DESC").
    ToSql()
  
  rows, err := s.db.Query(sqlFormats, argsFormats...)
  if err == nil {
    defer rows.Close()
    formats := make([]model.Product, 0)
    for rows.Next() {
      var p model.Product
      var mappingsJSON, rulesJSON string
      if err := rows.Scan(&p.ID, &p.Code, &p.Insurer, &p.FilePrefix, 
        &p.SheetName, &p.HeaderRow, &mappingsJSON, &rulesJSON, &p.CreatedAt); err != nil {
        continue
      }
      _ = json.Unmarshal([]byte(mappingsJSON), &p.Mappings)
      _ = json.Unmarshal([]byte(rulesJSON), &p.Rules)
      formats = append(formats, p)
    }
    if len(formats) > 0 { return formats }
  }
  return []model.Product{}
}
```

#### **SearchPoliciesPage (Paginación)**

```go
func (s *Store) SearchPoliciesPage(
  productID, creditNumber, documentNumber string,
  page, pageSize int,
) ([]model.PolicyRecord, int) {
  qb := s.sb.Select("...").From("policies")
  
  if productID != "" {
    qb = qb.Where(sq.Eq{"product_id": productID})
  }
  if creditNumber != "" {
    qb = qb.Where(sq.Expr("credit_number LIKE ?", "%"+creditNumber+"%"))
  }
  if documentNumber != "" {
    qb = qb.Where(sq.Expr("document_number LIKE ?", "%"+documentNumber+"%"))
  }
  
  // Total
  countSQL, countArgs, _ := qb.Columns("COUNT(*)").ToSql()
  var total int
  s.db.QueryRow(countSQL, countArgs...).Scan(&total)
  
  // Offset & limit
  offset := (page - 1) * pageSize
  qb = qb.Offset(uint64(offset)).Limit(uint64(pageSize))
  qb = qb.OrderBy("created_at DESC")
  
  sql, args, _ := qb.ToSql()
  rows, _ := s.db.Query(sql, args...)
  
  var items []model.PolicyRecord
  // Scan rows
  return items, total
}
```

### 3.3 Validación de Configuración

```go
func validateProductConfig(p model.Product) []string {
  mapped := map[string]bool{}
  for _, m := range p.Mappings {
    f := strings.ToLower(strings.TrimSpace(m.CanonicalField))
    if f != "" { mapped[f] = true }
  }
  
  required := []string{
    "document_number",
    "credit_number",
    "monthly_premium",
  }
  
  code := strings.ToUpper(strings.TrimSpace(p.Code))
  if strings.HasPrefix(code, "MAPFRE") {
    required = append(required,
      "birth_date", "activation_date",
      "coverage_start_date", "coverage_end_date",
    )
    if code == codeMapfreInclusionVidaVoluntario {
      required = append(required, "initial_term_months")
    }
  } else if strings.HasPrefix(code, "BOLIVAR") {
    required = append(required,
      "birth_date", "activation_date",
      "initial_debt_amount", "rate_percent",
      "loan_award_date", "loan_due_date_current",
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
```

---

## 4. Frontend Admin (React)

### 4.1 Estructura

**Ubicación:** `frontend-admin/src/`

Componente principal: `App.tsx` (~1.4k líneas)

```tsx
function App() {
  const [activeTab, setActiveTab] = useState<TabId>('operacion')
  const [apiBaseUrl, setApiBaseUrl] = useState('/api/v1')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [lastResponse, setLastResponse] = useState<JsonValue | null>(null)
  const [flash, setFlash] = useState('')
  
  // Estado por tab
  const [health, setHealth] = useState<JsonObject | null>(null)
  const [progress, setProgress] = useState<JsonObject | null>(null)
  const [products, setProducts] = useState<JsonObject[]>([])
  const [files, setFiles] = useState<JsonObject[]>([])
  const [policies, setPolicies] = useState<JsonObject[]>([])
  
  // ... más estado específico por tab
  
  return (
    <main className="adminLayout">
      <aside className="sidebar">
        {/* Tabs, estadísticas generales */}
      </aside>
      <section className="content">
        {/* Contenido dinámico por tab */}
      </section>
    </main>
  )
}
```

### 4.2 Tabs Principales

#### **1. Operación**

- Health check
- Seed de productos
- Trigger scan SFTP
- Monitoreo de progreso (tabla con auto-refresh 5s)
- Incidentes recientes (últimos 6 errores)
- Paso-a-paso checklist

```tsx
{activeTab === 'operacion' ? (
  <Card title="Operación E2E">
    <div className="kpis">
      <KpiCard value={`${completedSteps}/5`} label="pasos completados" />
      <KpiCard value={progressItems.length} label="items en progreso" />
    </div>
    <div className="checklist">
      {processChecklist.map((step) => (
        <div key={step.label} className={step.done ? 'done' : 'pending'}>
          <span>{step.done ? '●' : '○'}</span>
          <span>{step.label}</span>
        </div>
      ))}
    </div>
    <div className="row">
      <button onClick={() => void loadHealth()}>Health</button>
      <button onClick={() => void seedProducts()}>Seed</button>
      <button onClick={() => void triggerScan()}>Scan SFTP</button>
      <button onClick={() => void loadProgress()}>Progreso</button>
    </div>
    {progressItems.length > 0 ? (
      <table>
        <thead>
          <tr>
            <th>Archivo</th>
            <th>Paso</th>
            <th>%</th>
            <th>Estado</th>
            <th>Error</th>
          </tr>
        </thead>
        <tbody>
          {progressItems.map((item) => (
            <tr key={item.file_name}>
              <td>{item.file_name}</td>
              <td>{item.step}</td>
              <td>{item.percent}%</td>
              <td><StatusBadge status={item.status} /></td>
              <td>
                {item.last_error ? (
                  <button onClick={() => openErrorDetail([...])}>Ver error</button>
                ) : <span className="muted">-</span>}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    ) : null}
  </Card>
) : null}
```

#### **2. Productos**

- Listado de productos existentes
- CRUD via editor JSON
- Wizard guiado (5 pasos)
- Validación de campos requeridos

#### **3. Primas**

- Selector de producto
- Listar primas permitidas
- Agregar/eliminar/reemplazar primas
- Parseo de CSV para bulk operations

#### **4. Archivos**

- Filtro por estado y producto
- Resumen de calidad (ACTIVE/FROZEN/MANUAL_REVIEW/CANCELLED)
- Descarga de validación (CSV/XLSX)
- Retry de ERROR
- Ver informe de validación en drawer modal

#### **5. Pólizas**

- Búsqueda por documento/crédito/producto
- Paginación (page_size hasta 200)
- Toggle include_raw (expone raw_data_json)
- Listar por producto sin paginación

### 4.3 Llamadas API

```tsx
async function callApi(path: string, init?: RequestInit): Promise<JsonValue | null> {
  setLoading(true)
  setError('')
  try {
    const response = await fetch(`${apiBaseUrl}${path}`, {
      ...init,
      headers: {
        'Content-Type': 'application/json',
        ...(init?.headers ?? {}),
      },
    })
    
    const text = await response.text()
    const maybeJson = text ? JSON.parse(text) : { ok: true }
    
    if (!response.ok) {
      throw new Error(
        typeof maybeJson === 'object' && 'error' in maybeJson
          ? String(maybeJson.error)
          : `HTTP ${response.status}`,
      )
    }
    
    setLastResponse(maybeJson)
    return maybeJson
  } catch (err) {
    setError(err instanceof Error ? err.message : 'Error inesperado')
    return null
  } finally {
    setLoading(false)
  }
}

// Auto-refresh en Operación
useEffect(() => {
  if (!autoRefreshProgress || activeTab !== 'operacion') return
  const timer = setInterval(() => {
    void loadProgress()
    void loadFiles()
  }, 5000)
  return () => clearInterval(timer)
}, [autoRefreshProgress, activeTab])
```

---

## 5. SFTP & Almacenamiento

### 5.1 SFTP Client

**Ubicación:** `services/api/internal/sftp/client.go`

```go
type Client struct {
  c *sftp.Client
}

func (c *Client) ListRootFiles() ([]os.FileInfo, error) {
  return c.c.ReadDir(".")
}

func (c *Client) Open(fileName string) (io.ReadCloser, error) {
  return c.c.Open(fileName)
}

func (c *Client) MoveToFolder(fileName, folder string) (string, error) {
  dst := fmt.Sprintf("%s/%s", folder, fileName)
  err := c.c.Rename(fileName, dst)
  if err != nil {
    return "", err
  }
  return dst, nil
}

func (c *Client) Close() error {
  return c.c.Close()
}
```

### 5.2 Archivos Locales (Testing)

```go
type localFileSource struct {
  baseDir string
}

func (l localFileSource) Open(fileName string) (io.ReadCloser, error) {
  path := filepath.Join(l.baseDir, fileName)
  return os.Open(path)
}

func (l localFileSource) MoveToFolder(fileName, folder string) (string, error) {
  srcPath := filepath.Join(l.baseDir, fileName)
  dstFolder := filepath.Join(l.baseDir, folder)
  os.MkdirAll(dstFolder, 0755)
  
  dstPath := filepath.Join(dstFolder, fileName)
  err := os.Rename(srcPath, dstPath)
  if err != nil {
    return "", err
  }
  return dstPath, nil
}
```

---

## 6. Notificación (SendGrid)

**Ubicación:** `services/api/internal/notify/sendgrid.go`

```go
type FileNotifier interface {
  NotifyFileProcessing(input FileEmailInput) error
}

type SendGridNotifier struct {
  apiKey        string
  fromEmail     string
  errorToEmails []string
}

func (s *SendGridNotifier) NotifyFileProcessing(input FileEmailInput) error {
  if s.apiKey == "" {
    // Silent: si no hay config, no enviar
    log.Printf("[notify] sendgrid no configurado, notificación omitida file_id=%s", input.FileID)
    return nil
  }
  
  // Construir contenido del email
  subject := fmt.Sprintf("Busk: %s - %s", input.Status, input.FileName)
  
  // Body: resumen de archivo, counts, errores, novedades
  body := buildEmailBody(input)
  
  // Adjuntos: CSV y/o XLSX de validación
  attachments := make([]string, 0)
  if input.ReportArchivePath != "" {
    attachments = append(attachments, input.ReportArchivePath)
  }
  
  // Enviar vía SendGrid API
  m := mail.NewV3Mail()
  m.SetFrom(mail.NewEmail("Busk Seguros", s.fromEmail))
  m.Subject = subject
  
  for _, to := range s.errorToEmails {
    m.AddTo(mail.NewEmail("", to))
  }
  
  m.AddContent(mail.NewContent("text/html", body))
  
  for _, path := range attachments {
    data, _ := ioutil.ReadFile(path)
    m.AddAttachment(&mail.Attachment{
      Filename: filepath.Base(path),
      Type:     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
      Data:     base64.StdEncoding.EncodeToString(data),
    })
  }
  
  client := sendgrid.NewSendClient(s.apiKey)
  response, err := client.Send(m)
  
  if err != nil {
    return err
  }
  if response.StatusCode >= 400 {
    return fmt.Errorf("sendgrid http %d", response.StatusCode)
  }
  
  return nil
}
```

**Configuración:**
```bash
SENDGRID_API_KEY="SG.xxx..."
SENDGRID_FROM_EMAIL="noreply@busk.example.com"
SENDGRID_ERROR_TO_EMAILS="operator@busk.example.com,admin@busk.example.com"
```

Si no se setean, notificaciones son ignoradas silenciosamente (no rompe el flujo).

