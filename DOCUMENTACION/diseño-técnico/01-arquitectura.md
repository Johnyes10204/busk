# Arquitectura de Busk Seguros

## Resumen Ejecutivo

Busk Seguros es una plataforma integral de ingesta, validación y materialización de pólizas de seguros. Su arquitectura distribuida conecta múltiples sistemas de almacenamiento (SFTP, disco local, MySQL) con un motor de procesamiento de alto rendimiento basado en workers, expuesto mediante una API REST que alimenta una consola de administración web.

**Stack tecnológico:**
- Backend: Go 1.23 (API HTTP + pipeline de procesamiento)
- Frontend: React 19 + TypeScript + Vite
- Almacenamiento: MySQL 8.0+, SFTP remoto, almacenamiento local en disco
- Notificaciones: SendGrid API

---

## Visión General del Sistema

```
┌─────────────────────────────────────────────────────────────────────┐
│                          EXTERIOR (SFTP)                            │
│                   Servidores de Seguros Remotos                     │
│                   (MAPFRE, Bolívar, ESAL, etc.)                     │
└──────────────────────────┬──────────────────────────────────────────┘
                           │
                    [SFTP Bridge]
                           │
        ┌──────────────────┴───────────────────┬────────────────────┐
        │                                      │                    │
        v                                      v                    v
┌──────────────────────┐               ┌────────────────┐   ┌─────────────┐
│  FILES_ARCHIVE_DIR   │               │  REPORTS_      │   │ Disco Local │
│  (Respaldo archivos  │               │  ARCHIVE_DIR   │   │  (modo dev) │
│   originales SHA256) │               │  (Reportes)    │   │             │
└──────────────────────┘               └────────────────┘   └─────────────┘
        │                                      │
        └──────────┬───────────────────────────┘
                   │
    ┌──────────────┴──────────────┐
    │                             │
    v                             v
┌─────────────────────────────────────────────┐
│         BUSK API (Go 1.23, :8080)           │
│                                             │
│  ┌────────────────────────────────────┐    │
│  │  HTTP Routes & Request Handling    │    │
│  │  (main.go, ~1.6k lines)            │    │
│  └────────────────────────────────────┘    │
│                                             │
│  ┌────────────────────────────────────┐    │
│  │  Processor Service                 │    │
│  │  • Worker Pool (async)             │    │
│  │  • File ingestion & parsing        │    │
│  │  • Validation rules engine         │    │
│  │  • Policy materialization          │    │
│  │  • SFTP/Local source handling      │    │
│  └────────────────────────────────────┘    │
│                                             │
│  ┌────────────────────────────────────┐    │
│  │  Store (Repository Pattern)        │    │
│  │  • MySQL queries (Squirrel)        │    │
│  │  • Schema migrations (auto)        │    │
│  │  • Product catalog & formats       │    │
│  │  • Policy persistence              │    │
│  └────────────────────────────────────┘    │
│                                             │
│  ┌────────────────────────────────────┐    │
│  │  Notifier (SendGrid)               │    │
│  │  • Email per archivo               │    │
│  │  • Adjuntos (CSV, XLSX)            │    │
│  │  • Fallback silencioso             │    │
│  └────────────────────────────────────┘    │
│                                             │
│  ┌────────────────────────────────────┐    │
│  │  Internal packages:                │    │
│  │  • config (env loading)            │    │
│  │  • model (DTOs)                    │    │
│  │  • sftp (SFTP client)              │    │
│  │  • processor (file flow)           │    │
│  │  • validationnotes (i18n msgs)     │    │
│  └────────────────────────────────────┘    │
│                                             │
└─────────────────────────────────────────────┘
        │
        └────────────┬────────────────────────────────┐
                     │                                │
                     v                                v
             ┌──────────────┐            ┌────────────────────┐
             │   MySQL DB   │            │   SendGrid API     │
             │  (Pólizas,   │            │   (Notificaciones) │
             │   Archivo,   │            └────────────────────┘
             │   Catálogo)  │
             └──────────────┘
                     │
                     │ (read)
                     │
        ┌────────────┴────────────┐
        │                         │
        v                         v
┌──────────────────┐      ┌──────────────────────────┐
│  Frontend Admin  │      │  Query & API Consumers   │
│  (React 19)      │      │  (Scripts, dashboards)   │
│  Port :5173      │      │                          │
└──────────────────┘      └──────────────────────────┘
```

---

## Componentes Principales

### 1. **API HTTP (Go)**

**Propósito:** Exponer todas las operaciones de ingesta, validación, consulta y configuración mediante REST.

**Ubicación:** `services/api/`

**Características clave:**
- **Single binary:** `go run main.go` o `go build -o busk ./services/api`
- **Puerto:** `:8080` (configurable con `API_PORT`)
- **Rutas principales:**
  - `/api/v1/health` → Health check
  - `/api/v1/process/scan` → Enqueue SFTP files
  - `/api/v1/process/scan-local` → Enqueue local files (testing)
  - `/api/v1/process/progress` → Real-time processing status
  - `/api/v1/products*` → Catalog CRUD
  - `/api/v1/product-formats*` → Format CRUD
  - `/api/v1/product-formats/match-test` → Test file-to-format matching
  - `/api/v1/products/allowed-premiums*` → Premium catalog management
  - `/api/v1/files*` → File records, summaries, validation reports
  - `/api/v1/policies*` → Policy query & search
  - `/api/v1/bootstrap/sample-products` → Seed initial data

**Flujo de inicialización:**
```
main() {
  loadDotEnv()              // .env + config.json
  Store.NewMySQLFromEnv()   // Connect MySQL, run migrations
  Processor.New()           // Start worker pool
  http.ListenAndServe()     // Listen on :8080
}
```

---

### 2. **Processor Service (Worker Pool)**

**Propósito:** Orquestar el procesamiento asíncrono de archivos con garantías de finalización.

**Ubicación:** `services/api/internal/processor/`

**Arquitectura:**
```
ScanAndEnqueue()  ──enqueue──>  [ Job Channel (256 capacity) ]
                                         │
                    ┌────────────────────┼────────────────────┐
                    │                    │                    │
                    v                    v                    v
                 Worker_1            Worker_2              Worker_N
                 runJob()            runJob()              runJob()
                    │                    │                    │
                    └────────────────────┼────────────────────┘
                                         │
                    ┌────────────────────┼────────────────────┐
                    │                    │                    │
                    v                    v                    v
            ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
            │  processByName()│  │ validateFile()  │  │ InsertPolicies()│
            │                 │  │                 │  │                 │
            │ • ID producto   │  │ • Parse XLSX    │  │ • Persist póliz│
            │ • Open remote   │  │ • Validate rows │  │ • Fire stock    │
            │ • Extract rows  │  │ • Generate rept │  │   cancellations │
            │                 │  │                 │  │ • Apply MAPFRE  │
            │ [SFTP|Local]    │  │ [Rules engine]  │  │   cancellations │
            │                 │  │                 │  │ • Notify        │
            └─────────────────┘  └─────────────────┘  └─────────────────┘
                    │                    │                    │
                    └────────────────────┼────────────────────┘
                                         │
                                         v
                            ┌──────────────────────────┐
                            │  AddFileRecord(record)   │
                            │  Update DB status,       │
                            │  validation report,      │
                            │  archive paths           │
                            └──────────────────────────┘
                                         │
                                         v
                         [state = PROCESSED|ERROR|SKIPPED]
                                         │
                                         v
                            ┌──────────────────────────┐
                            │  notifyFileProcessing()  │
                            │  SendGrid email with     │
                            │  attached CSV/XLSX       │
                            └──────────────────────────┘
```

**Garantías de finalización:**
- Defer/recover en `runJob()` captura panics y marca archivo como ERROR
- `FinalizeFileStatus()` es una red de seguridad: UPDATE mínimo al estado terminal
- `MarkStaleFilesAsError()` se ejecuta al startup para recuperar archivos huérfanos
- Todo archivo debe terminar en estado PROCESSED, ERROR o SKIPPED

**Configuración:**
```bash
PROCESSOR_WORKERS=2                        # (default)
PROCESSOR_READ_FULL_FILE_ON_ROW_ERRORS=false
```

---

### 3. **Store (Repository Pattern)**

**Propósito:** Abstraer todas las operaciones de base de datos.

**Ubicación:** `services/api/internal/store/`

**Operaciones principales:**
- **Productos & Formatos:** `UpsertProduct()`, `ListProductFormats()`, `FindProductFormatCandidates()`
- **Políticas:** `InsertPolicies()`, `ListPoliciesByProduct()`, `SearchPoliciesPage()`, `CancelMissingStockPolicies()`
- **Archivos:** `AddFileRecord()`, `ListFileRecords()`, `GetFileValidationReport()`, `GetFileQualitySummary()`
- **Primas permitidas:** `UpsertAllowedPremiums()`, `GetAllowedPremiums()`, `AddAllowedPremium()`, `DeleteAllowedPremium()`
- **Dedup & estado:** `FileHashAlreadyProcessed()`, `FinalizeFileStatus()`, `MarkStaleFilesAsError()`
- **Parámetros de reglas:** `UpsertProductRuleParam()`, `UpsertGlobalRuleParam()`

**Builder SQL:** Squirrel (type-safe query builder, no ORM)

**Migraciones:**
- Automáticas al startup via `runMigrations()` en `store.go`
- Clave: mapa `migrations` con claves sortables `YYYYMMDDNN`
- Ejemplo: `20250801_001_create_schema`, `20250802_001_add_product_formats`

---

### 4. **Frontend Admin (React)**

**Propósito:** Consola de administración para operadores.

**Ubicación:** `frontend-admin/`

**Stack:**
- React 19 (Hooks)
- TypeScript
- Vite (dev server + build)
- CSS puro (sin framework, custom styling)

**Tabs principales:**
1. **Operación:** Health check, scan SFTP, monitoreo de progreso en tiempo real
2. **Productos:** CRUD de productos, formatos y reglas (editores JSON)
3. **Primas:** Gestión de catálogos permitidos por producto
4. **Archivos:** Listado, descarga, retry, validación reports (CSV/XLSX)
5. **Pólizas:** Búsqueda avanzada (doc/crédito/producto), paginación

**API proxy:** 
```
GET /api/v1/* → http://localhost:8080/api/v1/*
```

**Características:**
- Auto-refresh de progreso (5s) en tab Operación
- Incidentes recientes (últimos 6 errores)
- Paso-a-paso wizard para configurar productos nuevos
- Test de matching de formatos (headers CSV vs prefijo archivo)

---

### 5. **SFTP Bridge**

**Propósito:** Conectar con servidores remotos para descargar e ingesting archivos.

**Ubicación:** `services/api/internal/sftp/`

**Flujo:**
1. `ScanAndEnqueue()` → `c.ListRootFiles()` lista raíz del SFTP
2. Filtra spreadsheets (`.xlsx`, `.xls`, `.csv`)
3. Ordena por prioridad (`filePriority()`: STOCK primero, luego INCLUSION, etc.)
4. Enqueue cada archivo en canal de jobs
5. Worker descarga vía `src.Open(fileName)` → `io.Copy(temp, archive, hasher)`
6. Tras procesamiento: `src.MoveToFolder(fileName, "PROCESSED"|"ERROR")`

**Standalone tool:**
```bash
cd tools/sftpconnect
SFTP_PASSWORD='...' go run .
```

---

### 6. **Almacenamiento**

#### **MySQL (Primary Store)**

Tablas principales:
```sql
-- Catálogo
products            (id, code, insurer, file_prefix, mappings_json, rules_json)
product_formats     (id, product_id, file_prefix, priority, active, mappings_json, rules_json)

-- Datos
policies            (file_id, product_id, document_number, credit_number, policy_status, 
                     raw_data_json, validation_json)
processed_files     (id, file_name, status, file_hash, error_reason, 
                     validation_report_json, archive_path, report_archive_path)

-- Tunables
product_allowed_premiums  (product_id, premium_value)
product_rule_params       (product_id, param_name, param_value)
global_rule_params        (param_name, param_value)
```

#### **Almacenamiento Local**

- **FILES_ARCHIVE_DIR** (default `./data/files-archive/`): Respaldo SHA-256 de cada archivo ingested
- **REPORTS_ARCHIVE_DIR** (default `./data/reports-archive/`): Reportes de validación en XLSX para auditoría

#### **SFTP Remoto**

Estructura esperada:
```
sftp://host:port/remote_dir/
  ├── PROCESSED/
  ├── ERROR/
  └── [archivos XLSX/XLS/CSV]
```

---

## Flujo End-to-End Completo

```
1. ENTRADA
   └─> POST /api/v1/process/scan
       └─> ScanAndEnqueue() lista SFTP, enqueue archivos ordenados por prioridad

2. ENCOLA
   └─> Archivo entra en canal (256 capacity)
       └─> Status: QUEUED

3. WORKER PICKUP
   └─> Worker_N obtiene job
       └─> Status: PROCESSING

4. ID PRODUCTO
   └─> FindProductFormatCandidates(fileName)
       └─> Match por substring case-insensitive del file_prefix
           └─> Retorna candidates ordenados por length, priority, created_at
               └─> Si no hay match → ERROR, mover a ERROR/

5. DESCARGA & DEDUP
   └─> src.Open(fileName) → io.Copy(temp, archive, hasher)
       └─> Calcular SHA-256
           └─> Si duplicado (ya PROCESSED|SKIPPED) → SKIPPED, mover a PROCESSED/

6. PARSING
   └─> selectProductCandidateFromWorkbook()
       └─> Abrir XLSX/XLS → Parse sheet_name, header_row
           └─> Extraer header y rows
               └─> Build fieldToCol map (canonical field → column index)

7. VALIDACIÓN POR FILA
   └─> Para cada row (excepto header):
       ├─> Mapeo canónico: raw header values → canonical fields
       ├─> Reglas declaradas: runRules()
       ├─> Reglas de negocio por aseguradora: applyDiagramRules()
       │   (MAPFRE: plan↔prima matching, fechas, edad)
       │   (BOLÍVAR: deuda, plazo, fecha vencimiento, prima)
       ├─> Si hay violations y PROCESSOR_READ_FULL_FILE_ON_ROW_ERRORS=false
       │   → Error inmediato, devolver archivo sin persiste
       └─> Si violations con flag true → Status=MANUAL_REVIEW, guardar notas

8. FILE-LEVEL GATE (CRÍTICA)
   └─> Si alguna fila tiene blocking issues (MANUAL_REVIEW o regla hard):
       ├─> NO persisten pólizas
       ├─> Status: ERROR
       ├─> Generar informe de validación (JSON + XLSX)
       └─> Mover a ERROR/, notificar

9. PERSISTENCIA
   └─> Si no hay blocking issues:
       ├─> InsertPolicies() → INSERT INTO policies (transacción)
       ├─> Si es STOCK → CancelMissingStockPolicies()
       │   (Pólizas históricas del producto no en stock actual → CANCELLED)
       ├─> Si es MAPFRE anulación → applyMapfreCancellationsToStock()
       │   (Marcar filas stock matching como CANCELLED)
       └─> Status: PROCESSED

10. REPORTES & ARCHIVO
    └─> BuildFileValidationReportFromPolicies()
        ├─> JSON validaciones en BD
        ├─> XLSX en disco (audit trail)
        └─> Usar XLSX en archivo, no regenerar

11. MOVIMIENTO SFTP
    └─> moveRemoteFile(fileName, "PROCESSED"|"ERROR")
        └─> Si falla → Advertencia, no fallar carga

12. NOTIFICACIÓN
    └─> notifyFileProcessing()
        ├─> Status terminal (PROCESSED|ERROR|SKIPPED) obligatorio
        ├─> SendGrid email con adjunto (CSV/XLSX)
        └─> Si falla → Log + Set email_error en BD, no re-raise

13. ESTADO FINAL
    └─> AddFileRecord() + FinalizeFileStatus()
        └─> Status = PROCESSED|ERROR|SKIPPED (jamás PENDING|QUEUED|PROCESSING)
```

---

## Interacciones Clave

### **Processor ↔ Store**

Processor depende de Store para:
- Buscar candidatos de producto/formato
- Insertar pólizas
- Actualizar estado de archivo
- Obtener parámetros de reglas
- Dedup por SHA-256

```go
products := s.store.FindProductFormatCandidates(fileName)
if err := s.store.InsertPolicies(policies); err != nil { ... }
s.store.AddFileRecord(rec)
```

### **Processor ↔ Notifier**

Processor notifica al Notifier al final del procesamiento:

```go
s.notifier.NotifyFileProcessing(notify.FileEmailInput{
  FileID:               rec.ID,
  FileName:            rec.FileName,
  Status:              string(rec.Status),
  ValidationReportJSON: rec.ValidationReportJSON,
  ArchivePath:         rec.ArchivePath,
  ReportArchivePath:   rec.ReportArchivePath,
})
```

### **API ↔ Frontend**

Frontend llama a API vía fetch:
- `GET /health` → Verificar salud
- `POST /process/scan` → Trigger scan
- `GET /process/progress` → Poll estado (5s interval)
- `POST /files/retry?file_id=...` → Reintentar archivo
- `GET /files/validation-report?file_id=...` → Obtener informe
- `GET /files/validation-xlsx?file_id=...` → Descargar XLSX

---

## Patrones de Seguridad

### **Isolación de Datos**

- **Per-file hash:** Dedup global por contenido (SHA-256)
- **Per-product context:** Cada formato pertenece a un producto, rules al producto
- **Status transitions:** `PENDING → QUEUED → PROCESSING → {PROCESSED|ERROR|SKIPPED}` (nunca atrás)

### **Concurrencia**

- Worker pool (configurable, default 2) procesa archivos en paralelo
- Job channel buffered (256) para no sobrecargar memoria
- Mutex en `Service.progress` para thread-safe status updates
- MySQL transacciones para InsertPolicies (atomicidad)

### **Recuperación ante fallos**

- Panic recovery en `runJob()` → ERROR + stack para debug
- `MarkStaleFilesAsError()` al startup
- `FinalizeFileStatus()` como net safety si `AddFileRecord()` falla
- Retry logic en API (`/files/retry`)

---

## Herramientas de Desarrollo

```bash
# Dev all-in-one
bash tools/dev/start-api-with-docs.sh

# API standalone
cd services/api && go run main.go

# Frontend standalone
cd frontend-admin && npm run dev

# Tests
cd services/api && go test ./...

# SFTP connectivity test
cd tools/sftpconnect && SFTP_PASSWORD='...' go run .
```

---

## Resumen de Capacidades

| Aspecto | Capacidad |
|---------|-----------|
| **Archivo máximo** | Limitado por memoria (suele ~100MB de política por worker) |
| **Políticas por archivo** | Sin límite (validación row-by-row) |
| **Productos activos** | Sin límite (búsqueda indexable en BD) |
| **Primas per-product** | Sin límite (almacenadas en tabla `product_allowed_premiums`) |
| **Workers concurrentes** | Configurable (default 2) |
| **Job queue capacity** | 256 (tunable) |
| **Validación de edad** | Integrada en reglas MAPFRE/BOLÍVAR |
| **Cancelación automática** | STOCK + MAPFRE anulación masiva |
| **Notificaciones** | SendGrid (silent fallback si env var no set) |

---

## Próximos Pasos

Consultar:
1. **02-componentes.md** → Detalles técnicos de cada componente
2. **03-decisiones-tecnicas.md** → Por qué cada tecnología y trade-offs
3. **04-patrones-diseño.md** → Patrones reutilizables
4. **05-seguridad.md** → Mitigación de riesgos específicos
5. **06-performance-escalabilidad.md** → Optimización y tunables
