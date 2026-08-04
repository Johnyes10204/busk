# Casos de Uso — Busk Seguros

## Descripción

Este documento detalla 8 escenarios reales de procesamiento en Busk Seguros, ilustrando cómo el sistema maneja archivos exitosos, fallidos, duplicados, y excepciones.

---

## Caso 1: Procesamiento Exitoso — Stock MAPFRE (10k pólizas)

### Contexto
- **Archivo**: `STOCK_JUNIO_2026_MAPFRE.xlsx`
- **Tamaño**: 10,000 filas + header
- **Formato**: MAPFRE Vida Voluntario (identificado por substring `STOCK`)
- **Contenido esperado**: Pólizas activas de referencia
- **Comportamiento**: Importar y deduplicar contra stock anterior

### Pasos

**1. Usuario inicia escaneo**
```bash
POST /api/v1/process/scan
Response: {
  "queued": 1,
  "files": [
    {
      "id": "file_1724079825000000001",
      "filename": "STOCK_JUNIO_2026_MAPFRE.xlsx",
      "status": "QUEUED",
      "processed_files_id": 1001
    }
  ]
}
```

**2. Worker asignado**
- Descarga archivo de SFTP
- Extrae filename → identifica como MAPFRE STOCK (substring `STOCK`)
- Obtiene `product_format.id = 3` (MAPFRE Vida STOCK)

**3. Parseo (excelize)**
- Lee 10,000 filas
- Mapea: Col A=ID, Col B=Nombre, Col C=Fecha nac, Col D=Plan, Col E=Prima, Col F=Fecha inicio
- Construye 10,000 structs Policy

**4. Validación: Todas las filas OK**
- DNIs válidos
- Nombres presentes
- Edades 18-75
- Planes en catálogo
- Primas coinciden
- Fechas válidas
- **Resultado**: No hay bloqueadores

**5. INSERT**
```sql
BEGIN TRANSACTION;
INSERT INTO policies (
  identification_number, policyholder_name, birthdate,
  plan, prime_annual, coverage_start_date,
  product_format_id, policy_status, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, 'ACTIVE', NOW())
  ... 10,000 inserts ...
COMMIT;
```

**6. Post-insert: Deduplicación de stock**
```go
CancelMissingStockPolicies() {
  // Obtén pólizas nuevas: (DNI, plan) tuples
  newStockSet := {
    ("12345678", "V3-50K"),
    ("87654321", "V4-100K"),
    ... 10,000 pares ...
  }

  // Busca pólizas ACTIVE en BD
  previousActive := db.Query(
    "SELECT identification_number, plan FROM policies WHERE policy_status = 'ACTIVE'"
  )

  // Diferencia: qué estaba antes pero ya no está
  toCancel := previousActive - newStockSet
  // Ej: ("11111111", "V3-50K") estaba antes, no está en nuevo stock

  // Marca como CANCELLED
  for each pair in toCancel {
    UPDATE policies
    SET policy_status = 'CANCELLED', cancellation_reason = 'Ausente en stock'
    WHERE identification_number = ? AND plan = ?
  }
}
```

**7. Generación de reportes**
```json
{
  "filename": "STOCK_JUNIO_2026_MAPFRE.xlsx",
  "status": "PROCESSED",
  "summary": {
    "total_rows": 10000,
    "inserted": 10000,
    "cancelled_from_previous_stock": 345,
    "blocking_issues": 0,
    "informative_issues": 0
  },
  "timestamp": "2026-08-04T14:23:45Z"
}
```

**8. Email de confirmación**
```
To: operador@busk.com
Subject: PROCESADO: STOCK_JUNIO_2026_MAPFRE.xlsx

Archivo procesado exitosamente.
- Pólizas nuevas insertadas: 10,000
- Pólizas del stock anterior canceladas: 345
- Estado: ACTIVE (10,000)
- Reporte: adjunto

Timestamp: 2026-08-04 14:23:45 UTC
```

### Resultado Final
- **Archivo**: `processed_files.status = PROCESSED`
- **BD**: 10,000 nuevas pólizas ACTIVE; 345 pólizas anteriores CANCELLED
- **Archivos locales**:
  - `FILES_ARCHIVE_DIR/file_1724079825000000001.xlsx`
  - `REPORTS_ARCHIVE_DIR/validation-hash1234.json`

---

## Caso 2: Procesamiento Fallido — Errores de Validación (Edad)

### Contexto
- **Archivo**: `INCLUSION_JUNIO_2026_MAPFRE_VoluntarioVida.xlsx`
- **Tamaño**: 500 filas
- **Problema**: 3 filas tienen edad > 75 (BLOQUEADOR)

### Pasos

**1. Validación: Detección de bloqueadores**

| Fila | DNI | Nombre | Edad Calculada | Issue | Bloqueador |
|---|---|---|---|---|---|
| 1 | 12345678 | Juan Pérez | 45 | — | No |
| 2 | 23456789 | María López | 78 | EDAD_FUERA_RANGO | **Sí** |
| 3 | 34567890 | Carlos Rodríguez | 52 | — | No |
| ... | ... | ... | ... | ... | ... |
| 250 | 87654321 | Ana García | 82 | EDAD_FUERA_RANGO | **Sí** |
| ... | ... | ... | ... | ... | ... |
| 500 | 99999999 | Roberto Martínez | 76 | EDAD_FUERA_RANGO | **Sí** |

**2. Gate: ¿Hay bloqueadores?**
```
policesRowSetHasBlockingIssues(allPolicies) 
  → 3 filas con EDAD_FUERA_RANGO
  → true (BLOQUEAR)
```

**3. Decisión: ERROR de archivo**
```go
if policiesRowSetHasBlockingIssues {
    processedFile.Status = "ERROR"
    // NO SE EJECUTA:
    // - INSERT
    // - CancelMissingStockPolicies()
    // - applyMapfreCancellationsToStock()
    
    // SÍ SE EJECUTA:
    // - generateValidationReports()
    // - sendGridNotification()
}
```

**4. Generación de reportes**
```json
{
  "filename": "INCLUSION_JUNIO_2026_MAPFRE_VoluntarioVida.xlsx",
  "status": "ERROR",
  "summary": {
    "total_rows": 500,
    "inserted_policies": 0,
    "blocking_issues_count": 3,
    "informative_issues_count": 0
  },
  "blocking_issues": [
    {
      "row": 2,
      "identification_number": "23456789",
      "policyholder_name": "María López",
      "birthdate": "15/06/1944",
      "age": 82,
      "issue_code": "EDAD_FUERA_RANGO",
      "reason": "Age 82 exceeds maximum of 75"
    },
    {
      "row": 250,
      "identification_number": "87654321",
      "policyholder_name": "Ana García",
      "birthdate": "20/02/1942",
      "age": 84,
      "issue_code": "EDAD_FUERA_RANGO",
      "reason": "Age 84 exceeds maximum of 75"
    },
    {
      "row": 500,
      "identification_number": "99999999",
      "policyholder_name": "Roberto Martínez",
      "birthdate": "11/12/1949",
      "age": 76,
      "issue_code": "EDAD_FUERA_RANGO",
      "reason": "Age 76 exceeds maximum of 75"
    }
  ]
}
```

**5. Email de error (a operador)**
```
To: operador@busk.com
Subject: ERROR: INCLUSION_JUNIO_2026_MAPFRE_VoluntarioVida.xlsx

Archivo rechazado por errores de validación.

BLOQUEADORES (impiden procesamiento):
- 3 filas con EDAD_FUERA_RANGO (edad máxima: 75 años)
  • Fila 2: María López, DNI 23456789, edad 82
  • Fila 250: Ana García, DNI 87654321, edad 84
  • Fila 500: Roberto Martínez, DNI 99999999, edad 76

Acción requerida:
1. Revisar datos de edad en filas indicadas
2. Corregir archivo
3. Reintentar via /api/v1/files/retry

Reporte adjunto: validation-hash5678.xlsx
Timestamp: 2026-08-04 14:45:22 UTC
```

### Resultado Final
- **Archivo**: `processed_files.status = ERROR`
- **BD**: NO se insertó nada (0 pólizas nuevas)
- **Acción operador**: Corregir datos o contactar cliente, luego reintentar

---

## Caso 3: Procesamiento Parcial — Errores Prima ↔ Plan (MAPFRE)

### Contexto
- **Archivo**: `INCLUSION_JULIO_2026_MAPFRE_VoluntarioVida.xlsx`
- **Tamaño**: 100 filas
- **Problema**: 2 filas con prima incorrecta para plan indicado (BLOQUEADOR)

### Pasos

**1. Validación: Prima ↔ Plan**

| Fila | DNI | Plan | Prima Archivo | Prima Catálogo | Issue | Tag |
|---|---|---|---|---|---|---|
| 1 | 12345678 | V3-50K | 8,500 | 8,500 | — | — |
| 2 | 23456789 | V3-50K | 9,000 | 8,500 | Mismatch | REVISAR PRIMA (PLAN) |
| ... | ... | ... | ... | ... | ... | ... |
| 75 | 87654321 | V4-100K | 10,500 | 12,500 | Mismatch | REVISAR PRIMA (PLAN) |
| 100 | 99999999 | V3-100K | 12,500 | 12,500 | — | — |

**2. Gate: Hay bloqueadores (2 prima mismatch)**
→ Archivo entero va a ERROR

**3. Reporte de error**
```json
{
  "filename": "INCLUSION_JULIO_2026_MAPFRE_VoluntarioVida.xlsx",
  "status": "ERROR",
  "blocking_issues": [
    {
      "row": 2,
      "identification_number": "23456789",
      "plan": "V3-50K",
      "prima_file": 9000,
      "prima_catalog": 8500,
      "issue_code": "PRIMA_PLAN_MISMATCH_PRIMA",
      "issue_tag": "REVISAR PRIMA (PLAN)",
      "reason": "Plan V3-50K requires 8,500 but got 9,000"
    },
    {
      "row": 75,
      "identification_number": "87654321",
      "plan": "V4-100K",
      "prima_file": 10500,
      "prima_catalog": 12500,
      "issue_code": "PRIMA_PLAN_MISMATCH_PRIMA",
      "issue_tag": "REVISAR PRIMA (PLAN)",
      "reason": "Plan V4-100K requires 12,500 but got 10,500"
    }
  ]
}
```

**4. Email a operador**
```
To: operador@busk.com
Subject: ERROR: Prima incorrecta — INCLUSION_JULIO_2026_MAPFRE_VoluntarioVida.xlsx

BLOQUEADORES — REVISAR PRIMA (PLAN):
- Fila 2: DNI 23456789, Plan V3-50K
  Prima archivo: 9,000 | Prima catálogo: 8,500
  
- Fila 75: DNI 87654321, Plan V4-100K
  Prima archivo: 10,500 | Prima catálogo: 12,500

Acción:
1. Verificar tabla de precios oficiales de MAPFRE
2. Corregir columna prima en archivo
3. Reintentar

Timestamp: 2026-08-04 15:10:33 UTC
```

### Nota Importante: Por Qué "REVISAR PRIMA (PLAN)" y No "REVISAR PLAN"

Si la validación hubiera fallado porque el **plan no existe** (ej: plan="V7-999K"), entonces:
```json
{
  "issue_tag": "REVISAR PLAN",
  "reason": "Plan V7-999K not found in catalog"
}
```

En este caso, el operador revisa el **código de plan** mismo. En el caso 3, el operador revisa la **prima** (el código de plan es correcto, pero el precio está mal).

---

## Caso 4: Anulación Masiva — Cancelar Múltiples Pólizas de MAPFRE

### Contexto
- **Archivo**: `ANULACION_MASIVA_AGOSTO_2026_MAPFRE.xlsx`
- **Tamaño**: 50 filas
- **Contenido**: 50 pares (DNI, plan) a cancelar
- **BD previa**: Stock MAPFRE con pólizas ACTIVE

### Pasos

**1. Formato del archivo**
| DNI | Plan | Fecha Cancelación | Motivo |
|---|---|---|---|
| 12345678 | V3-50K | 31/08/2026 | Solicitud cliente |
| 23456789 | V4-100K | 15/08/2026 | Cambio de póliza |
| 34567890 | V3-50K | 10/08/2026 | Incumplimiento |
| ... | ... | ... | ... |

**2. Validación**
- Identificación: OK
- Plan: OK (en catálogo)
- Fecha cancelación: OK (≤ hoy)
- Póliza existe: busca en BD `WHERE identification_number = ? AND plan = ? AND policy_status IN ('ACTIVE', 'FROZEN')`
  - Todas las 50 encontradas ✓
  - **Resultado**: No hay bloqueadores

**3. INSERT (tabla de cancelaciones o tabla de events)**
```sql
BEGIN TRANSACTION;
INSERT INTO policy_cancellations (
  identification_number, plan, cancellation_date, reason,
  created_at
) VALUES (?, ?, ?, ?, NOW())
  ... 50 inserts ...
COMMIT;
```

**4. Post-insert: aplicar cancelaciones a stock**
```go
applyMapfreCancellationsToStock() {
  // Para cada (DNI, plan) en archivo de anulación
  for each (id, plan) in cancellations {
    UPDATE policies
    SET policy_status = 'CANCELLED', cancellation_reason = 'Anulación masiva'
    WHERE identification_number = id AND plan = plan
    AND policy_status IN ('ACTIVE', 'FROZEN')
  }
}
```

**5. Reporte**
```json
{
  "filename": "ANULACION_MASIVA_AGOSTO_2026_MAPFRE.xlsx",
  "status": "PROCESSED",
  "summary": {
    "total_rows": 50,
    "cancellations_applied": 50,
    "policies_cancelled": 50
  }
}
```

**6. Email de confirmación**
```
To: operador@busk.com
Subject: PROCESADO: Anulación Masiva MAPFRE — 50 pólizas canceladas

50 pólizas canceladas correctamente.
- Pólizas ACTIVE → CANCELLED: 50
- Motivo: Anulación masiva
- Timestamp: 2026-08-04 16:30:00 UTC

Detalles en reporte adjunto.
```

### Resultado Final
- **Archivo de cancelación**: `processed_files.status = PROCESSED`
- **BD**:
  - 50 pólizas: estado ACTIVE → CANCELLED
  - Auditoría: registro de quién/cuándo canceló

---

## Caso 5: Duplicados Detectados — Mismo DNI+Plan en Archivo

### Contexto
- **Archivo**: `INCLUSION_AGOSTO_2026_MAPFRE_VoluntarioVida.xlsx`
- **Tamaño**: 200 filas
- **Problema**: La fila 150 es duplicada (mismo DNI+plan que fila 50)

### Pasos

**1. Validación: Detección de duplicados**

| Fila | DNI | Plan | Status | Issue |
|---|---|---|---|---|
| 50 | 12345678 | V3-50K | Primera ocurrencia | — |
| ... | ... | ... | ... | ... |
| 150 | 12345678 | V3-50K | Segunda ocurrencia | DUPLICADO_EN_ARCHIVO |
| 200 | 87654321 | V4-100K | — | — |

**2. Clasificación de bloqueador**
- `DUPLICADO_EN_ARCHIVO`: **INFORMATIVO** (no bloquea)
- Razón: puede ser error de cliente, se registra como "nota" en reporte

**3. Gate: ¿Hay bloqueadores?**
- NO hay bloqueadores (duplicados son informativos)
- Procede a INSERT

**4. INSERT**
- Se insertan todas las 200 filas (sí, incluyendo duplicado)
- Base de datos tendrá 2 registros para `(12345678, V3-50K)` con diferentes timestamps

**5. Reporte**
```json
{
  "filename": "INCLUSION_AGOSTO_2026_MAPFRE_VoluntarioVida.xlsx",
  "status": "PROCESSED",
  "summary": {
    "total_rows": 200,
    "inserted_policies": 200,
    "blocking_issues": 0,
    "informative_issues": 1
  },
  "informative_issues": [
    {
      "row": 150,
      "identification_number": "12345678",
      "plan": "V3-50K",
      "issue_code": "DUPLICADO_EN_ARCHIVO",
      "issue_tag": "DUPLICADO_EN_ARCHIVO",
      "reason": "Duplicate (identification_number, plan) pair found. First occurrence: row 50"
    }
  ]
}
```

**6. Email de confirmación con nota**
```
To: operador@busk.com
Subject: PROCESADO: INCLUSION_AGOSTO_2026_MAPFRE_VoluntarioVida.xlsx
  (con 1 duplicado detectado)

Archivo procesado: 200 pólizas insertadas.

⚠️ INFORMACIÓN:
- 1 duplicado detectado (fila 150)
  DNI 12345678, Plan V3-50K (también en fila 50)
  → Se insertó ambas ocurrencias; revisar con cliente si es intencional

Reporte: adjunto
Timestamp: 2026-08-04 17:00:00 UTC
```

### Resultado Final
- **Archivo**: `processed_files.status = PROCESSED`
- **BD**: 200 pólizas insertadas (2 con mismo DNI+plan)
- **Operador nota**: Revisa con cliente si es error o intencional

---

## Caso 6: Stock Duplicado en BD — Póliza Anterior Reemplazada

### Contexto
- **BD previa**: Póliza ACTIVE para (DNI 12345678, plan V3-50K), inserida hace 2 meses
- **Nuevo stock**: Mismo archivo pero ahora DNI 12345678, plan V3-50K con prima actualizada (8,500 → 9,000)
- **Comportamiento**: ¿Se reemplaza o se duplica?

### Pasos

**1. Parseo del nuevo stock**
- Fila 10: DNI 12345678, plan V3-50K, prima 9,000 (nueva)

**2. Validación**
- Prima 9,000 ≠ catálogo (8,500) → BLOQUEADOR "REVISAR PRIMA (PLAN)"
- **Resultado**: Archivo entero → ERROR

**3. Reporte**
```json
{
  "status": "ERROR",
  "blocking_issues": [
    {
      "row": 10,
      "identification_number": "12345678",
      "plan": "V3-50K",
      "prima_file": 9000,
      "prima_catalog": 8500,
      "issue_tag": "REVISAR PRIMA (PLAN)"
    }
  ]
}
```

**Nota**: Si la prima hubiera sido correcta (8,500), entonces se procedería al INSERT y no habría duplicación porque:
- La nueva póliza reemplaza la anterior (mismo DNI+plan)
- O se crea un registro adicional si la BD permite
- Comportamiento exacto depende de schema (UNIQUE constraint, UPDATE vs INSERT)

---

## Caso 7: Pólizas Faltantes en Stock — Cancelación Automática

### Contexto
- **BD previa**:
  - Póliza 1: ACTIVE (DNI 11111111, plan V3-50K) — última vez en stock de junio
  - Póliza 2: ACTIVE (DNI 22222222, plan V4-100K) — última vez en stock de junio
  - Póliza 3: ACTIVE (DNI 33333333, plan V3-50K) — última vez en stock de junio
- **Nuevo stock** (agosto):
  - Solo contiene pólizas 2 y 3
  - Póliza 1 DESAPARECE del stock
- **Comportamiento**: Póliza 1 debe ser cancelada automáticamente

### Pasos

**1. Parseo y validación del nuevo stock**
- 2 filas (pólizas 2 y 3)
- Todas OK, no hay bloqueadores

**2. INSERT**
```sql
INSERT INTO policies (identification_number, plan, ...) VALUES
  ('22222222', 'V4-100K', ...),
  ('33333333', 'V3-50K', ...);
```

**3. Post-insert: CancelMissingStockPolicies()**
```go
// Paso 1: Obtén nuevas pólizas del stock
newStockSet := {
  ('22222222', 'V4-100K'),
  ('33333333', 'V3-50K')
}

// Paso 2: Busca pólizas ACTIVE en BD
previousActive := db.Query(
  "SELECT identification_number, plan FROM policies WHERE policy_status = 'ACTIVE'"
)
// Retorna: {(11111111, V3-50K), (22222222, V4-100K), (33333333, V3-50K)}

// Paso 3: Diferencia
toCancel := previousActive - newStockSet
// toCancel = {(11111111, V3-50K)}

// Paso 4: Cancela
for each pair in toCancel {
  UPDATE policies
  SET policy_status = 'CANCELLED',
      cancellation_reason = 'Ausente en stock'
  WHERE identification_number = '11111111' AND plan = 'V3-50K'
}
```

**4. Reporte**
```json
{
  "status": "PROCESSED",
  "summary": {
    "inserted_new": 2,
    "cancelled_from_previous_stock": 1,
    "active_after": 2  // (2 nuevas) + (3 previas - 1 cancelada) = 4? No.
  }
}
```

**5. Email**
```
To: operador@busk.com
Subject: Stock procesado — 1 póliza cancelada por ausencia

- Pólizas nuevas: 2
- Pólizas canceladas (ausentes en nuevo stock): 1
  • DNI 11111111, Plan V3-50K (activa desde junio, no aparece en agosto)

Motivo cancelación: Ausente en stock
Timestamp: 2026-08-04 18:15:00 UTC
```

### Resultado Final
- **BD**:
  - Póliza 1: ACTIVE → CANCELLED (reason: "Ausente en stock")
  - Póliza 2: ACTIVE (sin cambio)
  - Póliza 3: ACTIVE (sin cambio)

---

## Caso 8: SFTP Timeout — Manejo de Excepciones

### Contexto
- **Usuario**: inicia escaneo con `POST /api/v1/process/scan`
- **SFTP**: servidor cuelga durante conexión (timeout)

### Pasos

**1. Intento de escaneo**
```
ProcessOne(file_id)
    ↓
ScanAndEnqueue()
    ├─ Conecta a SFTP
    │   └─ ctx.WithTimeout(30 seconds)
    │   └─ Espera respuesta
    │   └─ 30 segundos pasan
    │   └─ ctx.Done() → timeout
    └─ Error: "SFTP connection timeout"
```

**2. Manejo de error**
```go
if errors.Is(err, context.DeadlineExceeded) {
    // Registra error
    log.Error("SFTP timeout after 30s", map["file_id": file_id])
    
    // Actualiza estado
    processedFile.Status = "SKIPPED"
    processedFile.ErrorMessage = "SFTP timeout: unable to connect"
    
    // Envía notificación
    SendGridErrorNotification(
      to: SENDGRID_ERROR_TO_EMAILS,
      subject: "SFTP TIMEOUT — Busk Seguros",
      body: "Could not connect to SFTP server. Manual retry needed."
    )
}
```

**3. Response a usuario**
```json
{
  "queued": 0,
  "skipped": 1,
  "errors": [
    {
      "filename": "unknown",
      "status": "SKIPPED",
      "reason": "SFTP timeout"
    }
  ]
}
```

**4. Email de error**
```
To: devops@busk.com
Subject: CRITICAL: SFTP Connection Timeout

Busk Seguros unable to connect to SFTP server.
- Timestamp: 2026-08-04 19:00:00 UTC
- Error: Connection timeout after 30 seconds
- Action: Check SFTP server status and network connectivity

All pending files remain queued. Retry manually or wait for recovery.
```

**5. Recuperación**
- Operador/DevOps verifica servidor SFTP
- Una vez recuperado, usuario puede iniciar nuevo scan ó usar `/api/v1/files/retry`

### Resultado Final
- **Estado**: Archivos en SFTP → estado desconocido
- **BD**: `processed_files` con status SKIPPED + error message
- **Acción**: Manual recovery + retry

---

## Caso 9: Validación Parcial — Mezcla de Bloqueadores e Informativos

### Contexto
- **Archivo**: `INCLUSION_MIXTO_BOLIVAR.xlsx` (Deudores ESAL)
- **Tamaño**: 100 filas
- **Problemas**:
  - Fila 25: Cuota = 0 (BLOQUEADOR: `MONTHLY_PAYMENT_INVALID`)
  - Fila 60: Plazo > 120 meses (INFORMATIVO: `REVISAR PLAZO`)
  - Fila 80: DNI duplicado dentro del archivo (INFORMATIVO: `DUPLICADO_EN_ARCHIVO`)

### Pasos

**1. Validación de todas las filas**
- Fila 25: outstanding_debt=100,000, monthly_payment=0
  → `MONTHLY_PAYMENT_INVALID` (BLOQUEADOR)
- Fila 60: outstanding_debt=150,000, monthly_payment=1,200
  → ratio = 125 > 120
  → `PLAZO_EXCEDE_MAXIMO` (INFORMATIVO, tag: `REVISAR PLAZO`)
- Fila 80: (identification=12345678, coverage_date=01/08/2026) duplica fila 15
  → `DUPLICADO_EN_ARCHIVO` (INFORMATIVO)

**2. Gate: ¿Hay bloqueadores?**
- SÍ: Fila 25 tiene `MONTHLY_PAYMENT_INVALID`
- **Decisión**: Archivo → ERROR, no persiste

**3. Reporte de error**
```json
{
  "filename": "INCLUSION_MIXTO_BOLIVAR.xlsx",
  "status": "ERROR",
  "summary": {
    "blocking_issues_count": 1,
    "informative_issues_count": 2
  },
  "blocking_issues": [
    {
      "row": 25,
      "issue_code": "MONTHLY_PAYMENT_INVALID",
      "value": 0,
      "reason": "Monthly payment must be > 0"
    }
  ],
  "informative_issues": [
    {
      "row": 60,
      "issue_code": "PLAZO_EXCEDE_MAXIMO",
      "issue_tag": "REVISAR PLAZO",
      "plazo_calculated": 125,
      "reason": "Loan term (125 months) exceeds maximum of 120"
    },
    {
      "row": 80,
      "issue_code": "DUPLICADO_EN_ARCHIVO",
      "reason": "Duplicate (identification_number, coverage_start_date)"
    }
  ]
}
```

**4. Email a operador**
```
To: operador@busk.com
Subject: ERROR: INCLUSION_MIXTO_BOLIVAR.xlsx (1 bloqueo + 2 informativos)

BLOQUEADORES:
- Fila 25: Cuota mensual = 0 (debe ser > 0)
  DNI: 12345678

Avisos (no bloquean, pero revisar):
- Fila 60: Plazo calculado 125 meses > máximo 120
  DNI: 23456789
  Deuda: 150,000 | Cuota: 1,200
  
- Fila 80: Póliza duplicada dentro del archivo
  DNI: 34567890 (también en fila 15)

Acción:
1. Corregir cuota en fila 25
2. Revisar deuda/cuota en fila 60 con cliente
3. Fila 80: verificar si es intencional o error de captura

Reintentar tras correcciones.
```

### Resultado Final
- **Archivo**: `processed_files.status = ERROR`
- **BD**: NO se insertó nada
- **Operador**: Debe corregir fila 25 (bloqueador) y revisar filas 60-80

---

## Resumen: Matriz de Escenarios

| Caso | Archivo | Problema | Bloqueador | Acción | Resultado |
|---|---|---|---|---|---|
| 1 | Stock MAPFRE 10k | — | No | Inserta + dedup | PROCESSED, 10k+345 cancelled |
| 2 | INCLUSION MAPFRE | 3 EDAD > 75 | Sí | Error + reporte | ERROR, 0 insertas |
| 3 | INCLUSION MAPFRE | 2 prima mismatch | Sí | Error + reporte | ERROR, 0 insertas |
| 4 | Anulación masiva | — | No | Inserta + aplica cancel | PROCESSED, 50 cancelled |
| 5 | INCLUSION MAPFRE | 1 duplicado en archivo | No | Inserta + registra | PROCESSED, 200 insertas |
| 6 | Stock MAPFRE | Prima incorrecta en stock | Sí | Error | ERROR, 0 insertas |
| 7 | Stock MAPFRE | 1 póliza faltante | No | Inserta + cancela ausente | PROCESSED, 1 cancelled |
| 8 | Scan SFTP | Timeout SFTP | N/A | Skip + notifica | SKIPPED, manual retry |
| 9 | INCLUSION Bolívar | Cuota=0, plazo alto, duplicado | Sí (cuota) | Error | ERROR, 0 insertas |

