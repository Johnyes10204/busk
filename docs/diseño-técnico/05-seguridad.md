# Seguridad: Mitigación de Riesgos

## 1. Validación de Entrada

### 1.1 HTTP Input Sanitization

Todos los parámetros HTTP se validan antes de usar:

```go
// GET /api/v1/products/allowed-premiums?product_id=X
productID := strings.TrimSpace(r.URL.Query().Get("product_id"))
if productID == "" {
  writeJSON(w, http.StatusBadRequest, map[string]string{
    "error": "query param product_id es obligatorio",
  })
  return
}

// File ID en URL
fileID := strings.TrimSpace(r.URL.Query().Get("file_id"))
if fileID == "" {
  writeJSON(w, http.StatusBadRequest, map[string]string{
    "error": "query param file_id es obligatorio",
  })
  return
}

// Page/size con límites
page, pageSize := 1, 50
if raw := r.URL.Query().Get("page"); raw != "" {
  n, err := strconv.Atoi(raw)
  if err != nil || n < 1 {
    writeJSON(w, http.StatusBadRequest, map[string]string{
      "error": "page inválido",
    })
    return
  }
  page = n
}
const maxPageSize = 200
if pageSize > maxPageSize {
  pageSize = maxPageSize  // Enforce limit
}
```

### 1.2 JSON Parsing Seguro

```go
// POST /api/v1/products
var p model.Product
if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
  writeJSON(w, http.StatusBadRequest, map[string]string{
    "error": err.Error(),
  })
  return
}

// Validar campos obligatorios
if p.ID == "" || p.Code == "" {
  writeJSON(w, http.StatusBadRequest, map[string]string{
    "error": "id y code son obligatorios",
  })
  return
}

// Validar config de producto (si mappings)
if len(p.Mappings) > 0 {
  if missing := validateProductConfig(p); len(missing) > 0 {
    writeJSON(w, http.StatusBadRequest, map[string]any{
      "error":          "configuración incompleta",
      "missing_fields": missing,
    })
    return
  }
}
```

### 1.3 Mapeo Canónico

Cada header Excel/CSV se mapea a campo canónico con validación:

```go
// Header Excel: "IDENTIFICACIONAFILIADO" → Canonical: "document_number"
// Si header no mapeado como requerido → Error

headerIdx := make(map[string]int)
for i, h := range header {
  headerIdx[strings.ToUpper(strings.TrimSpace(h))] = i
}

fieldToCol := make(map[string]int)
for _, m := range p.Mappings {
  col, ok := columnIndexForFieldMap(header, m)
  if !ok {
    if m.Required {
      return nil, fileHash, archivePath, p.ID, 
        fmt.Errorf("falta columna requerida: %s", m.SourceHeader)
    }
    continue
  }
  fieldToCol[m.CanonicalField] = col
}
```

---

## 2. Inyección SQL Prevention

### 2.1 Parameterized Queries

**NUNCA** concatenar strings en SQL:

```go
// MALO:
query := fmt.Sprintf("SELECT * FROM policies WHERE product_id = '%s'", productID)

// BUENO: Squirrel query builder (prepared statements)
sqlStr, args, _ := s.sb.Select("...").
  From("policies").
  Where(sq.Eq{"product_id": productID}).
  ToSql()
rows, _ := s.db.Query(sqlStr, args...)
```

Ejemplo con LIKE:
```go
// Squirrel maneja escaping automático
Where(sq.Expr("UPPER(?) LIKE CONCAT('%', UPPER(f.file_prefix), '%')", name))
// Genera: WHERE UPPER(?) LIKE CONCAT('%', UPPER(f.file_prefix), '%')
// args = [name]  (name nunca interpolado, es parametrizado)
```

### 2.2 Audit Trail

Todas las writes se loguean:

```go
log.Printf("[processor] file_id=%s status=%s archived_to=%s", 
  rec.ID, rec.Status, rec.ArchivePath)
```

---

## 3. Inyección SFTP & Path Traversal

### 3.1 Nombres de Archivo Validados

Archivos remotos se filtran por extensión:

```go
func isSpreadsheet(name string) bool {
  lower := strings.ToLower(name)
  return strings.HasSuffix(lower, ".xlsx") ||
         strings.HasSuffix(lower, ".xls") ||
         strings.HasSuffix(lower, ".csv")
}

// Uso
for _, e := range entries {
  if e.IsDir() { continue }
  name := e.Name()
  if !isSpreadsheet(name) { continue }  // Rechazar ejecutables, scripts, etc.
  candidates = append(candidates, name)
}
```

### 3.2 Path Traversal Prevention

Nombres remotos se usan directamente (no path join en local):

```go
// MALO:
path := filepath.Join("/uploads", remoteName)  // remoteName podría ser "../../../etc/passwd"

// BUENO: respaldo en disco via archiveDir + hash
archivePath, err := buildArchivePath(fileID, fileName)
// buildArchivePath retorna: {ARCHIVE_DIR}/{fileID_hash}

func buildArchivePath(fileID, fileName string) (string, error) {
  archiveDir := strings.TrimSpace(os.Getenv("FILES_ARCHIVE_DIR"))
  if archiveDir == "" {
    archiveDir = "./data/files-archive"
  }
  os.MkdirAll(archiveDir, 0755)
  
  // Usar fileID (generado por servidor) + hash, NO fileName
  hash := sha256.Sum256([]byte(fileID + fileName))
  archiveFile := filepath.Join(archiveDir, fmt.Sprintf("%x", hash))
  return archiveFile, nil
}
```

---

## 4. Datos Sensibles

### 4.1 Credenciales SFTP

**NUNCA** loguear passwords:

```go
// OK: loguear operación sin credenciales
log.Printf("[sftp] conectando a host=%s port=%s user=%s", host, port, user)

// MALO:
log.Printf("[sftp] conectando con password=%s", password)  // ¡¡ NUNCA !!
```

Variables de entorno:
```bash
SFTP_PASSWORD="secret123"
SENDGRID_API_KEY="SG.xxx..."
MYSQL_DSN="root:password@tcp(...)"
```

Requiere secrets management:
- Kubernetes: `kubectl create secret`
- Docker: Docker Secrets o environment file (permisos 0600)
- Manual: `.env.local` con permisos restrictivos (gitignored)

### 4.2 Datos PII en Respuestas

Campos sensibles son opcionales en response:

```go
// GET /api/v1/policies/search?credit_number=123&include_raw=false
// Respuesta:
{
  "customer_data": {
    "id_number": "123456789",     // PII, pero necesario
    "full_name": "Juan Pérez",    // PII
    "birth_date": "1980-05-15",   // PII
    "email": "..."
  },
  "raw_data": null   // Omitido si include_raw=false
}
```

Función `compactMap()` elimina campos vacíos:

```go
func compactMap(m map[string]any) map[string]any {
  out := make(map[string]any, len(m))
  for k, v := range m {
    switch val := v.(type) {
    case string:
      if strings.TrimSpace(val) != "" {
        out[k] = v  // Solo si no vacío
      }
    // ... más tipos
    }
  }
  return out
}
```

---

## 5. Acceso a Base de Datos

### 5.1 Connection Security

```go
// DSN con SSL/TLS (recomendado en producción)
dsn := "user:pass@tcp(db.example.com:3306)/busk?tls=true&parseTime=true"
db, _ := sql.Open("mysql", dsn)

// Connection pooling
db.SetMaxOpenConns(25)    // Limite conexiones abiertas
db.SetMaxIdleConns(5)     // Reciclaje de conexiones
db.SetConnMaxLifetime(5 * time.Minute)
```

### 5.2 Least Privilege

Usuario MySQL debe tener permisos mínimos:

```sql
-- Usuario API (solo SELECT, INSERT, UPDATE en tablas específicas)
CREATE USER 'busk_api'@'%' IDENTIFIED BY '...';
GRANT SELECT, INSERT, UPDATE ON busk.products TO 'busk_api'@'%';
GRANT SELECT, INSERT, UPDATE ON busk.policies TO 'busk_api'@'%';
GRANT SELECT, INSERT, UPDATE ON busk.processed_files TO 'busk_api'@'%';
-- NOT GRANT ALL PRIVILEGES

-- Usuario de migración (solo en deployment)
CREATE USER 'busk_migrate'@'localhost' IDENTIFIED BY '...';
GRANT ALL ON busk.* TO 'busk_migrate'@'localhost';
```

### 5.3 Query Timeout

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

row := s.db.QueryRowContext(ctx, sqlStr, args...)
```

---

## 6. Autorización (Access Control)

### 6.1 No hay autenticación en Busk v1

⚠️ **IMPORTANTE:** La API no implementa autenticación.

**Asunción:** Desplegada en red privada (VPN, intranet).

**Para producción:**
```go
// Middleware de autenticación (ejemplo)
func requireAPIKey(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    apiKey := r.Header.Get("X-API-Key")
    expected := os.Getenv("API_KEY_SECRET")
    if apiKey != expected {
      writeJSON(w, http.StatusUnauthorized, map[string]string{
        "error": "unauthorized",
      })
      return
    }
    next.ServeHTTP(w, r)
  })
}

// Aplicar a rutas
mux.Handle("/api/v1/products", requireAPIKey(
  http.HandlerFunc(productsHandler),
))
```

Alternativa: OAuth2 / JWT (más complejo).

### 6.2 Frontend (No implementado)

Frontend admin expuesto al mundo sin autenticación.

**Para producción:**
- Proteger con proxy reverse (nginx + auth básica)
- O agregar autenticación React + session cookies

```bash
# nginx.conf
server {
  listen 80;
  server_name admin.example.com;
  
  auth_basic "Restricted";
  auth_basic_user_file /etc/nginx/.htpasswd;
  
  location / {
    proxy_pass http://localhost:5173;
  }
  
  location /api/v1 {
    proxy_pass http://localhost:8080;
  }
}
```

---

## 7. Inyección CSV/XLSX

### 7.1 Datos Generados

Reportes de validación se generan desde datos en BD, no del archivo original:

```go
// Safe: datos serializados por json.Marshal()
b, _ := json.Marshal(values)
rawDataJSON := string(b)

// Cuando exportar a CSV:
content := buildCSVFromReport(report)
// Usar report.PendingValidations (estructurado), no raw file content
```

### 7.2 Fórmulas de Excel

Si algún dato comienza con `=`, `+`, `-`, `@`, escape antes de XLSX:

```go
func escapeExcelFormula(s string) string {
  if len(s) > 0 && (s[0] == '=' || s[0] == '+' || s[0] == '-' || s[0] == '@') {
    return "'" + s  // Prefix ' escapa en Excel
  }
  return s
}

// Usar al serializar a XLSX:
cell.Value = escapeExcelFormula(policyData.DocumentNumber)
```

---

## 8. Rate Limiting & DoS Prevention

### 8.1 Queue Capacity

Worker pool tiene buffer limitado (256 jobs):

```go
select {
case s.jobs <- job:
  enqueued++
default:
  // Queue full: rechazar archivo
  s.store.AddFileRecord(model.FileProcessRecord{
    ID:          job.ID,
    Status:      model.FileStatusError,
    ErrorReason: "cola de procesamiento llena",
  })
}
```

### 8.2 Paginación Forzada

```go
const maxPageSize = 200
if pageSize > maxPageSize {
  pageSize = maxPageSize
}

// GET /policies/search?product_id=X&page_size=999
// → page_size limitado a 200
```

### 8.3 Timeout en SFTP

```go
client, err := sftpclient.Connect(cfg)
if err != nil {
  log.Printf("[processor] SFTP timeout: %v", err)
  return model.FileProcessRecord{
    Status:      model.FileStatusError,
    ErrorReason: "SFTP connection timeout",
  }
}
```

---

## 9. Logging & Auditoría

### 9.1 Todos los eventos críticos se loguean

```go
log.Printf("[processor] file_id=%s file=%s status=%s", rec.ID, rec.FileName, rec.Status)
log.Printf("[store] AddFileRecord file_id=%s validation_json_bytes=%d", 
  r.ID, len(r.ValidationReportJSON))
log.Printf("[processor] hash calculado file_id=%s sha256=%s", fileID, fileHash)
log.Printf("[processor] stock cancelaciones producto=%s file_id=%s canceladas=%d", 
  productID, rec.ID, cancelled)
```

### 9.2 Niveles de Log

Busk no implementa niveles (debug/info/warn/error).

**Mejora futura:**
```go
log.Warnf("[processor] worker_panic file_id=%s", job.ID)
log.Infof("[processor] file processed OK file_id=%s", job.ID)
log.Errorf("[store] query error: %v", err)
```

### 9.3 Rotación de Logs

Logs van a stdout (systemd/Docker maneja rotación):

```bash
# systemd
journalctl -u busk --since="1 hour ago" -f

# Docker
docker logs --tail=100 busk-container
```

---

## 10. Actualización de Dependencias

### 10.1 Go Modules

```bash
go mod download   # Descargar dependencias
go mod verify     # Verificar checksums
go mod tidy       # Remover no usadas
go mod graph      # Visualizar árbol
```

### 10.2 Scanning de Vulnerabilidades

```bash
go install github.com/google/osv-scanner/cmd/osv-scanner@latest
osv-scanner -r .

# Alternativa: govulncheck
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

---

## 11. Secrets Management Checklist

| Secret | Dónde Vive | Cómo Compartir |
|--------|-----------|----------------|
| `MYSQL_DSN` | Env var | Kubernetes secret / AWS Secrets Manager |
| `SFTP_PASSWORD` | Env var | Mismo |
| `SENDGRID_API_KEY` | Env var | Mismo |
| `API_KEY_SECRET` | Env var (si auth) | Mismo |
| `.env` local | `.env.local` (gitignored) | Nunca commit, solo dev |
| DB backups | `/var/lib/mysql/backup` | Permisos 0600, encriptado |

---

## 12. Seguridad en Deployment

### 12.1 Dockerfile (Ejemplo)

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY . .
RUN go build -o busk services/api/main.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates mysql-client
COPY --from=builder /build/busk /usr/local/bin/
RUN adduser -D -s /bin/false busk
USER busk
EXPOSE 8080
CMD ["busk"]
```

### 12.2 Network Policy (Kubernetes)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: busk-api
spec:
  podSelector:
    matchLabels:
      app: busk-api
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress
      ports:
        - protocol: TCP
          port: 8080
  egress:
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 3306  # MySQL
        - protocol: TCP
          port: 22    # SFTP
        - protocol: TCP
          port: 443   # SendGrid API (HTTPS)
```

### 12.3 HTTPS en Producción

```bash
# nginx + Let's Encrypt
certbot certonly --webroot -w /var/www/html -d api.example.com

# nginx.conf
server {
  listen 443 ssl http2;
  server_name api.example.com;
  
  ssl_certificate /etc/letsencrypt/live/api.example.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/api.example.com/privkey.pem;
  ssl_protocols TLSv1.2 TLSv1.3;
  ssl_ciphers HIGH:!aNULL:!MD5;
  
  location /api/v1 {
    proxy_pass http://localhost:8080;
  }
}
```

---

## Resumen de Mitigaciones

| Riesgo | Mitigación |
|--------|-----------|
| **Inyección SQL** | Squirrel query builder, prepared statements |
| **Path traversal** | Nombres de archivo filtrados por extensión, hash local |
| **Inyección SFTP** | Validación de entrada, sanitización |
| **Credenciales expuestas** | Env vars, secrets manager, no logs de passwords |
| **Acceso no autorizado** | Middleware auth (futuro), red privada |
| **DoS** | Queue capacity, paginación forzada, timeouts |
| **Datos PII** | Campos opcionales en response, compactMap |
| **Fórmulas Excel** | Escape de caracteres especiales |
| **Vulnerabilidades deps** | go mod verify, govulncheck |
| **Auditoria** | Logs de todos eventos críticos |
| **HTTPS en prod** | nginx + TLS 1.2+ |

