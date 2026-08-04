# API Endpoints — Busk Seguros

## Base URL
```
http://localhost:8080/api/v1
```

---

## 1. Health & Bootstrap

### 1.1 GET /health

Verificar estado del servicio.

**Request**:
```bash
curl -X GET http://localhost:8080/api/v1/health
```

**Response** (200 OK):
```json
{
  "status": "ok",
  "timestamp": "2026-08-04T14:23:45Z"
}
```

---

### 1.2 POST /bootstrap/sample-products

Crear productos y formatos de ejemplo (solo para desarrollo/testing).

**Request**:
```bash
curl -X POST http://localhost:8080/api/v1/bootstrap/sample-products \
  -H "Content-Type: application/json" \
  -d '{}'
```

**Response** (200 OK):
```json
{
  "products_created": 2,
  "formats_created": 8,
  "products": [
    {
      "id": 1,
      "name": "MAPFRE",
      "active": true
    },
    {
      "id": 2,
      "name": "BOLÍVAR",
      "active": true
    }
  ],
  "formats": [
    {
      "id": 1,
      "product_id": 1,
      "name": "Vida Voluntario",
      "file_prefix": "VOLUNTARIO,5024424900103",
      "active": true,
      "priority": 1
    },
    ...
  ]
}
```

---

## 2. Products & Formats

### 2.1 GET /products

Listar todos los productos.

**Request**:
```bash
curl -X GET http://localhost:8080/api/v1/products
```

**Response** (200 OK):
```json
[
  {
    "id": 1,
    "name": "MAPFRE",
    "active": true,
    "created_at": "2026-06-01T00:00:00Z",
    "updated_at": "2026-06-01T00:00:00Z"
  },
  {
    "id": 2,
    "name": "BOLÍVAR",
    "active": true,
    "created_at": "2026-06-01T00:00:00Z",
    "updated_at": "2026-06-01T00:00:00Z"
  }
]
```

---

### 2.2 GET /product-formats

Listar todos los formatos de producto.

**Request**:
```bash
curl -X GET http://localhost:8080/api/v1/product-formats
```

**Response** (200 OK):
```json
[
  {
    "id": 1,
    "product_id": 1,
    "product_name": "MAPFRE",
    "name": "Vida Voluntario",
    "file_prefix": "VOLUNTARIO,5024424900103",
    "priority": 1,
    "active": true,
    "mappings_json": {
      "alias_mapping": {
        "DNI": "identification_number",
        "Nombre": "policyholder_name",
        ...
      }
    },
    "rules_json": {
      "required_fields": ["identification_number", "policyholder_name", ...],
      "validation_rules": [...]
    },
    "created_at": "2026-06-01T00:00:00Z",
    "updated_at": "2026-06-01T00:00:00Z"
  },
  ...
]
```

---

### 2.3 GET /product-formats/active

Listar solo formatos activos.

**Request**:
```bash
curl -X GET http://localhost:8080/api/v1/product-formats/active
```

**Response** (200 OK): Idem `/product-formats`, filtrado por `active: true`.

---

### 2.4 POST /product-formats/match-test

Probar si un nombre de archivo coincide con algún formato.

**Request**:
```bash
curl -X POST http://localhost:8080/api/v1/product-formats/match-test \
  -H "Content-Type: application/json" \
  -d '{
    "filename": "INCLUSION_JUNIO_2026_MAPFRE_VoluntarioVida.xlsx"
  }'
```

**Response** (200 OK):
```json
{
  "filename": "INCLUSION_JUNIO_2026_MAPFRE_VoluntarioVida.xlsx",
  "matched": true,
  "product_format": {
    "id": 1,
    "product_id": 1,
    "product_name": "MAPFRE",
    "name": "Vida Voluntario",
    "file_prefix": "VOLUNTARIO,5024424900103",
    "priority": 1
  }
}
```

**Response** (200 OK, no match):
```json
{
  "filename": "UNKNOWN_FILE_2026.xlsx",
  "matched": false,
  "product_format": null,
  "message": "No matching product format found"
}
```

---

### 2.5 GET /products/allowed-premiums

Listar primas permitidas por producto (para validación).

**Request**:
```bash
curl -X GET http://localhost:8080/api/v1/products/allowed-premiums
```

**Response** (200 OK):
```json
[
  {
    "id": 1,
    "product_id": 1,
    "product_name": "MAPFRE",
    "plan": "V3-50K",
    "prime_annual": 8500,
    "active": true,
    "created_at": "2026-06-01T00:00:00Z"
  },
  {
    "id": 2,
    "product_id": 1,
    "product_name": "MAPFRE",
    "plan": "V3-100K",
    "prime_annual": 12500,
    "active": true,
    "created_at": "2026-06-01T00:00:00Z"
  },
  ...
]
```

---

## 3. Processing

### 3.1 POST /process/scan

Escanear SFTP y encolar archivos para procesamiento.

**Request**:
```bash
curl -X POST http://localhost:8080/api/v1/process/scan \
  -H "Content-Type: application/json" \
  -d '{}'
```

**Response** (200 OK):
```json
{
  "timestamp": "2026-08-04T14:23:45Z",
  "queued": 3,
  "skipped": 1,
  "files": [
    {
      "id": "file_1724079825000000001",
      "filename": "STOCK_JUNIO_2026_MAPFRE.xlsx",
      "status": "QUEUED",
      "processed_files_id": 1001,
      "matched_product_format": {
        "id": 3,
        "product_id": 1,
        "name": "MAPFRE STOCK"
      }
    },
    {
      "id": "file_1724079825000000002",
      "filename": "INCLUSION_JUNIO_2026_MAPFRE_VoluntarioVida.xlsx",
      "status": "QUEUED",
      "processed_files_id": 1002,
      "matched_product_format": {
        "id": 1,
        "product_id": 1,
        "name": "MAPFRE Vida Voluntario"
      }
    },
    ...
  ],
  "skipped_files": [
    {
      "filename": "README.txt",
      "reason": "Unsupported file type (only .xlsx, .xls, .csv)"
    }
  ],
  "errors": []
}
```

**Response** (500 Internal Server Error):
```json
{
  "error": "SFTP connection failed",
  "details": "unable to connect to SFTP server after 30s timeout",
  "timestamp": "2026-08-04T14:23:45Z"
}
```

---

### 3.2 GET /process/progress

Obtener progreso actual de procesamiento de archivos.

**Request**:
```bash
curl -X GET http://localhost:8080/api/v1/process/progress
```

**Response** (200 OK):
```json
{
  "timestamp": "2026-08-04T14:25:12Z",
  "active_workers": 2,
  "total_queued": 5,
  "currently_processing": [
    {
      "file_id": "file_1724079825000000001",
      "filename": "STOCK_JUNIO_2026_MAPFRE.xlsx",
      "product_format": "MAPFRE STOCK",
      "status": "PROCESSING",
      "progress": {
        "current_row": 5234,
        "total_rows": 10000,
        "percentage": 52.34,
        "elapsed_seconds": 180,
        "estimated_remaining_seconds": 165
      }
    },
    {
      "file_id": "file_1724079825000000002",
      "filename": "INCLUSION_JUNIO_2026_MAPFRE_VoluntarioVida.xlsx",
      "product_format": "MAPFRE Vida Voluntario",
      "status": "QUEUED",
      "progress": null
    }
  ]
}
```

---

## 4. Files

### 4.1 GET /files

Listar todos los archivos procesados.

**Query Parameters**:
- `status` (string, opcional): filtrar por estado (PENDING, QUEUED, PROCESSING, PROCESSED, ERROR, SKIPPED)
- `limit` (int, default 50): cantidad máxima de resultados
- `offset` (int, default 0): para paginación

**Request**:
```bash
curl -X GET http://localhost:8080/api/v1/files?status=ERROR&limit=10&offset=0
```

**Response** (200 OK):
```json
{
  "total_count": 145,
  "limit": 10,
  "offset": 0,
  "items": [
    {
      "id": 1001,
      "filename": "INCLUSION_JUNIO_2026_MAPFRE_VoluntarioVida.xlsx",
      "file_hash": "abc123def456...",
      "status": "ERROR",
      "product_format_id": 1,
      "product_format_name": "MAPFRE Vida Voluntario",
      "file_size_bytes": 52000,
      "uploaded_at": "2026-08-04T14:20:00Z",
      "processed_at": "2026-08-04T14:45:22Z",
      "processing_duration_seconds": 1522,
      "total_rows": 10000,
      "blocking_issues_count": 2,
      "informative_issues_count": 0,
      "inserted_policies_count": 0,
      "error_message": "Validation failed: 2 blocking issues detected"
    },
    ...
  ]
}
```

---

### 4.2 POST /files/retry

Reintentar procesamiento de un archivo que falló (status ERROR o SKIPPED).

**Request**:
```bash
curl -X POST http://localhost:8080/api/v1/files/retry \
  -H "Content-Type: application/json" \
  -d '{
    "file_id": 1001
  }'
```

**Response** (200 OK):
```json
{
  "file_id": 1001,
  "filename": "INCLUSION_JUNIO_2026_MAPFRE_VoluntarioVida.xlsx",
  "status": "QUEUED",
  "message": "File requeued for processing"
}
```

**Response** (400 Bad Request):
```json
{
  "error": "Invalid file status",
  "message": "File with ID 1001 has status PROCESSED; only ERROR and SKIPPED files can be retried"
}
```

---

### 4.3 GET /files/summary

Resumen estadístico de archivos procesados.

**Request**:
```bash
curl -X GET http://localhost:8080/api/v1/files/summary
```

**Response** (200 OK):
```json
{
  "timestamp": "2026-08-04T14:30:00Z",
  "summary_by_status": {
    "PENDING": {
      "count": 0,
      "files": []
    },
    "QUEUED": {
      "count": 2,
      "files": ["file_1", "file_2"]
    },
    "PROCESSING": {
      "count": 1,
      "files": ["file_3"]
    },
    "PROCESSED": {
      "count": 15,
      "total_policies_inserted": 45000
    },
    "ERROR": {
      "count": 3,
      "files": ["file_4", "file_5", "file_6"]
    },
    "SKIPPED": {
      "count": 1,
      "files": ["file_7"]
    }
  },
  "total_files": 22,
  "overall_statistics": {
    "total_policies_inserted": 45000,
    "total_processing_errors": 3,
    "success_rate": 0.862,
    "average_processing_time_seconds": 1250
  }
}
```

---

### 4.4 GET /files/validation-report

Obtener reporte de validación JSON de un archivo.

**Query Parameters**:
- `file_id` (int, requerido): ID del archivo

**Request**:
```bash
curl -X GET http://localhost:8080/api/v1/files/validation-report?file_id=1001 \
  -H "Accept: application/json"
```

**Response** (200 OK):
```json
{
  "filename": "INCLUSION_JUNIO_2026_MAPFRE_VoluntarioVida.xlsx",
  "file_hash": "abc123def456...",
  "processed_at": "2026-08-04T14:45:22Z",
  "status": "ERROR",
  "product_format_id": 1,
  "product_format_name": "MAPFRE Vida Voluntario",
  "summary": {
    "total_rows": 10000,
    "inserted_policies": 0,
    "blocking_issues_count": 2,
    "informative_issues_count": 0
  },
  "blocking_issues": [
    {
      "row": 523,
      "identification_number": "12345678",
      "policyholder_name": "Juan Pérez",
      "birthdate": "01/01/1950",
      "age": 76,
      "issue_code": "EDAD_FUERA_RANGO",
      "reason": "Age 76 exceeds maximum of 75"
    },
    {
      "row": 8234,
      "identification_number": "87654321",
      "policyholder_name": "Ana García",
      "plan": "V3-50K",
      "prime_annual": 10000,
      "issue_code": "PRIMA_PLAN_MISMATCH_PRIMA",
      "issue_tag": "REVISAR PRIMA (PLAN)",
      "reason": "Plan V3-50K requires 8500 but got 10000"
    }
  ],
  "informative_issues": []
}
```

---

### 4.5 GET /files/validation-csv

Descargar reporte de validación en formato CSV.

**Query Parameters**:
- `file_id` (int, requerido): ID del archivo

**Request**:
```bash
curl -X GET http://localhost:8080/api/v1/files/validation-csv?file_id=1001 \
  -H "Accept: text/csv" \
  --output validation-1001.csv
```

**Response** (200 OK, Content-Type: text/csv):
```csv
row,identification_number,policyholder_name,issue_code,issue_tag,value,reason,blocking
523,12345678,Juan Pérez,EDAD_FUERA_RANGO,,76,Age 76 exceeds maximum of 75,true
8234,87654321,Ana García,PRIMA_PLAN_MISMATCH_PRIMA,REVISAR PRIMA (PLAN),10000,Plan V3-50K requires 8500 but got 10000,true
```

---

### 4.6 GET /files/validation-xlsx

Descargar reporte de validación en formato Excel.

**Query Parameters**:
- `file_id` (int, requerido): ID del archivo

**Request**:
```bash
curl -X GET http://localhost:8080/api/v1/files/validation-xlsx?file_id=1001 \
  --output validation-1001.xlsx
```

**Response** (200 OK, Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet)

Archivo Excel con hojas:
1. **Resumen**: Metadatos del archivo, counts de issues
2. **Bloqueadores**: Tabla de filas con issues bloqueadores
3. **Informativos**: Tabla de filas con issues informativos (si hay)

---

### 4.7 GET /files/download

Descargar archivo original procesado.

**Query Parameters**:
- `file_id` (int, requerido): ID del archivo

**Request**:
```bash
curl -X GET http://localhost:8080/api/v1/files/download?file_id=1001 \
  --output original-file.xlsx
```

**Response** (200 OK, Content-Type: application/octet-stream)

Archivo XLSX/XLS/CSV original (descargado del SFTP durante procesamiento).

---

## 5. Policies

### 5.1 GET /policies

Listar pólizas.

**Query Parameters**:
- `status` (string, opcional): filtrar por estado (ACTIVE, FROZEN, MANUAL_REVIEW, CANCELLED)
- `product_format_id` (int, opcional): filtrar por formato de producto
- `identification_number` (string, opcional): buscar por DNI (exact match)
- `limit` (int, default 50): cantidad máxima de resultados
- `offset` (int, default 0): para paginación

**Request**:
```bash
curl -X GET http://localhost:8080/api/v1/policies?status=ACTIVE&product_format_id=1&limit=20&offset=0
```

**Response** (200 OK):
```json
{
  "total_count": 45000,
  "limit": 20,
  "offset": 0,
  "items": [
    {
      "id": 50001,
      "identification_number": "12345678",
      "policyholder_name": "Juan Pérez",
      "birthdate": "15/03/1985",
      "plan": "V3-50K",
      "prime_annual": 8500,
      "coverage_start_date": "01/06/2026",
      "status": "ACTIVE",
      "product_format_id": 1,
      "product_format_name": "MAPFRE Vida Voluntario",
      "processed_file_id": 1001,
      "created_at": "2026-06-01T10:15:00Z",
      "updated_at": "2026-06-01T10:15:00Z",
      "notes": null
    },
    ...
  ]
}
```

---

### 5.2 POST /policies/search

Búsqueda avanzada de pólizas.

**Request**:
```bash
curl -X POST http://localhost:8080/api/v1/policies/search \
  -H "Content-Type: application/json" \
  -d '{
    "identification_number": "12345678",
    "plan": "V3-50K",
    "status": "ACTIVE",
    "coverage_start_date_from": "2026-01-01",
    "coverage_start_date_to": "2026-12-31",
    "limit": 100,
    "offset": 0
  }'
```

**Response** (200 OK):
```json
{
  "total_count": 5,
  "limit": 100,
  "offset": 0,
  "query": {
    "identification_number": "12345678",
    "plan": "V3-50K",
    "status": "ACTIVE",
    "coverage_start_date_from": "2026-01-01",
    "coverage_start_date_to": "2026-12-31"
  },
  "items": [
    {
      "id": 50001,
      "identification_number": "12345678",
      "policyholder_name": "Juan Pérez",
      "plan": "V3-50K",
      "status": "ACTIVE",
      "coverage_start_date": "01/06/2026",
      "prime_annual": 8500,
      "created_at": "2026-06-01T10:15:00Z"
    },
    ...
  ]
}
```

---

## 6. Error Responses

Todos los endpoints pueden retornar errores con este formato:

### 4xx Client Error

```json
{
  "error": "Bad Request",
  "message": "Invalid query parameter: limit must be > 0",
  "code": "INVALID_QUERY_PARAM",
  "timestamp": "2026-08-04T14:30:00Z"
}
```

### 5xx Server Error

```json
{
  "error": "Internal Server Error",
  "message": "Database connection lost",
  "code": "DB_ERROR",
  "timestamp": "2026-08-04T14:30:00Z"
}
```

---

## 7. HTTP Status Codes

| Código | Significado |
|--------|-----------|
| 200 | OK — Operación exitosa |
| 201 | Created — Recurso creado (no aplicable en esta API) |
| 400 | Bad Request — Parámetros inválidos |
| 401 | Unauthorized — No autenticado (futuro) |
| 403 | Forbidden — No autorizado (futuro) |
| 404 | Not Found — Recurso no encontrado |
| 409 | Conflict — Conflicto (ej: archivo duplicado) |
| 500 | Internal Server Error — Error del servidor |
| 503 | Service Unavailable — Servicio no disponible (ej: SFTP caído) |

---

## 8. Autenticación y Autorización

**Estado actual**: SIN autenticación (API abierta, solo para desarrollo).

**Futuro**: Se implementará autenticación basada en tokens JWT.

---

## 9. Rate Limiting

**Estado actual**: SIN límite de rate (API abierta).

**Futuro**: Se implementará límite de 100 req/min por cliente.

---

## 10. Ejemplos de Uso Completo

### Ejemplo 1: Procesar Archivo Completo

```bash
# 1. Escanear SFTP
curl -X POST http://localhost:8080/api/v1/process/scan

# 2. Esperar progreso
for i in {1..30}; do
  curl -X GET http://localhost:8080/api/v1/process/progress
  sleep 2
done

# 3. Obtener resumen
curl -X GET http://localhost:8080/api/v1/files/summary

# 4. Ver archivo procesado
curl -X GET http://localhost:8080/api/v1/files?status=PROCESSED&limit=1

# 5. Descargar reporte
curl -X GET http://localhost:8080/api/v1/files/validation-report?file_id=1001

# 6. Buscar pólizas insertadas
curl -X POST http://localhost:8080/api/v1/policies/search \
  -H "Content-Type: application/json" \
  -d '{"status": "ACTIVE", "product_format_id": 1, "limit": 10}'
```

### Ejemplo 2: Reintentar Archivo Fallido

```bash
# 1. Listar archivos con error
curl -X GET http://localhost:8080/api/v1/files?status=ERROR&limit=1

# 2. Obtener ID del archivo (ej: 1001)
# 3. Reintentar
curl -X POST http://localhost:8080/api/v1/files/retry \
  -H "Content-Type: application/json" \
  -d '{"file_id": 1001}'

# 4. Monitorear progreso
curl -X GET http://localhost:8080/api/v1/process/progress

# 5. Verificar resultado
curl -X GET http://localhost:8080/api/v1/files/validation-report?file_id=1001
```

### Ejemplo 3: Búsqueda de Pólizas por Cliente

```bash
# Buscar todas las pólizas activas de un cliente
curl -X POST http://localhost:8080/api/v1/policies/search \
  -H "Content-Type: application/json" \
  -d '{
    "identification_number": "12345678",
    "status": "ACTIVE"
  }'
```

