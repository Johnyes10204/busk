# 03. Ejecución de Pruebas — Busk Seguros

## 1. Pruebas Unitarias (Go)

### 1.1 Ejecutar todos los tests

```bash
cd services/api
go test ./...
```

**Salida esperada:**
```
=== RUN   TestSeedFilePrefixes_CoverageDownloads
--- PASS: TestSeedFilePrefixes_CoverageDownloads (0.00s)
...
--- PASS: TestMapfrePrimaCoincideTarifa (0.00s)
PASS
ok  	github.com/buskseguros-design/services/api	2.345s
```

**Tiempo total:** ~2-3 segundos (143 tests)

### 1.2 Ejecutar test específico

```bash
# Test individual
go test ./internal/processor -run TestValidarPlanMapfre_PrimaNoCoincidePlan -v

# Múltiples tests con patrón
go test ./... -run "TestBolivar*" -v

# Todos en un paquete
go test ./internal/processor -v
```

### 1.3 Verbose output

```bash
go test ./... -v
```

Muestra cada test:
```
=== RUN   TestCompletedYearsBetween
--- PASS: TestCompletedYearsBetween (0.00s)
```

### 1.4 Cobertura de código

```bash
# Generar coverage report
go test ./... -coverprofile=coverage.out

# Ver en HTML
go tool cover -html=coverage.out -o coverage.html
open coverage.html

# Resumen por función
go tool cover -func=coverage.out
```

**Resultado esperado:** ~60-70% cobertura en processor/

### 1.5 Race detector

```bash
go test -race ./...
```

Detecta data races en goroutines (sin resultados esperados — código es linear)

### 1.6 Compilación sin tests

```bash
go build ./...
go build -o busk-api ./
```

### 1.7 Benchmark (si existen)

```bash
go test -bench=. ./...
```

No hay benchmarks actualmente, pero estructura disponible.

### 1.8 Información tests disponibles

```bash
# Listar todos sin ejecutar
go test -list ./...

# Listar en paquete específico
go test -list ./internal/processor
```

### 1.9 Fallos y debugging

```bash
# Mostrar log output (t.Log, t.Fatalf output)
go test ./... -v

# Debugger con dlv
dlv test ./internal/processor -- -test.run TestValidarPlanMapfre_PrimaNoCoincidePlan
```

### 1.10 Requisitos

- Go 1.23+ (`go version`)
- NO requiere MySQL, SFTP, ni variables de entorno
- Todos los tests son **unitarios puros**

---

## 2. Pruebas de Integración (Manual + Postman)

### 2.1 Setup

**Requisitos:**
1. MySQL ejecutando:
```bash
# macOS con Homebrew
brew services start mysql
# O Docker
docker run -d -p 3306:3306 -e MYSQL_ROOT_PASSWORD=root mysql:8.0
```

2. Variables de entorno:
```bash
export MYSQL_DSN="root@tcp(127.0.0.1:3306)/busk?parseTime=true&multiStatements=true"
export PROCESSOR_WORKERS=2
```

3. API Go ejecutando:
```bash
cd services/api
go run main.go
# Escucha en :8080
```

### 2.2 Bootstrap (crear productos)

```bash
curl -X POST http://localhost:8080/api/v1/bootstrap/sample-products

# Respuesta esperada:
# { "message": "sample products created" }
```

Esto crea en BD:
- 2 productos (MAPFRE, BOLÍVAR)
- 9 formatos (Vida, ACC, Cáncer, Deudores Banco Micro/Pyme, ESAL Micro/Pyme, Stock, Anulación)
- Prefijos de matching

### 2.3 Verificar productos

```bash
curl http://localhost:8080/api/v1/products

# Respuesta: Array de productos
```

### 2.4 Listar formatos activos

```bash
curl http://localhost:8080/api/v1/product-formats/active

# Respuesta: Array de formatos con mappings + rules
```

### 2.5 Scan SFTP (requiere SFTP configurado)

```bash
export SFTP_HOST="sftp.example.com"
export SFTP_PORT="22"
export SFTP_USER="user"
export SFTP_PASSWORD="pass"
export SFTP_REMOTE_DIR="/incoming"

curl -X POST http://localhost:8080/api/v1/process/scan

# Respuesta:
# { "message": "scan iniciado", "filesEnqueued": 5 }
```

### 2.6 Monitorear progreso

```bash
# Polls en loop
while true; do
  curl http://localhost:8080/api/v1/process/progress
  sleep 2
done

# Respuesta:
# {
#   "activeWorkers": 2,
#   "filesQueued": 3,
#   "processed": [
#     {
#       "file_id": "f123",
#       "file_name": "STOCK_MAPFRE_Marzo.xlsx",
#       "status": "PROCESSED",
#       "policiesCreated": 1250
#     }
#   ]
# }
```

### 2.7 Listar archivos procesados

```bash
curl http://localhost:8080/api/v1/files

# Respuesta: Array de processed_files con status, counts
```

### 2.8 Descargar reporte de validación

```bash
curl http://localhost:8080/api/v1/files/validation-xlsx?fileId=f123 \
  -o reporte.xlsx

# Archivo descargable con sheets: Datos archivo, Incidencias, Informes, Espejo
```

### 2.9 Reporte JSON

```bash
curl http://localhost:8080/api/v1/files/validation-report?fileId=f123
# Respuesta: JSON structure
```

### 2.10 Reintentar archivo en ERROR

```bash
curl -X POST http://localhost:8080/api/v1/files/retry?fileId=f123

# Respuesta:
# { "message": "archivo reintentado" }
```

### 2.11 Búsqueda de pólizas

```bash
curl "http://localhost:8080/api/v1/policies/search?documentNumber=123456&limit=10"

# Respuesta: Array de policies con validation_notes, status
```

### 2.12 Postman Collection

Ubicación: `/docs/postman/Busk_Seguros_API.postman_collection.json`

**Importar en Postman:**
1. Postman → Import → elegir JSON
2. Set variable `base_url = http://localhost:8080`
3. Run carpetas en orden:
   - **Setup:** bootstrap
   - **Productos:** list, match test
   - **Procesamiento:** scan, progress
   - **Reportes:** validation-xlsx, validation-json
   - **Búsqueda:** policies search

---

## 3. Pruebas E2E (Flujo Completo)

### 3.1 Setup

```bash
# Terminal 1: API
cd services/api && go run main.go

# Terminal 2: Docs
cd docs && npx docsify serve . --port 3000

# Terminal 3: Frontend
cd frontend-admin && npm install && npm run dev
# Escucha en :5173
```

### 3.2 Flujo manual

**Paso 1: Subir archivo a SFTP**
```bash
# Usar tools/sftpconnect o sftp client
sftp user@sftp.example.com
> put test-file.xlsx incoming/
```

**Paso 2: Ejecutar scan**
```bash
curl -X POST http://localhost:8080/api/v1/process/scan
```

**Paso 3: Monitorear en dashboard**
- Abrir http://localhost:5173
- Ver progreso en tiempo real
- Verificar lista de archivos

**Paso 4: Verificar en BD**
```bash
mysql -uroot -proot busk

mysql> SELECT COUNT(*) FROM policies WHERE file_id = 'f123';
+----------+
| COUNT(*) |
| 1250     |
+----------+

mysql> SELECT * FROM processed_files WHERE id = 'f123'\G
```

**Paso 5: Descargar reporte**
- Dashboard → Archivos → Click en f123
- Descargar Excel (cliente)
- Verificar sheets: Datos archivo, Espejo

**Paso 6: Verificar email**
- Check inbox para `SENDGRID_ERROR_TO_EMAILS`
- Email debe contener:
  - Nombre archivo
  - Estado (PROCESSED/ERROR)
  - Resumen de pólizas
  - Adjunto: reporte XLSX (si ERROR)

---

## 4. Frontend Testing (Manual)

### 4.1 No automatizado (React 19 sin Jest)

**Stack:**
- React 19 + Vite
- TypeScript
- ESLint (lint solamente, no tests)

**Para correr:**
```bash
cd frontend-admin
npm install
npm run dev
# Abre http://localhost:5173
```

### 4.2 Pruebas manuales

**Funcionalidad 1: Dashboard**
- [ ] Carga sin errores
- [ ] Muestra archivos procesados
- [ ] Progreso actualiza en tiempo real (polling cada 2s)
- [ ] Clicks en archivo abre detalles

**Funcionalidad 2: Descarga reportes**
- [ ] Click "Descargar Excel" descarga archivo
- [ ] Abre en Excel/Numbers sin corrupción
- [ ] Sheets correctos: Datos archivo, Espejo

**Funcionalidad 3: Búsqueda pólizas**
- [ ] Search by documento ID
- [ ] Muestra lista con notas
- [ ] Paginación funciona

**Funcionalidad 4: Responsive**
- [ ] Mobile (375px): menú colapsible
- [ ] Tablet (768px): 2 columnas
- [ ] Desktop (1024px+): 3 columnas

### 4.3 ESLint

```bash
npm run lint

# Esperado:
# ✓ No errors
```

---

## 5. Troubleshooting

### Error: "database connection refused"

**Solución:**
```bash
mysql -uroot -proot -e "CREATE DATABASE IF NOT EXISTS busk;"
go test ./...
```

### Error: "go: module not found"

```bash
cd services/api
go mod download
go mod tidy
```

### Test timeout

```bash
go test -timeout 5m ./...
```

### Memory leak en tests

```bash
go test -memprofile=mem.out ./...
go tool pprof -http=:8081 mem.out
```

---

## 6. Resumen Ejecución

| Tipo | Comando | Tiempo | Requisitos |
|------|---------|--------|-----------|
| **Unitarias** | `go test ./...` | 2-3s | Go 1.23 |
| **Integración** | Postman collection | Manual | API + MySQL |
| **E2E** | Browser http://localhost:5173 | Manual | API + Frontend + SFTP |
| **Lint** | `npm run lint` (frontend) | 1s | npm |
| **Coverage** | `go test -cover ./...` | 2-3s | Go 1.23 |

---

**Versión:** 1.0  
**Última actualización:** 2026-08-04
