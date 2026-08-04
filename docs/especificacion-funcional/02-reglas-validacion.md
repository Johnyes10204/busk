# Reglas de Validación — Busk Seguros

## 1. Arquitectura de Validación

### 1.1 Niveles de Validación

```
Fila (Policy struct)
    ├─ Validaciones de campo (obligatorio, tipo, rango)
    ├─ Validaciones de lógica producto (ej: prima↔plan)
    └─ Cross-row (duplicados dentro del archivo actual)
    
Resultado: lista de Issues por fila
    ├─ Issue.Blocking = true  → BLOQUEADOR (impide persistencia de TODO)
    └─ Issue.Blocking = false → INFORMATIVO (se registra, no bloquea)
```

### 1.2 El Gate: Por Qué Falla el Archivo Entero

**Regla crítica**: Si CUALQUIER fila tiene CUALQUIER bloqueador → el archivo entero va a ERROR y NO se persiste NADA.

```go
// Pseudocódigo
if policiesRowSetHasBlockingIssues(allPolicies) {
    // Archivo entero → ERROR
    processedFile.Status = "ERROR"
    // Solo genera reportes, SIN INSERT
    generateValidationReports(issues, policies)
} else {
    // Hay solo informativos (ó ninguno)
    // Procede a INSERT
    insertPolicies(allPolicies)
}
```

**Razón**: Las pólizas son **atómicas por lote**. Un archivo representa un lote de ofertas/cambios del cliente; si hay errores de negocio, no se debe aceptar ninguna parte del lote (garantiza consistencia de inventario).

---

## 2. Validaciones de Campo

### 2.1 Identificación (DNI / RIF / Cédula)

**Aplica a**: Todos los productos (como `identification_number`)

**Regla**:
1. Obligatorio
2. Tipo: texto (convertido a string tras parseo XLSX)
3. No vacío ni espacios en blanco
4. Longitud: 6-15 caracteres
5. Caracteres válidos: dígitos + letras (A-Z, a-z), se normaliza a UPPER

**Validación de dígitos verificadores** (cédulas venezolanas):
- Formato: V-12345678 ó E-12345678 ó 12345678
- Si detecta formato V/E: valida dígito verificador (algoritmo módulo 11)
- Si es número puro: sin validación de dígito

**Issues**:
- `IDENTIFICATION_REQUIRED` → BLOQUEADOR
- `IDENTIFICATION_INVALID_FORMAT` → BLOQUEADOR
- `IDENTIFICATION_CHECKSUM_INVALID` → BLOQUEADOR (si aplica)

---

### 2.2 Nombres (Policyholder, Beneficiary, etc.)

**Aplica a**: `policyholder_name`, `beneficiary_name`, `business_name`

**Regla**:
1. Obligatorio (según producto)
2. Tipo: texto
3. No vacío ni solo espacios
4. Longitud: 3-150 caracteres
5. Caracteres permitidos: letras (incl. acentos), espacios, guiones

**Issues**:
- `NAME_REQUIRED` → BLOQUEADOR
- `NAME_INVALID_LENGTH` → BLOQUEADOR
- `NAME_INVALID_CHARACTERS` → INFORMATIVO (flag: possible corruption)

---

### 2.3 Fechas (Nascimento, Cobertura, etc.)

**Aplica a**: `birthdate`, `coverage_start_date`, `cancellation_date`, `exclusion_cancer_date`

**Formato esperado**: DD/MM/YYYY (validación estricta)

**Regla**:
1. Obligatorio (según producto)
2. Formato: DD/MM/YYYY (ej: 15/03/1985)
3. Fecha válida (ej: no 31/02/2020)
4. Rango razonable:
   - `birthdate`: 1920-01-01 ≤ fecha ≤ hoy
   - `coverage_start_date`: (hoy - 2 años) ≤ fecha ≤ (hoy + 30 días)
   - `cancellation_date`: (hoy - 5 años) ≤ fecha ≤ hoy
   - `exclusion_cancer_date`: ≥ `coverage_start_date`

**Issues**:
- `DATE_REQUIRED` → BLOQUEADOR
- `DATE_INVALID_FORMAT` → BLOQUEADOR
- `DATE_INVALID_CALENDAR` → BLOQUEADOR (31/02, etc.)
- `DATE_OUT_OF_RANGE` → BLOQUEADOR (ej: birthdate en 2050)
- `DATE_INCONSISTENCY` → BLOQUEADOR (ej: exclusion < coverage_start)

---

### 2.4 Números (Prima, Deuda, Cuota)

**Aplica a**: `prime_annual`, `outstanding_debt`, `monthly_payment`

**Regla**:
1. Obligatorio
2. Tipo: número (decimal, formato XLSX o string numérico)
3. Rango: ≥ 0 (prima/deuda pueden ser 0, cuota > 0)
4. Precisión: máx 2 decimales (para moneda)
5. Normalizacion: se redondea a 2 decimales en BD

**Issues**:
- `NUMERIC_REQUIRED` → BLOQUEADOR
- `NUMERIC_INVALID_FORMAT` → BLOQUEADOR
- `NUMERIC_OUT_OF_RANGE` → BLOQUEADOR
- `NUMERIC_INVALID_DECIMALS` → INFORMATIVO (se redondea automáticamente)

---

### 2.5 Status (ACTIVA / INACTIVA)

**Aplica a**: `status` en productos MAPFRE

**Regla**:
1. Obligatorio
2. Valores válidos: "ACTIVA", "INACTIVA" (case-insensitive)
3. Se normaliza a UPPER en BD

**Issues**:
- `STATUS_REQUIRED` → BLOQUEADOR
- `STATUS_INVALID_VALUE` → BLOQUEADOR

---

### 2.6 Plan (Catálogo)

**Aplica a**: `plan` en MAPFRE (Vida, AP Menor, AP Cáncer)

**Regla**:
1. Obligatorio
2. Debe existir en catálogo de planes del producto
3. Case-insensitive búsqueda
4. Ejemplo: "V3-50K", "V4-100K" para Vida Voluntario

**Catálogo MAPFRE (ejemplo)**:
| Plan | Prima Anual | Cobertura |
|------|-------------|-----------|
| V3-50K | 8,500 | 50,000 |
| V3-100K | 12,500 | 100,000 |
| V4-50K | 9,200 | 50,000 |

**Issues**:
- `PLAN_REQUIRED` → BLOQUEADOR
- `PLAN_NOT_FOUND` → BLOQUEADOR
- `PLAN_INACTIVE` → BLOQUEADOR (si existe pero no está activo)

---

## 3. Validaciones de Lógica de Negocio

### 3.1 Prima ↔ Plan (MAPFRE)

**Aplica a**: Vida Voluntario, AP Menor, AP Cáncer

**Regla**:
1. `prime_annual` debe coincidir exactamente con el valor oficial del `plan`
2. Si no coincide: **BLOQUEADOR**

**Diferenciación de etiqueta** (CRÍTICA):

```
if (plan_exists && prime_annual != plan.official_price) {
    if (plan_exists_in_catalog) {
        // La prima es incorrecta, pero el plan existe
        issue.Tag = "REVISAR PRIMA (PLAN)"  // ← Operador revisa prima
    } else {
        // El plan en sí es incorrecto
        issue.Tag = "REVISAR PLAN"          // ← Operador revisa plan
    }
}
```

**Razón de la diferenciación**:
- `REVISAR PRIMA (PLAN)`: El operador ve "Vida Voluntario" como plan válido, pero la prima está mal → revisa tabla de precios
- `REVISAR PLAN`: No existe un plan con ese nombre → revisa si el cliente escribió mal el código

**Issues**:
- `PRIMA_PLAN_MISMATCH_PRIMA` (tag: `REVISAR PRIMA (PLAN)`) → BLOQUEADOR
- `PRIMA_PLAN_MISMATCH_PLAN` (tag: `REVISAR PLAN`) → BLOQUEADOR

**Ejemplo**:
```
Fila: identification=12345678, plan="V3-50K", prime_annual=10,000
Catálogo: V3-50K = 8,500

Resultado: BLOQUEADOR
  issue.Tag = "REVISAR PRIMA (PLAN)"
  reason: "Plan V3-50K requires 8,500 but got 10,000"
```

---

### 3.2 Edad (Derivada de Birthdate)

**Aplica a**: Todos los productos

**Regla por producto**:
| Producto | Rango Mín | Rango Máx | Issue Tag |
|----------|---|---|---|
| MAPFRE Vida Voluntario | 18 | 75 | `EDAD_FUERA_RANGO` |
| MAPFRE AP Menor (beneficiary) | 0 | 17 | `EDAD_MENOR_INVALIDA` |
| MAPFRE AP Menor (responsable) | 18 | 100 | `EDAD_RESPONSABLE_FUERA_RANGO` |
| MAPFRE AP Cáncer | 18 | 75 | `EDAD_FUERA_RANGO` |
| BOLÍVAR Deudores | 18 | 75 | `EDAD_FUERA_RANGO` |

**Cálculo**:
```
age = (today - birthdate).Years
if age < min OR age > max:
    BLOQUEADOR
```

**Issues**:
- `EDAD_FUERA_RANGO` → BLOQUEADOR
- `EDAD_MENOR_INVALIDA` → BLOQUEADOR
- `EDAD_RESPONSABLE_FUERA_RANGO` → BLOQUEADOR

---

### 3.3 Deuda / Plazo (BOLÍVAR)

**Aplica a**: Deudores Banco, Deudores ESAL (BOLÍVAR)

**Regla**:
1. `outstanding_debt` ≥ 0
2. `monthly_payment` > 0
3. Ratio: `outstanding_debt / monthly_payment` ≤ 120 (plazo máx 120 meses = 10 años)
   - Si ratio > 120: **INFORMATIVO** (flag: `REVISAR PLAZO`)

**Cálculo de plazo para reporte**:
```
plazo_meses = REDONDEAR.MENOS(outstanding_debt / monthly_payment; 0)
// Función Excel: redondea hacia abajo
// Ejemplo: 100,000 / 1,050 = 95.24 → 95 meses
```

**Razón del threshold 120**:
- BOLÍVAR permite financiamiento de máx 120 meses
- Si cliente ofrece plazo > 120, es probable error de datos ó cliente tiene morosidad (deuda muy alta respecto a cuota)

**Issues**:
- `PLAZO_EXCEDE_MAXIMO` (tag: `REVISAR PLAZO`) → INFORMATIVO
- `MONTHLY_PAYMENT_INVALID` (cuota ≤ 0) → BLOQUEADOR
- `OUTSTANDING_DEBT_INVALID` (deuda < 0) → BLOQUEADOR

---

### 3.4 Exclusión Cáncer (MAPFRE AP Cáncer)

**Aplica a**: AP Cáncer

**Regla**:
1. Si existe `exclusion_cancer_date`:
   - Debe ser ≥ `coverage_start_date`
   - Debe ser ≤ hoy
   - Formato DD/MM/YYYY

**Issues**:
- `EXCLUSION_DATE_BEFORE_COVERAGE` → BLOQUEADOR
- `EXCLUSION_DATE_IN_FUTURE` → BLOQUEADOR

---

## 4. Validaciones de Duplicados (Dentro del Archivo)

### 4.1 Duplicados en MAPFRE

**Regla**: En un mismo archivo INCLUSION, si el mismo `(identification_number, plan)` aparece 2+ veces:
- Primera ocurrencia: OK
- Segunda+ ocurrencias: **INFORMATIVO** (tag: `DUPLICADO_EN_ARCHIVO`)

**Razón**: Puede ser error de cliente (copiar-pegar fila), pero se registra como "nota" sin bloquear.

---

### 4.2 Duplicados en BOLÍVAR

**Regla**: En un mismo archivo, si el mismo `(identification_number, coverage_start_date)` aparece 2+ veces:
- Primera ocurrencia: OK
- Segunda+ ocurrencias: **INFORMATIVO** (tag: `DUPLICADO_EN_ARCHIVO`)

---

## 5. Validaciones de Integridad Referencial

### 5.1 Póliza Existente (Anulación Masiva)

**Aplica a**: MAPFRE Anulación Masiva

**Regla**:
1. El par `(identification_number, plan)` debe coincidir con alguna póliza STOCK existente en BD (estado ACTIVE ó FROZEN)
2. Si no existe: **BLOQUEADOR** (tag: `POLIZA_NO_ENCONTRADA`)

**Issues**:
- `POLIZA_NO_ENCONTRADA` → BLOQUEADOR

---

### 5.2 Responsable Válido (MAPFRE AP Menor)

**Aplica a**: AP Menor (cuando se requiere responsable)

**Regla**:
1. Si el campo `policyholder_identification` es obligatorio: debe existir en BD como pólizaholding histórico ó validarse como DNI válido
2. Implementación actual: validación de DNI formato, no búsqueda en BD

**Issues**:
- `POLICYHOLDER_IDENTIFICATION_INVALID` → BLOQUEADOR

---

## 6. Validaciones Específicas por Producto

### 6.1 MAPFRE Vida Voluntario

**Checklist**:
1. Identification: obligatorio, válido ✓
2. Name: obligatorio, válido ✓
3. Birthdate: obligatorio, válido, 18-75 años ✓
4. Plan: obligatorio, en catálogo ✓
5. Prima: obligatorio, coincide con plan ✓
6. Coverage start date: obligatorio, válido, rango razonable ✓
7. Status: obligatorio, ACTIVA/INACTIVA ✓
8. Duplicados: `(identification_number, plan)` en archivo ✓

---

### 6.2 MAPFRE AP Menor

**Checklist** (idem Vida + extra):
1. Beneficiary identification: obligatorio, válido, 0-17 años ✓
2. Beneficiary name: obligatorio, válido ✓
3. Policyholder identification: obligatorio, válido, 18-100 años ✓
4. Policyholder name: obligatorio, válido ✓
5. Plan: obligatorio, en catálogo ✓
6. Prima: obligatorio, coincide con plan ✓
7. Coverage start date: obligatorio, válido ✓
8. Duplicados: `(beneficiary_id, policyholder_id, plan)` en archivo ✓

---

### 6.3 MAPFRE AP Cáncer

**Checklist**:
1. Identification: obligatorio, válido ✓
2. Name: obligatorio, válido ✓
3. Birthdate: obligatorio, válido, 18-75 años ✓
4. Plan: obligatorio, en catálogo ✓
5. Prima: obligatorio, coincide con plan ✓
6. Coverage start date: obligatorio, válido ✓
7. Exclusion cancer date: opcional, si existe ≥ coverage_start ✓
8. Duplicados: `(identification_number, plan)` en archivo ✓

---

### 6.4 BOLÍVAR Deudores Banco / ESAL

**Checklist**:
1. Identification: obligatorio, válido ✓
2. Name: obligatorio, válido ✓
3. Birthdate: obligatorio, válido, 18-75 años ✓
4. Outstanding debt: obligatorio, ≥ 0 ✓
5. Monthly payment: obligatorio, > 0 ✓
6. Ratio: outstanding / monthly ≤ 120 ✓ (si > 120: informativo REVISAR PLAZO)
7. Coverage start date: obligatorio, válido ✓
8. Status: obligatorio, ACTIVA/INACTIVA ✓
9. Business name (ESAL/Pyme): obligatorio si es PYME ✓
10. Duplicados: `(identification_number, coverage_start_date)` en archivo ✓

---

## 7. Estructura de Issue (JSON)

Cada issue registrada tiene esta estructura:

```json
{
  "row": 523,
  "identification_number": "12345678",
  "policyholder_name": "Juan Pérez",
  "issue_code": "EDAD_FUERA_RANGO",
  "issue_tag": "EDAD_FUERA_RANGO",
  "blocking": true,
  "value": 76,
  "expected_range": "18-75",
  "reason": "Age 76 exceeds maximum of 75",
  "timestamp": "2026-08-04T14:23:45Z"
}
```

**Campos**:
- `row`: número de fila en archivo (1-indexed, sin contar header)
- `identification_number`: DNI/RIF para trazabilidad
- `policyholder_name`: nombre para trazabilidad
- `issue_code`: código técnico (ej: `EDAD_FUERA_RANGO`)
- `issue_tag`: etiqueta para operador (ej: `REVISAR PRIMA (PLAN)`)
- `blocking`: true/false
- `value`: valor actual en fila
- `expected_range` ó `expected`: rango/valor esperado
- `reason`: descripción humana del motivo
- `timestamp`: cuándo se detectó

---

## 8. Generación de Reportes

### 8.1 Reporte JSON (validation-HASH.json)

```json
{
  "filename": "INCLUSION_JUNIO_2026_MAPFRE_VoluntarioVida.xlsx",
  "file_hash": "abc123def456...",
  "processed_at": "2026-08-04T14:23:45Z",
  "status": "ERROR",
  "product_format_id": 1,
  "product_format_name": "MAPFRE Vida Voluntario",
  "summary": {
    "total_rows": 10000,
    "blocking_issues_count": 2,
    "informative_issues_count": 15,
    "inserted_policies": 0
  },
  "blocking_issues": [
    {
      "row": 523,
      "identification_number": "12345678",
      "issue_code": "EDAD_FUERA_RANGO",
      "reason": "Age 76 exceeds maximum of 75"
    },
    ...
  ],
  "informative_issues": [
    {
      "row": 1050,
      "identification_number": "87654321",
      "issue_code": "DUPLICADO_EN_ARCHIVO",
      "reason": "Duplicate (identification_number, plan) pair"
    },
    ...
  ]
}
```

### 8.2 Reporte XLSX (validation-HASH.xlsx)

Tablas de Excel:
1. **Resumen**: metadatos del archivo, counts
2. **Bloqueadores**: tabla con todas las filas que tienen al menos 1 bloqueador
3. **Informativos**: tabla con issues informativos

---

## 9. Orden de Validación (Pseudocódigo)

```go
func ValidatePolicy(policy *Policy, format *ProductFormat) []Issue {
    var issues []Issue

    // 1. Validaciones de campo básicas
    if policy.IdentificationNumber == "" {
        issues.append(Issue{Code: "IDENTIFICATION_REQUIRED", Blocking: true})
        return issues  // No continua si falta ID
    }

    if policy.IdentificationNumber.ChecksumInvalid() {
        issues.append(Issue{Code: "IDENTIFICATION_CHECKSUM_INVALID", Blocking: true})
    }

    if policy.Name == "" {
        issues.append(Issue{Code: "NAME_REQUIRED", Blocking: true})
    }

    // 2. Validaciones de fecha
    if policy.Birthdate == "" {
        issues.append(Issue{Code: "DATE_REQUIRED", Blocking: true})
    } else if !policy.Birthdate.IsValidFormat() {
        issues.append(Issue{Code: "DATE_INVALID_FORMAT", Blocking: true})
    } else if !policy.Birthdate.IsValidCalendar() {
        issues.append(Issue{Code: "DATE_INVALID_CALENDAR", Blocking: true})
    } else if age := calculateAge(policy.Birthdate); age < 18 || age > 75 {
        issues.append(Issue{Code: "EDAD_FUERA_RANGO", Blocking: true, Value: age})
    }

    // 3. Validaciones de lógica de negocio
    if format.IsMapfrePlan {
        plan := FindPlan(policy.Plan)
        if plan == nil {
            issues.append(Issue{Code: "PLAN_NOT_FOUND", Blocking: true})
        } else if policy.PrimeAnnual != plan.OfficialPrice {
            issueCode := "PRIMA_PLAN_MISMATCH_PRIMA"
            issues.append(Issue{
                Code: issueCode,
                Tag: "REVISAR PRIMA (PLAN)",
                Blocking: true,
            })
        }
    }

    if format.IsBolívarDeudores {
        if policy.MonthlyPayment <= 0 {
            issues.append(Issue{Code: "MONTHLY_PAYMENT_INVALID", Blocking: true})
        }
        ratio := policy.OutstandingDebt / policy.MonthlyPayment
        if ratio > 120 {
            issues.append(Issue{
                Code: "PLAZO_EXCEDE_MAXIMO",
                Tag: "REVISAR PLAZO",
                Blocking: false,  // ← INFORMATIVO
            })
        }
    }

    return issues
}
```

---

## 10. Tabla Resumen: Todas las Validaciones

| Validación | Código | Tag | Producto | Blocking | Descripción |
|---|---|---|---|---|---|
| ID Obligatorio | IDENTIFICATION_REQUIRED | — | Todo | Sí | DNI/RIF no proporcionado |
| ID Formato | IDENTIFICATION_INVALID_FORMAT | — | Todo | Sí | Formato inválido (ej: solo espacios) |
| ID Checksum | IDENTIFICATION_CHECKSUM_INVALID | — | Todo | Sí | Dígito verificador inválido |
| Nombre Obligatorio | NAME_REQUIRED | — | Todo | Sí | Nombre no proporcionado |
| Nombre Longitud | NAME_INVALID_LENGTH | — | Todo | Sí | Nombre muy corto/largo |
| Fecha Requerida | DATE_REQUIRED | — | Todo | Sí | Fecha no proporcionada |
| Fecha Formato | DATE_INVALID_FORMAT | — | Todo | Sí | No es DD/MM/YYYY |
| Fecha Calendario | DATE_INVALID_CALENDAR | — | Todo | Sí | Ej: 31/02/2020 |
| Fecha Rango | DATE_OUT_OF_RANGE | — | Todo | Sí | Ej: birthdate en 2050 |
| Edad Rango | EDAD_FUERA_RANGO | — | MAPFRE/Bolívar | Sí | Edad fuera de 18-75 |
| Edad Menor | EDAD_MENOR_INVALIDA | — | MAPFRE AP Menor | Sí | Beneficiary edad > 17 |
| Plan Obligatorio | PLAN_REQUIRED | — | MAPFRE | Sí | Plan no proporcionado |
| Plan Existe | PLAN_NOT_FOUND | — | MAPFRE | Sí | Plan no existe en catálogo |
| Plan Activo | PLAN_INACTIVE | — | MAPFRE | Sí | Plan desactivado |
| Prima Requerida | NUMERIC_REQUIRED | — | MAPFRE/Bolívar | Sí | Prima/deuda no proporcionada |
| Prima Formato | NUMERIC_INVALID_FORMAT | — | MAPFRE/Bolívar | Sí | Prima no es número |
| Prima ↔ Plan | PRIMA_PLAN_MISMATCH_PRIMA | REVISAR PRIMA (PLAN) | MAPFRE | Sí | Prima no coincide con plan |
| Prima ↔ Plan | PRIMA_PLAN_MISMATCH_PLAN | REVISAR PLAN | MAPFRE | Sí | Plan incorrecto |
| Cuota > 0 | MONTHLY_PAYMENT_INVALID | — | Bolívar | Sí | Cuota es 0 o negativa |
| Plazo Máx | PLAZO_EXCEDE_MAXIMO | REVISAR PLAZO | Bolívar | No | Plazo > 120 meses |
| Status Válido | STATUS_INVALID_VALUE | — | MAPFRE | Sí | Status no es ACTIVA/INACTIVA |
| Exclusión Cáncer | EXCLUSION_DATE_BEFORE_COVERAGE | — | MAPFRE AP Cáncer | Sí | Exclusión < cobertura inicio |
| Póliza Existe | POLIZA_NO_ENCONTRADA | — | MAPFRE Anulación | Sí | Póliza no existe para anular |
| Duplicado Archivo | DUPLICADO_EN_ARCHIVO | — | Todo | No | Póliza duplicada en archivo |

---

## 11. Flujo de Decisión Gate (Decisión File-Level)

```
Validaciones completas sobre todas las filas
    ↓
¿Existe al menos 1 issue con blocking=true?
    ├─ SÍ → Archivo → ERROR
    │   ├─ Genera reportes (JSON + XLSX)
    │   ├─ NO ejecuta INSERT
    │   ├─ Email con bloqueadores
    │   └─ processed_files.status = "ERROR"
    │
    └─ NO → Archivo → Procede
        ├─ INSERT todas las pólizas
        ├─ Ejecuta lógica post-insert (stock, anulación)
        ├─ Genera reportes (JSON, solo informativos si hay)
        ├─ Email con resumen
        └─ processed_files.status = "PROCESSED"
```

