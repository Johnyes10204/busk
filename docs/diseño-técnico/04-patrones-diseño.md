# Patrones de Diseño Empleados

## 1. Worker Pool Pattern (Fan-Out/Fan-In)

### Descripción

Orquestar múltiples workers concurrentes consumiendo jobs de un canal compartido.

### Implementación en Busk

```go
// Producer: ScanAndEnqueue()
for _, name := range candidates {
  job := queuedJob{
    ID:       fmt.Sprintf("file_%d", time.Now().UnixNano()),
    FileName: name,
  }
  s.updateProgress(job.ID, job.FileName, "QUEUED", 0, "encolado", "", "")
  s.store.AddFileRecord(model.FileProcessRecord{
    ID:       job.ID,
    FileName: job.FileName,
    Status:   model.FileStatusQueued,
  })
  
  select {
  case s.jobs <- job:  // Enqueue
    enqueued++
  default:  // Queue full: reject
    s.store.AddFileRecord(model.FileProcessRecord{
      ID:          job.ID,
      Status:      model.FileStatusError,
      ErrorReason: "cola de procesamiento llena",
    })
  }
}

// Consumers: startWorker() inicia N goroutines
for i := 0; i < s.workers; i++ {
  go func(id int) {
    for job := range s.jobs {  // Block until job available
      s.runJob(id, job)
    }
  }(i + 1)
}
```

### Problema Resuelto

- **Paralelismo:** Procesar múltiples archivos simultáneamente
- **Resource control:** Buffered channel limita memory (256 jobs max)
- **Backpressure:** Si queue llena, rechazar archivo vs. memory blow-up
- **Simplicidad:** Go channels son minimal, legible

### Patrones Relacionados

- **Fan-Out:** ScanAndEnqueue distribuye N jobs a canal
- **Fan-In:** N workers compiten por mismo canal
- **Semaphore:** Buffered channel actúa como semáforo (max 256 concurrentes)

### Límites y Tradeoffs

| Aspecto | Ventaja | Desventaja |
|---------|---------|-----------|
| **Configurabilidad** | PROCESSOR_WORKERS tunable | Requiere restart para cambiar |
| **Monitoring** | Job canal observable | Sin métricas de Prometheus built-in |
| **Escalado** | Cabe en 1 servidor (~100 workers) | No distribuido (vs. Kafka) |

---

## 2. Repository Pattern (Data Access Abstraction)

### Descripción

Encapsular todas las operaciones de base de datos detrás de una interfaz.

### Implementación en Busk

```go
type Store interface {
  // Catálogo
  ListProducts() []model.Product
  UpsertProduct(p model.Product) model.Product
  FindProductFormatCandidates(fileName string) []model.Product
  
  // Políticas
  InsertPolicies(policies []model.PolicyRecord) error
  ListPoliciesByProduct(productID, status string, limit int) []model.PolicyRecord
  SearchPoliciesPage(productID, credit, doc string, page, pageSize int) ([]model.PolicyRecord, int)
  CancelMissingStockPolicies(productID, fileID string, currentCredits []string) (int64, error)
  
  // Archivos
  AddFileRecord(r model.FileProcessRecord)
  ListFileRecords() []model.FileProcessRecord
  GetFileValidationReport(fileID string) (*store.FileValidationReport, error)
  FinalizeFileStatus(r model.FileProcessRecord) error
  MarkStaleFilesAsError() (int64, error)
  
  // ... más métodos
}

type Store struct {
  db *sql.DB
  sb sq.StatementBuilderType  // Squirrel query builder
}

// Uso en Processor
func (s *Service) processByName(job queuedJob) model.FileProcessRecord {
  products := s.store.FindProductFormatCandidates(job.FileName)
  // ...
  if err := s.store.InsertPolicies(policies); err != nil { ... }
}
```

### Problema Resuelto

- **Desacoplamiento:** Processor no conoce SQL, MySQL internals
- **Testabilidad:** Mock Store para unit tests sin DB
- **Maintainability:** Cambios de schema localizados
- **Flexibility:** Futuro cambio a PostgreSQL sin refactor Processor

### Variantes

#### **Interfaz vs. Struct Concreto**

**Busk usa:** Struct concreto `Store` + métodos receivers.

```go
func (s *Store) InsertPolicies(policies []model.PolicyRecord) error { ... }
```

**Alternativa:** Interfaz explícita para mocking.

```go
type IStore interface {
  InsertPolicies(...) error
}

var _ IStore = &Store{}  // Compile-time check
```

**Decision:** Struct directo es suficiente. Go duck typing + simple interfaces.

### Límites

- Repository pattern es abstracto; Busk es simple (no complex queries)
- Overkill para CRUD trivial, aquí es justificado (validaciones complexas)

---

## 3. Pipeline Pattern (ETL)

### Descripción

Encadenar transformaciones de datos a través de stages. Cada stage produce salida para siguiente.

### Implementación en Busk

```
Stage 1: Escanear & Enqueue
  Entrada:  SFTP remoto
  Proceso:  ListRootFiles(), filtrar spreadsheets, ordenar por prioridad
  Salida:   Job en canal

Stage 2: Descargar & Dedup
  Entrada:  Job
  Proceso:  src.Open(), io.Copy(temp, archive, hasher), calcular SHA-256
  Salida:   Archivo descargado, fileHash, o ERROR si duplicado

Stage 3: Parsear & Mapear
  Entrada:  Archivo descargado
  Proceso:  selectProductCandidateFromWorkbook(), extraer header/rows
  Salida:   Matriz de values, fieldToCol map, o ERROR si no hay formato

Stage 4: Validar Filas
  Entrada:  Matriz de values
  Proceso:  runRules(), applyDiagramRules(), acumular notas
  Salida:   Arreglo de PolicyRecord con status + validaciones

Stage 5: Gate de Archivo
  Entrada:  Arreglo de PolicyRecord
  Proceso:  policiesRowSetHasBlockingIssues()
  Salida:   InsertPolicies() o ERROR sin persisten

Stage 6: Post-procesamiento
  Entrada:  Pólizas insertadas
  Proceso:  CancelMissingStockPolicies(), applyMapfreCancellationsToStock()
  Salida:   Pólizas finales, cancelaciones aplicadas

Stage 7: Reportes & Notificación
  Entrada:  Pólizas finales
  Proceso:  BuildFileValidationReportFromPolicies(), notifyFileProcessing()
  Salida:   Estado terminal (PROCESSED|ERROR|SKIPPED), email enviado
```

### Código

```go
func (s *Service) processByName(job queuedJob) model.FileProcessRecord {
  rec := model.FileProcessRecord{ ... }
  
  // Stage 1: Identificar producto
  products := s.store.FindProductFormatCandidates(fileName)
  if len(products) == 0 {
    rec.Status = model.FileStatusError
    rec.ErrorReason = "no existe producto configurado"
    return rec  // Early exit
  }
  
  // Stage 2: Descargar & dedup
  policies, fileHash, archivePath, selectedProductID, err := validateFile(
    reader, rec.ID, fileName, products, s,
  )
  if err != nil {
    if errors.Is(err, errDuplicateFileHash) {
      rec.Status = model.FileStatusSkipped
      rec.ErrorReason = "archivo omitido: SHA-256 duplicado"
    } else {
      rec.Status = model.FileStatusError
      rec.ErrorReason = err.Error()
    }
    return rec  // Early exit
  }
  
  // Stage 3-4: Validar (dentro validateFile)
  
  // Stage 5: File-level gate
  if policiesRowSetHasBlockingIssues(policies) {
    rec.Status = model.FileStatusError
    rec.ErrorReason = "archivo omitido: incidencias bloqueantes"
    rec.ValidationReportJSON = string(b)
    return rec  // Early exit, NO insert
  }
  
  // Stage 5b: Persistir
  if err := s.store.InsertPolicies(policies); err != nil {
    rec.Status = model.FileStatusError
    rec.ErrorReason = "no se pudieron registrar pólizas"
    return rec
  }
  
  // Stage 6: Post-procesamiento
  if isStockProduct(...) {
    s.store.CancelMissingStockPolicies(...)
  }
  
  // Stage 7: Reportes & notificación
  rec.ValidationReportJSON, rec.ReportArchivePath = validationReportFromPolicies(...)
  rec.Status = model.FileStatusProcessed
  return rec
}
```

### Problema Resuelto

- **Separación de concerns:** Cada stage tiene responsabilidad clara
- **Early exit:** Error en stage N no ejecuta N+1
- **Composability:** Stages pueden reusarse (ej. validación sin insert)
- **Debugging:** Stack trace clara qué stage falló

### Error Handling

```go
// Patrón: error inmediato o continuar?
if somethingWrong {
  rec.Status = model.FileStatusError
  rec.ErrorReason = "..."
  return rec  // No continuar a siguiente stage
}

// Vs. warning (no detiene flujo):
if shouldWarn {
  if rec.ErrorReason == "" {
    rec.ErrorReason = msg
  } else {
    rec.ErrorReason += " | " + msg
  }
  // Continuar al siguiente stage
}
```

---

## 4. Strategy Pattern (Reglas de Validación)

### Descripción

Encapsular algoritmos de validación en objetos intercambiables.

### Implementación en Busk

#### **Estrategia 1: Reglas Declarativas**

```go
type RuleConfig struct {
  Type   string               `json:"type"`  // "required_not_empty", "number_gte", etc.
  Field  string               `json:"field"`
  Params map[string]float64   `json:"params"`
}

// Ejemplo en BD:
// {
//   "type": "number_gte",
//   "field": "monthly_premium",
//   "params": {"min": 0}
// }

func runRules(values map[string]string, rules []model.RuleConfig) (bool, []string) {
  violations := make([]string, 0)
  for _, rule := range rules {
    switch rule.Type {
    case "required_not_empty":
      if strings.TrimSpace(values[rule.Field]) == "" {
        violations = append(violations, fmt.Sprintf("campo requerido: %s", rule.Field))
      }
    case "number_gte":
      val, _ := parseFlexibleNumber(values[rule.Field])
      if min, ok := rule.Params["min"]; ok && val < min {
        violations = append(violations, fmt.Sprintf("%s debe ser >= %f", rule.Field, min))
      }
    case "freeze_on_zero_premium":
      // Return frozen=true si prima es 0
      if prem, _ := parseFlexibleNumber(values["monthly_premium"]); prem == 0 {
        return true, violations
      }
    }
  }
  return false, violations
}
```

#### **Estrategia 2: Reglas Procedurales (Aseguradora)**

```go
func applyDiagramRules(
  productCode string,
  values map[string]string,
  seenCredits map[string]struct{},
  inFileCreditCounts map[string]int,
  inFileCreditRows map[string][]int,
  ruleCfg ruleRuntimeConfig,
  svc *Service,
) ([]string, []string) {
  hards := make([]string, 0)  // Blocking issues
  notes := make([]string, 0)  // Informative
  
  if strings.Contains(strings.ToUpper(productCode), "MAPFRE") {
    // Regla MAPFRE: plan ↔ prima matching
    if err := validateMapfrePlanPremium(values, ruleCfg.AllowedPremiums); err != nil {
      hards = append(hards, err.Error())
    }
    
    // Regla MAPFRE: edad
    if err := validateAge(values["birth_date"], ruleCfg.AgeMin, ruleCfg.AgeMax); err != nil {
      hards = append(hards, err.Error())
    }
  }
  
  if strings.Contains(strings.ToUpper(productCode), "BOLIVAR") {
    // Regla Bolívar: deuda manual
    if debt, _ := parseFlexibleNumber(values["initial_debt_amount"]); 
       debt > ruleCfg.BolivarDebtManualThreshold {
      notes = append(notes, "deuda excede umbral manual, revisar")
    }
    
    // Regla Bolívar: plazo
    if err := validateBolivarTerm(values, ruleCfg); err != nil {
      hards = append(hards, err.Error())
    }
  }
  
  return hards, notes
}
```

### Problema Resuelto

- **Extensibilidad:** Agregar regla = escribir case en switch
- **Reusabilidad:** Reglas MAPFRE aplican a todos los formatos MAPFRE
- **Configurabilidad:** Primas permitidas, parámetros de edad, en BD (no hard-code)
- **Mantenibilidad:** Lógica centralizada, no esparcida en templates

### Variantes

- **Interpreter pattern:** DSL de reglas (JSON complejo)
  - Rechazado: complejidad excesiva para reglas actuales
- **Chain of responsibility:** Encadenar validadores
  - Usado implícitamente: runRules → applyDiagramRules → acumular violations

---

## 5. Builder Pattern (Construir Reportes)

### Descripción

Construir objetos complejos paso a paso.

### Implementación en Busk

```go
func BuildFileValidationReportFromPolicies(
  fileID, fileName, productID, fileStatus, errorReason, processedAt string,
  policies []model.PolicyRecord,
) FileValidationReport {
  
  // Inicializar report vacío
  report := FileValidationReport{
    FileID:          fileID,
    FileName:        fileName,
    ProductID:       productID,
    FileStatus:      fileStatus,
    ErrorReason:     errorReason,
    ProcessedAt:     processedAt,
    PolicyRowCount:  len(policies),
    DuplicateCredits: make([]FileDuplicateCredit, 0),
    PendingValidations: make([]FilePendingValidation, 0),
    InformativeValidations: make([]FilePendingValidation, 0),
  }
  
  // Paso 1: Contar duplicados por crédito
  creditCounts := make(map[string][]int)
  for i, p := range policies {
    credit := strings.TrimSpace(p.CreditNumber)
    if credit != "" {
      creditCounts[credit] = append(creditCounts[credit], i)
    }
  }
  for credit, indices := range creditCounts {
    if len(indices) > 1 {
      dupl := FileDuplicateCredit{
        CreditNumber:        credit,
        Count:               len(indices),
        RowNumbers:          indices,
        DuplicateRowNumbers: indices[1:],  // Todos excepto el primero
      }
      report.DuplicateCredits = append(report.DuplicateCredits, dupl)
      report.TotalDuplicateCredits++
      report.TotalDuplicateRows += len(indices) - 1
    }
  }
  
  // Paso 2: Enumerar validaciones por fila
  for _, p := range policies {
    var notes []string
    if p.ValidationJSON != "" {
      json.Unmarshal([]byte(p.ValidationJSON), &notes)
    }
    
    pv := FilePendingValidation{
      RowNumber:      p.RowNumber,
      DocumentNumber: p.DocumentNumber,
      CreditNumber:   p.CreditNumber,
      PolicyStatus:   p.PolicyStatus,
      Notes:          notes,
    }
    
    if p.PolicyStatus == "MANUAL_REVIEW" || len(notes) > 0 {
      if p.PolicyStatus == "MANUAL_REVIEW" {
        report.PendingValidations = append(report.PendingValidations, pv)
        report.TotalPendingValidations++
      } else {
        report.InformativeValidations = append(report.InformativeValidations, pv)
        report.TotalInformativeValidations++
      }
    }
  }
  
  // Paso 3: Serializar a XLSX (aparte, en saveValidationReportArchive)
  // ...
  
  return report
}
```

### Problema Resuelto

- **Incrementalidad:** Construir report en etapas
- **Claridad:** Cada paso es legible
- **Reusabilidad:** Mismo report sirve para JSON API + XLSX descargable

---

## 6. Observer Pattern (Progress Tracking)

### Descripción

Notificar observers cambios de estado sin acoplamiento directo.

### Implementación en Busk

```go
type ProgressInfo struct {
  FileID    string    `json:"file_id"`
  FileName  string    `json:"file_name"`
  Status    string    `json:"status"`
  Percent   int       `json:"percent"`
  Step      string    `json:"step"`
  CreatedAt time.Time `json:"created_at"`
  UpdatedAt time.Time `json:"updated_at"`
  LastError string    `json:"last_error,omitempty"`
}

type Service struct {
  progressMu sync.RWMutex
  progress   map[string]ProgressInfo
}

// Observer: actualizar progreso
func (s *Service) updateProgress(
  fileID, fileName, status string,
  percent int, step, productID, lastError string,
) {
  s.progressMu.Lock()
  defer s.progressMu.Unlock()
  
  s.progress[fileID] = ProgressInfo{
    FileID:    fileID,
    FileName:  fileName,
    Status:    status,
    Percent:   percent,
    Step:      step,
    CreatedAt: s.progress[fileID].CreatedAt,  // Preserve
    UpdatedAt: time.Now(),
    LastError: lastError,
    ProductID: productID,
  }
}

// Observable: GET /process/progress retorna snapshot
func (s *Service) SnapshotProgress() map[string]ProgressInfo {
  s.progressMu.RLock()
  defer s.progressMu.RUnlock()
  
  copy := make(map[string]ProgressInfo, len(s.progress))
  for k, v := range s.progress {
    copy[k] = v
  }
  return copy
}
```

**Frontend observa via polling:**
```tsx
useEffect(() => {
  const timer = setInterval(() => {
    void loadProgress()
  }, 5000)  // Poll cada 5 segundos
  return () => clearInterval(timer)
}, [autoRefreshProgress, activeTab])
```

### Problema Resuelto

- **Real-time monitoring:** Operador ve progreso sin refrescar página
- **Desacoplamiento:** Processor no conoce Frontend
- **Thread-safety:** RWMutex protege map concurrent reads

---

## 7. Adapter Pattern (File Source Abstraction)

### Descripción

Proporcionar interfaz uniforme para múltiples orígenes de archivos.

### Implementación en Busk

```go
type fileSource interface {
  Open(fileName string) (io.ReadCloser, error)
  MoveToFolder(fileName, folder string) (string, error)
  Close() error
}

type sftpFileSource struct {
  c *sftp.Client
}

func (s sftpFileSource) Open(fileName string) (io.ReadCloser, error) {
  return s.c.Open(fileName)
}

func (s sftpFileSource) MoveToFolder(fileName, folder string) (string, error) {
  dst := fmt.Sprintf("%s/%s", folder, fileName)
  err := s.c.Rename(fileName, dst)
  return dst, err
}

type localFileSource struct {
  baseDir string
}

func (l localFileSource) Open(fileName string) (io.ReadCloser, error) {
  return os.Open(filepath.Join(l.baseDir, fileName))
}

func (l localFileSource) MoveToFolder(fileName, folder string) (string, error) {
  // Crear folder, mover archivo localmente
  // ...
}

// Uso en processByName():
var src fileSource
if job.LocalDir != "" {
  src = localFileSource{baseDir: job.LocalDir}
} else {
  src = sftpFileSource{c: sftpClient}
}

reader, _ := src.Open(fileName)
// reader funciona igual sea SFTP o local
```

### Problema Resuelto

- **Testabilidad:** LocalFileSource para dev/testing
- **Flexibilidad:** Futuro: CloudFileSource (S3, GCS) sin cambiar processByName
- **Reusabilidad:** Mismo código para SFTP + local

---

## 8. Chain of Responsibility (Validación Multi-Capa)

### Descripción

Pasar request (fila validable) por cadena de handlers hasta encontrar uno que lo procese.

### Implementación en Busk

```go
// Cadena: runRules → applyDiagramRules → acumular violations

for i := p.HeaderRow; i < len(rows); i++ {
  row := rows[i]
  
  // Handler 1: Reglas declarativas
  frozen, ruleViolations := runRules(values, p.Rules)
  notes := make([]string, 0)
  for _, rv := range ruleViolations {
    notes = append(notes, noteIncidencia(rv))
  }
  status := "ACTIVE"
  if len(ruleViolations) > 0 {
    status = "MANUAL_REVIEW"
  }
  
  // Handler 2: Congelamiento por prima
  if frozen {
    status = "FROZEN"
    notes = append(notes, noteInformativo("Prima = 0"))
  }
  
  // Handler 3: Reglas procedurales (negocio)
  diagramHards, rowNotes := applyDiagramRules(
    p.Code, values, seenCredits, ...,
  )
  for _, msg := range rowNotes {
    notes = append(notes, noteInformativo(msg))
  }
  for _, msg := range diagramHards {
    notes = append(notes, noteIncidencia(msg))
  }
  if len(diagramHards) > 0 && status == "ACTIVE" {
    status = "MANUAL_REVIEW"
  }
  
  // Handler 4: Anulación MAPFRE
  if isMapfreCancelacionProduct(p.Code) && status == "ACTIVE" {
    status = "CANCELLED"
  }
  
  // Resultado: status final + acumuladas notes
  policies = append(policies, model.PolicyRecord{
    PolicyStatus:   status,
    ValidationJSON: string(noteJSONBytes),
  })
}
```

### Problema Resuelto

- **Composability:** Agregar nuevas reglas = agregar handler
- **Acumulación:** Todas las violations se acumulan (no corto-circuito en primera)
- **Claridad:** Orden de handlers documentado implícitamente

---

## Resumen de Patrones

| Patrón | Ubicación | Problema Resuelto |
|--------|-----------|-------------------|
| Worker Pool | processor.Service | Paralelismo, resource control |
| Repository | store.Store | Desacoplamiento, testabilidad |
| Pipeline | processByName() | Separación de concerns, early exit |
| Strategy | runRules() + applyDiagramRules() | Extensibilidad, configurabilidad |
| Builder | BuildFileValidationReportFromPolicies() | Construcción incremental |
| Observer | Service.progress + updateProgress() | Real-time monitoring |
| Adapter | fileSource interface | Testabilidad, flexibilidad |
| Chain of Responsibility | Validación multi-capa | Composability, acumulación |

