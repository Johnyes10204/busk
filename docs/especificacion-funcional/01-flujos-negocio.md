# Flujos de Negocio — Busk Seguros

## Descripción General

Busk Seguros es un motor de ingesta de archivos (XLSX, XLS, CSV) que:
1. Descarga archivos de un servidor SFTP
2. Valida cada fila contra reglas de negocio específicas por producto
3. Persiste pólizas en MySQL si la validación es exitosa
4. Genera reportes de validación y notificaciones vía email
5. Archiva archivos procesados y reportes localmente

El ciclo completo es **síncrono a nivel de archivo** (no se inicia el siguiente hasta que el actual esté completamente procesado), pero **paralelo a nivel de CPU** mediante un worker pool configurable.

---

## 1. Flujo General de Procesamiento

### Fase 1: Escaneo y Encolamiento
```
POST /api/v1/process/scan
    ↓
ScanAndEnqueue()
    ├─ Conecta a SFTP
    ├─ Lista archivos en raíz
    ├─ Filtra extensiones (.xlsx, .xls, .csv)
    ├─ Ordena por prioridad:
    │  1. Archivos STOCK
    │  2. Archivos INCLUSION
    │  3. Resto
    └─ Encola en canal para workers
```

**Salida**: Un archivo de proceso con estado `QUEUED` por cada archivo encontrado.

### Fase 2: Identificación de Producto/Formato
Para cada archivo encolado:
```
processOne(filename)
    ↓
FindProductFormatCandidates(filename)
    ├─ Búsqueda case-insensitive por substring en file_prefix
    ├─ Ordenado por:
    │  1. Longitud de prefix (más específico primero)
    │  2. priority (ascendente)
    │  3. created_at (más reciente primero)
    └─ Retorna lista de candidatos (generalmente 1)
```

**Ejemplo de matching**:
- Filename: `INCLUSION_JUNIO_MAPFRE_VoluntarioVida.xlsx`
- Búsqueda: `%VOLUNTARIO%` → Coincide con producto MAPFRE Vida Voluntario
- Resultado: `product_format.id = 1`, `mappings_json`, `rules_json`

### Fase 3: Parseo y Mapeo
```
ParseSpreadsheet(filename)
    ├─ Detecta formato (XLSX con excelize, XLS con xls, CSV con encoding detection)
    ├─ Lee filas (saltando header)
    └─ Para cada fila:
        ├─ Mapea columnas → campos canónicos según mappings_json
        ├─ Resuelve aliases (ej: "DNI" → "identification_number")
        ├─ Limpia espacios/casos
        └─ Construye Policy struct
```

### Fase 4: Validación y Control de Flujo
```
ValidateAndProcess(policies[], product_format)
    ├─ Ejecuta validaciones por fila (reglas_json + lógica específica)
    ├─ Marca issues por fila:
    │  ├─ BLOQUEADORES: impiden persistencia de TODO el archivo
    │  └─ INFORMATIVOS: se registran en report, pero archivo se persiste
    │
    └─ GATE (policiesRowSetHasBlockingIssues)
        ├─ ¿Alguna fila tiene bloqueador?
        │  ├─ SÍ → Archivo va a ERROR, solo reportes, NO inserts
        │  └─ NO → Continua
        │
        ├─ Inserta pólizas
        ├─ Aplica lógica post-insert:
        │  ├─ STOCK: CancelMissingStockPolicies()
        │  └─ ANULACION_MASIVA: applyMapfreCancellationsToStock()
        │
        └─ Genera reportes JSON + XLSX
```

### Fase 5: Archivo y Notificación
```
AfterProcessing(filename)
    ├─ Descarga archivo del SFTP a FILES_ARCHIVE_DIR
    ├─ Mueve remoto a PROCESSED/ o ERROR/
    ├─ Genera REPORTS_ARCHIVE_DIR:
    │  ├─ validation-HASH.json
    │  └─ validation-HASH.xlsx
    ├─ Actualiza processed_files.status
    └─ SendGrid:
        ├─ Email success: resumen + adjunto JSON
        └─ Email error: bloqueadores + adjunto XLSX
```

---

## 2. Flujos por Producto

### 2.1 MAPFRE — Vida Voluntario

**Identificación**:
- File prefix: `5024424900103` ó `VOLUNTARIO`

**Campos principales** (ver mapeos-columnas.md):
- `identification_number` (DNI/Cédula, obligatorio)
- `policyholder_name` (texto, obligatorio)
- `birthdate` (DD/MM/YYYY, obligatorio)
- `plan` (ej: "V3-50K", obligatorio, debe estar en catálogo)
- `prime_annual` (número, obligatorio, debe coincidir con plan)
- `coverage_start_date` (DD/MM/YYYY, obligatorio)
- `status` (ACTIVA/INACTIVA, obligatorio)

**Validaciones específicas**:
1. **Plan obligatorio e identificado** en catálogo de MAPFRE
2. **Prima-Plan**: `prime_annual` debe coincidir con `plan` (tasa oficial)
   - Si falla por prima: tag `REVISAR PRIMA (PLAN)`
   - Si falla por plan: tag `REVISAR PLAN`
3. **Fecha de cobertura**: formato DD/MM/YYYY, debe ser razonable (ej: últimos 2 años)
4. **Edad derivada**: del `birthdate`, debe ser ≥18 y ≤75
5. **Duplicados**: `identification_number` + `plan` en mismo archivo = marcar segunda ocurrencia
6. **Status válido**: ACTIVA o INACTIVA

**Tipo de archivo**: INCLUSION

**Formato procesamiento**:
- Sin deduplicación histórica: cada ingesta reemplaza anterior
- Validación de fila = validación de póliza

---

### 2.2 MAPFRE — AP Menores (Anexo 3)

**Identificación**:
- File prefix: `5024524900101` ó `ACC MEN` ó `ACCIDENTE_MENORES`

**Campos principales**:
- `identification_number` (DNI del beneficiario menor, obligatorio)
- `beneficiary_name` (texto, obligatorio)
- `birthdate` (DD/MM/YYYY, obligatorio)
- `policyholder_identification` (DNI del responsable, obligatorio)
- `policyholder_name` (texto del responsable, obligatorio)
- `plan` (ej: "AP50-MENOR", obligatorio)
- `prime_annual` (número, obligatorio)
- `coverage_start_date` (DD/MM/YYYY, obligatorio)

**Validaciones específicas**:
1. **Edad del menor**: 0 ≤ edad derivada ≤ 17
2. **Edad del responsable**: 18 ≤ edad ≤ 100
3. **Plan y prima**: idem Vida Voluntario
4. **Identificaciones únicas**: ambas DNI deben ser válidas y diferentes
5. **Duplicados**: combinación `identification_number` + `policyholder_identification` + `plan`

**Tipo de archivo**: INCLUSION

---

### 2.3 MAPFRE — AP Cáncer (Anexo 2)

**Identificación**:
- File prefix: `5024524900103` ó `CANCER` ó `CANCER_SEGURO`

**Campos principales**:
- `identification_number` (DNI, obligatorio)
- `policyholder_name` (texto, obligatorio)
- `birthdate` (DD/MM/YYYY, obligatorio)
- `plan` (ej: "CANCER-50K", obligatorio)
- `prime_annual` (número, obligatorio)
- `coverage_start_date` (DD/MM/YYYY, obligatorio)
- `exclusion_cancer_date` (DD/MM/YYYY, opcional; si existe, cáncer NO está cubierto desde esa fecha)

**Validaciones específicas**:
1. **Plan y prima**: idem Vida Voluntario
2. **Fecha de exclusión**: si existe, debe ser ≥ `coverage_start_date`
3. **Edad**: 18 ≤ edad ≤ 75

**Tipo de archivo**: INCLUSION

---

### 2.4 MAPFRE — Stock (Anexo 1)

**Identificación**:
- File prefix: `STOCK` ó `Anexo 1`

**Campos principales**:
- Idénticos a Vida Voluntario (DNI, nombre, fecha nac, plan, prima, fecha inicio)
- Generalmente sin status (todos ACTIVA por defecto)

**Validaciones específicas**:
1. Idénticas a Vida Voluntario
2. **Deduplicación temporal**: si una póliza `(identification_number, plan)` estaba ACTIVE y NO aparece en el nuevo stock → `CancelMissingStockPolicies()` la marca CANCELLED con motivo "Ausente en stock"
3. **Reemplazo**: si DNI+plan existe en BD y aparece en nuevo stock con mismos datos → no se modifica
4. **Nuevas**: se insertan

**Tipo de archivo**: STOCK (no INCLUSION)

**Lógica crítica**:
```
Antes: ACTIVE(Juan, DNI 123, V3-50K)
Stock nuevo: Solo incluye Juan con V3-100K, no V3-50K
Después: ACTIVE(Juan, V3-100K)
         CANCELLED(Juan, V3-50K, "Ausente en stock")
```

---

### 2.5 MAPFRE — Anulación Masiva

**Identificación**:
- File prefix: `ANULACION_MASIVA` ó `CANCELACION` ó `ANULACION`

**Campos principales**:
- `identification_number` (DNI, obligatorio)
- `plan` (ej: "V3-50K", obligatorio)
- `cancellation_date` (DD/MM/YYYY, fecha del efecto de cancelación)
- `cancellation_reason` (texto, opcional; almacenado en BD)

**Validaciones específicas**:
1. **Póliza debe existir**: DNI+plan en ACTIVE ó FROZEN → búsqueda en stock histórico
2. **Fecha de cancelación**: formato DD/MM/YYYY, debe ser ≤ hoy
3. **No duplicados**: si el mismo DNI+plan aparece 2+ veces, error

**Lógica post-insert**:
1. Inserta fila de "cancelación" en tabla (meta)
2. Busca todas las pólizas STOCK con mismo `(identification_number, plan)`
3. Marca esas pólizas como CANCELLED con motivo "Anulación masiva"
4. Registra en auditoría

**Tipo de archivo**: INCLUSION (pero desencadena cancellations en STOCK)

---

### 2.6 BOLÍVAR — Deudores Banco (Micro/Pyme)

**Identificación**:
- File prefix: `DEUDORES_BANCO` ó `Deudores_Banco_Bolivar`

**Variantes**: MICRO ó PYME (en nombre de archivo)

**Campos principales**:
- `identification_number` (DNI/RIF, obligatorio)
- `policyholder_name` (nombre, obligatorio)
- `outstanding_debt` (monto en Bs., obligatorio, ≥0)
- `monthly_payment` (monto en Bs., obligatorio, >0)
- `debt_currency` (VEF o USD, por defecto VEF)
- `coverage_start_date` (DD/MM/YYYY, obligatorio)
- `status` (ACTIVA/INACTIVA, obligatorio)

**Validaciones específicas**:
1. **Deuda y cuota**: ambas numéricas, positivas
2. **Ratio deuda/cuota**: `outstanding_debt / monthly_payment` ≤ 120 (plazo máx 10 años)
   - Si falla: tag `REVISAR PLAZO` (indica morosidad probable)
3. **Plazo calculado**: `REDONDEAR.MENOS(outstanding_debt / monthly_payment; 0)` meses
   - Redondeo hacia abajo (función Excel)
4. **Edad**: 18 ≤ edad ≤ 75
5. **Duplicados**: `(identification_number, coverage_start_date)` en mismo archivo

**Tipo de archivo**: INCLUSION

---

### 2.7 BOLÍVAR — Deudores ESAL (Micro/Pyme)

**Identificación**:
- File prefix: `DEUDORES_ESAL` ó `Deudores_ESAL_Bolivar`

**Variantes**: MICRO ó PYME

**Campos principales**:
- Idénticos a Deudores Banco
- Adicionalmente: `business_name` (nombre de empresa, para PYME)

**Validaciones específicas**:
1. Idénticas a Deudores Banco
2. **Nombre empresa** (si PYME): obligatorio, texto no vacío

**Tipo de archivo**: INCLUSION

---

### 2.8 BOLÍVAR — Stock

**Identificación**:
- File prefix: `STOCK` (ídem MAPFRE, pero contexto es BOLÍVAR)

**Campos principales**:
- Idénticos a Deudores Banco/ESAL

**Validaciones específicas**:
1. Idénticas a Deudores Banco
2. **Deduplicación temporal**: idem MAPFRE stock

**Tipo de archivo**: STOCK

---

## 3. Ciclo Completo: Ejemplo Práctico

### Caso: Procesar archivo Vida Voluntario (10k pólizas)

**Entrada**:
```
INCLUSION_JUNIO_2026_MAPFRE_VoluntarioVida.xlsx (10,000 filas + header)
```

**Paso 1**: `POST /api/v1/process/scan`
- Sistema lista SFTP
- Encuentra archivo
- Encola: `processed_files.id=X, status=QUEUED`

**Paso 2**: Worker asignado
- Descarga archivo temporalmente
- Extrae filename → busca matching: `5024424900103` ó `VOLUNTARIO` → Vida Voluntario
- Obtiene `product_format.id = 1`, mappings JSON, rules JSON

**Paso 3**: Parseo (excelize)
- Lee 10k filas
- Mapea: Columna A (ID) → `identification_number`, Columna B (Nombre) → `policyholder_name`, etc.
- Construye 10k structs Policy

**Paso 4**: Validación fila-por-fila
- Fila 1: OK → no issues
- Fila 523: `birthdate = 01/01/1950` → edad 76 > 75 → **BLOQUEADOR** `EDAD_FUERA_RANGO`
- Fila 8234: `prime_annual = 10,000` pero plan requiere 8,500 → **BLOQUEADOR** `REVISAR PRIMA (PLAN)`
- Fila 9999: DNI duplicado con fila 2 → **INFORMATIVO** `DUPLICADO` (se registra pero no bloquea)

**Paso 5**: Gate (decisión file-level)
- ¿Hay bloqueadores? SÍ (filas 523, 8234)
- **Decisión**: Archivo entero → ERROR, no persiste NADA

**Paso 6**: Reportes y notificación
- Genera `validation-HASH.json`:
  ```json
  {
    "filename": "INCLUSION_JUNIO_2026_MAPFRE_VoluntarioVida.xlsx",
    "status": "ERROR",
    "total_rows": 10000,
    "blocking_issues": [
      {
        "row": 523,
        "identification_number": "12345678",
        "issue": "EDAD_FUERA_RANGO",
        "value": 76,
        "reason": "Must be between 18 and 75"
      },
      {
        "row": 8234,
        "identification_number": "87654321",
        "issue": "REVISAR PRIMA (PLAN)",
        "value": 10000,
        "reason": "Plan V3-50K requires 8500"
      }
    ],
    "informative_issues": [...],
    "inserted_policies": 0
  }
  ```
- Genera `validation-HASH.xlsx` (tablas con todas las issues)
- SendGrid: Email a operadores con attachments
- Status final: `processed_files.status = ERROR`

---

## 4. Flujo de Identificación: Casings y Substring Matching

El matching es **case-insensitive** y por **substring**:

```sql
SELECT * FROM product_formats pf
WHERE UPPER(?) LIKE CONCAT('%', UPPER(pf.file_prefix), '%')
ORDER BY LENGTH(pf.file_prefix) DESC, pf.priority ASC, pf.created_at DESC
LIMIT 1
```

**Ejemplos**:
| Filename | Búsqueda UPPER | Candidatos (ordered by specificity) | Resultado |
|----------|---|---|---|
| `INCLUSION_JUNIO_MAPFRE_VoluntarioVida.xlsx` | `%VOLUNTARIO%` | VOLUNTARIO | MAPFRE Vida Voluntario |
| `file_1784232459370419000_5024424900103_Pólizas.xlsx` | `%5024424900103%` | 5024424900103 | MAPFRE Vida Voluntario |
| `5024524900101_accidente_menores_junio.xlsx` | `%5024524900101%` | 5024524900101 | MAPFRE AP Menores |
| `STOCK_JUNIO_MAPFRE.xlsx` | `%STOCK%` | STOCK (MAPFRE), STOCK (BOLÍVAR) | Primer match o error de ambigüedad |

Si hay múltiples candidatos con igual specificity, se usa `priority` (valores bajos = prioritarios).

---

## 5. Estados de Archivo

| Estado | Significado | Transiciones |
|--------|-----------|---|
| `QUEUED` | Encolado, esperando worker | → PROCESSING |
| `PROCESSING` | Worker actualmente procesando | → PROCESSED ó ERROR |
| `PROCESSED` | Éxito: pólizas insertadas | (terminal) |
| `ERROR` | Falla: no se insertó nada | → QUEUED (retry via `/files/retry`) |
| `SKIPPED` | No se procesó (ej: extensión no válida) | (terminal) |
| `PENDING` | Estado inicial (desusado) | → QUEUED |

---

## 6. Modelo de Excepciones y Recuperación

### Excepciones en SFTP
- **Timeout de conexión**: error log, email notificación, archivo → SKIPPED
- **Archivo desaparece**: error log, archivo → SKIPPED
- **Permisos denegados**: email operador, → ERROR

### Excepciones en Base de Datos
- **Constraint violation** (ej: DNI no válido): validación previa debería detectar; si pasa a INSERT, transacción revierte, archivo → ERROR
- **MySQL timeout**: retry automático (3 intentos), luego → ERROR

### Recuperación Manual
- `POST /api/v1/files/retry?file_id=X` reinicia archivo desde SKIPPED/ERROR

---

## 7. Orquestación y Concurrencia

### Worker Pool
- Configurable: `PROCESSOR_WORKERS` (default 2)
- Cada worker consume canal de encolados
- **Sin garantía de orden** entre workers, pero **STOCK siempre primero** (encolado primero)

### Transacción por Archivo
- Cada archivo = una transacción MySQL (BEGIN, INSERT *N pólizas, COMMIT/ROLLBACK)
- Si algún INSERT falla → ROLLBACK total, archivo → ERROR

### Progress Tracking
- In-memory: `Service.progress[file_id] = ProgressEvent{row, total, status}`
- Expuesto vía `GET /api/v1/process/progress`

---

## 8. Resumen: Tabla Comparativa de Flujos

| Producto | Identificación | Tipo | Dedup | Post-Insert Logic | Etiquetas Clave |
|----------|---|---|---|---|---|
| MAPFRE Vida | VOLUNTARIO / 5024424900103 | INCLUSION | DNI+plan en archivo | — | REVISAR PRIMA (PLAN) / REVISAR PLAN |
| MAPFRE AP Menor | ACC MEN / 5024524900101 | INCLUSION | DNI+resp+plan | — | Idem |
| MAPFRE AP Cáncer | CANCER / 5024524900103 | INCLUSION | DNI+plan | — | Idem |
| MAPFRE Stock | STOCK (MAPFRE) | STOCK | DNI+plan archivo | CancelMissingStockPolicies() | — |
| MAPFRE Anulación | ANULACION_MASIVA | INCLUSION | DNI+plan archivo | applyMapfreCancellationsToStock() | — |
| BOLÍVAR Banco | DEUDORES_BANCO | INCLUSION | DNI+fecha | — | REVISAR PLAZO |
| BOLÍVAR ESAL | DEUDORES_ESAL | INCLUSION | DNI+fecha | — | REVISAR PLAZO |
| BOLÍVAR Stock | STOCK (BOLÍVAR) | STOCK | DNI+fecha | CancelMissingStockPolicies() | — |

