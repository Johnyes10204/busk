# Runbook API Paso a Paso

Este documento describe el flujo operativo completo del API de Busk Seguros:

- Inicio y verificación del API
- Creación de productos nuevos
- Actualización de productos existentes
- Parametrización de primas permitidas
- Flujo de procesamiento SFTP
- Seguimiento, consulta, búsqueda y descarga de archivos

> Referencia operativa recomendada: colección Postman `docs/postman/Busk-Seguros-API.postman_collection.json` (carpeta `00 - Flujo E2E Paso a Paso`).

---

## 0) Prerrequisitos

- API levantada (`services/api`).
- Base de datos MySQL disponible y variable `MYSQL_DSN` correcta.
- Variables SFTP configuradas (`SFTP_HOST`, `SFTP_PORT`, `SFTP_USER`, `SFTP_PASSWORD`, `SFTP_REMOTE_DIR`).
- (Opcional) Carpeta de archivado local: `FILES_ARCHIVE_DIR` (si no se define, usa `./data/files-archive`).

---

## 1) Validar salud del API

### Request

`GET /api/v1/health`

### Response esperada (200)

```json
{
  "status": "ok",
  "time": "2026-04-24T16:00:00Z"
}
```

### Punto importante

- Si falla este endpoint, no continuar con el flujo.

---

## 2) Cargar seed inicial (productos + parámetros base)

### Request

`POST /api/v1/bootstrap/sample-products`

### Response esperada (201)

```json
{
  "status": "seeded"
}
```

### Qué deja configurado la seed

- Productos MAPFRE/BOLIVAR (incluyendo `mapfre_stock` con prefijo `STOCK_MAPFRE`).
- Primas permitidas por producto (`product_allowed_premiums`).
- Parámetros globales y por producto para reglas (`global_rule_params`, `product_rule_params`).

---

## 3) Listar productos configurados

### Request

`GET /api/v1/products`

### Response esperada (200)

Lista de productos con:

- `id`, `code`, `insurer`, `file_prefix`
- `mappings[]`
- `rules[]`

### Punto importante

- El motor identifica producto por **contención** del `file_prefix` en el nombre del archivo.

---

## 4) Gestionar un producto nuevo (onboarding)

## 4.1 Definir mappings (campos canónicos)

Mínimos obligatorios generales:

- `document_number`
- `credit_number`
- `monthly_premium`

Adicionales obligatorios según aseguradora:

- MAPFRE: `birth_date`, `coverage_start_date`, `coverage_end_date` (y `initial_term_months` para `MAPFRE_VIDA`)
- BOLIVAR: `birth_date`, `initial_debt_amount`, `rate_percent`, `loan_award_date`, `loan_due_date_current`

> Aunque no mapees todos los campos del Excel, el procesador guarda toda la fila en `raw_data_json`.

## 4.2 Crear/actualizar producto

### Request

`POST /api/v1/products`

Ejemplo:

```json
{
  "id": "mapfre_vida_custom",
  "code": "MAPFRE_VIDA",
  "insurer": "MAPFRE",
  "file_prefix": "INCLUSION-VIDA-MAPFRE",
  "sheet_name": "Hoja1",
  "header_row": 1,
  "mappings": [
    { "canonical_field": "document_number", "source_header": "IDENTIFICACIONAFILIADO", "required": true },
    { "canonical_field": "birth_date", "source_header": "FECHANACIMIENTO", "required": true },
    { "canonical_field": "monthly_premium", "source_header": "PRIMAMENSUALPERIODO", "required": true },
    { "canonical_field": "credit_number", "source_header": "NUMEROPRESTAMO", "required": true },
    { "canonical_field": "coverage_start_date", "source_header": "FECHA INICIO DE VIGENCIA", "required": true },
    { "canonical_field": "coverage_end_date", "source_header": "FECHAFINVIGENCIADERIESGO REAL", "required": true },
    { "canonical_field": "initial_term_months", "source_header": "PLAZO INICIAL", "required": true }
  ],
  "rules": [
    { "type": "required_not_empty", "field": "document_number", "params": {} },
    { "type": "required_not_empty", "field": "birth_date", "params": {} },
    { "type": "required_not_empty", "field": "credit_number", "params": {} },
    { "type": "number_gte", "field": "monthly_premium", "params": { "min": 0 } },
    { "type": "freeze_on_zero_premium", "field": "monthly_premium", "params": {} }
  ]
}
```

### Response esperada (201)

Devuelve el producto upsertado.

---

## 5) Parametrizar primas permitidas del producto

### 5.1 Consultar catálogo actual

`GET /api/v1/products/allowed-premiums?product_id={{productId}}`

### 5.2 Reemplazar catálogo completo

`PUT /api/v1/products/allowed-premiums`

```json
{
  "product_id": "mapfre_acc_men",
  "premiums": [7800, 7410, 10600, 10070]
}
```

### 5.3 Agregar una prima

`POST /api/v1/products/allowed-premiums`

```json
{
  "product_id": "mapfre_acc_men",
  "premium": 12000
}
```

### 5.4 Eliminar una prima

`DELETE /api/v1/products/allowed-premiums?product_id=mapfre_acc_men&premium=12000`

### 5.5 Limpiar todas

`DELETE /api/v1/products/allowed-premiums?product_id=mapfre_acc_men`

### Punto importante

- Las primas permitidas se aplican en validación MAPFRE desde BD (si hay catálogo cargado).

---

## 6) Procesar archivos SFTP

### 6.1 Encolar scan

`POST /api/v1/process/scan`

Response típica:

```json
{
  "status": "queued",
  "enqueued": 2,
  "message": "archivos escaneados y encolados para procesamiento asíncrono con workers"
}
```

### 6.2 Monitorear progreso

`GET /api/v1/process/progress`

### Puntos importantes

- El procesamiento corre con workers (por defecto 2, configurable con `PROCESSOR_WORKERS`).
- Para archivos STOCK:
  - se aplica lógica de cancelación de pólizas faltantes (`CANCELLED`) por `credit_number`.
- Si una cancelación de faltantes falla, la carga principal puede continuar con advertencia.

---

## 7) Consultar resultados de procesamiento

### 7.1 Listado de archivos

`GET /api/v1/files`

Campos relevantes:

- `status`: `PROCESSED`, `SKIPPED`, `ERROR`, etc.
- `file_hash`: SHA-256 del contenido
- `processed_path`: ruta remota final en SFTP (`PROCESSED/` o `ERROR/`)
- `archive_path`: ruta local del archivo archivado para descarga

### 7.2 Resumen de calidad por archivo

`GET /api/v1/files/summary?file_id={{fileId}}`

Incluye conteos por estado de póliza:

- `active_count`
- `frozen_count`
- `manual_review_count`
- `cancelled_count`

### 7.3 Descargar archivo procesado

`GET /api/v1/files/download?file_id={{fileId}}`

### Punto importante

- Si `archive_path` no existe o está vacío, la descarga devuelve `404`.

---

## 8) Consultar pólizas

### 8.1 Por producto (opcional status/limit)

`GET /api/v1/policies?product_id={{productId}}&status={{policyStatus}}&limit={{limit}}`

### 8.2 Búsqueda paginada por documento/crédito

`GET /api/v1/policies/search?...`

Parámetros:

- al menos uno: `document_number` o `credit_number`
- opcional: `product_id`
- paginación: `page` (>=1), `page_size` (>=1, max 200)

Respuesta incluye:

- `total`
- `total_pages`
- `has_next_page`
- `has_previous_page`
- `items`

### Punto importante

- `items` incluye `raw_data` (objeto), además de `raw_data_json` string y `validation_notes`.

---

## 9) Actualizar un producto existente

Flujo recomendado:

1. `GET /api/v1/products` para traer estado actual.
2. `POST /api/v1/products` con mismo `id` y cambios en `mappings/rules/prefix`.
3. Ajustar primas permitidas en `allowed-premiums` (PUT/POST/DELETE).
4. Ejecutar un scan controlado y validar `files`, `summary` y `policies`.

---

## 10) Checklist operativo rápido (producción)

1. `GET /health` OK
2. `POST /bootstrap/sample-products` (si aplica despliegue inicial o resembrado)
3. `GET /products` validado
4. `GET /products/allowed-premiums` validado por producto
5. `POST /process/scan`
6. `GET /process/progress` hasta fin
7. `GET /files` + `GET /files/summary`
8. `GET /policies/search` (muestreo)
9. `GET /files/download` (muestra)

---

## 11) Errores frecuentes y diagnóstico

- **No encuentra producto para archivo**
  - Revisar `file_prefix` del producto y nombre real del archivo en SFTP.
  - Recordar: la búsqueda es por contención del prefijo en el nombre.

- **Archivo en ERROR por columnas**
  - Verificar encabezados exactos del Excel contra `mappings.source_header`.
  - Confirmar `header_row` correcto.

- **Pólizas no permitidas por prima**
  - Revisar catálogo en `allowed-premiums` del producto.

- **No descarga archivo**
  - Revisar `archive_path` en `/files`.
  - Validar existencia física y permisos de la carpeta `FILES_ARCHIVE_DIR`.

---

## 12) Convenciones importantes

- El procesador usa siempre la **primera hoja** del Excel.
- El hash del archivo se calcula sobre el **contenido completo**.
- Si se repite hash ya procesado para el producto, se marca `SKIPPED`.
- Se guarda la fila completa en `raw_data_json`, incluyendo columnas no canónicas.

