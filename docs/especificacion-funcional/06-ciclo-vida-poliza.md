# Ciclo de Vida de Póliza — Busk Seguros

## Descripción General

Una póliza en Busk Seguros transita a través de varios estados durante su ciclo de vida, desde su creación hasta su cancelación o congelamiento. Este documento detalla los estados, transiciones, eventos, y auditoría asociados.

---

## 1. Estados de Póliza

### 1.1 Estados Principales

```
┌──────────────────────────────────────────────────────────────────────┐
│                      CICLO DE VIDA DE PÓLIZA                         │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  CREACIÓN (INSERT)                                                   │
│          ↓                                                            │
│  ┌──────────────┐       ┌──────────────┐                            │
│  │   ACTIVE     │──────→│   FROZEN     │                            │
│  │ (Estado OK)  │       │ (Suspendida) │                            │
│  └──────────────┘       └──────────────┘                            │
│      ↓                         ↓                                      │
│      └──────────────┬──────────┘                                     │
│                     ↓                                                │
│  ┌──────────────────────────────┐                                   │
│  │   MANUAL_REVIEW              │                                   │
│  │ (Requiere revisión manual)   │                                   │
│  └──────────────────────────────┘                                   │
│      ↓                     ↑                                          │
│      └─────────────────────┘                                         │
│                     ↓                                                │
│  ┌──────────────────────────────┐                                   │
│  │   CANCELLED                  │ ← (Estado terminal)               │
│  │ (Póliza cancelada)           │                                   │
│  └──────────────────────────────┘                                   │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

### 1.2 Tabla de Estados

| Estado | Código BD | Significado | Inicio | Terminal | Transiciones Posibles |
|--------|---|---|---|---|---|
| **ACTIVE** | ACTIVE | Póliza vigente, en cobertura | INSERT | No | → FROZEN, → MANUAL_REVIEW, → CANCELLED |
| **FROZEN** | FROZEN | Póliza suspendida (ej: revisión, pago pendiente) | Acción manual | No | → ACTIVE, → MANUAL_REVIEW, → CANCELLED |
| **MANUAL_REVIEW** | MANUAL_REVIEW | Requiere revisión manual (ej: inconsistencias) | Validación / Manual | No | → ACTIVE, → FROZEN, → CANCELLED |
| **CANCELLED** | CANCELLED | Póliza anulada/cancelada | Acción manual / Sistema | **Sí** | (ninguna) |

---

## 2. Transiciones de Estado

### 2.1 ACTIVE → FROZEN

**Disparadores**:
1. Acción manual del operador (marca como suspendida)
2. Detección de pago pendiente (futuro)
3. Revisión de riesgo requerida

**Campos actualizados**:
- `policy_status = 'FROZEN'`
- `updated_at = NOW()`
- Registro en auditoría con motivo

**Ejemplo**:
```
Usuario en admin: "Suspender póliza 12345678/V3-50K"
  ↓
UPDATE policies
SET policy_status = 'FROZEN',
    updated_at = NOW()
WHERE identification_number = '12345678' AND plan = 'V3-50K'
  ↓
INSERT INTO audit_logs (action, policy_id, reason, user_id, timestamp)
VALUES ('STATUS_CHANGE', 50001, 'Suspensión solicitada por operador', 'user_123', NOW())
```

---

### 2.2 FROZEN → ACTIVE

**Disparadores**:
1. Acción manual: operador reactiva póliza
2. Resolución de incidente (pago confirmado, revisión completada)

**Campos actualizados**:
- `policy_status = 'ACTIVE'`
- `updated_at = NOW()`
- Registro en auditoría

---

### 2.3 ACTIVE ó FROZEN → MANUAL_REVIEW

**Disparadores**:
1. Validación detecta inconsistencia no bloqueadora (issue informativo)
   - Ej: Prima > 20% desviación del plan
   - Ej: Datos duplicados pero con pequeñas variaciones
2. Acción manual del operador

**Campos actualizados**:
- `policy_status = 'MANUAL_REVIEW'`
- `review_reason = 'Prima 20% desviada del plan'` (si aplica)
- `updated_at = NOW()`

---

### 2.4 MANUAL_REVIEW → ACTIVE ó FROZEN

**Disparadores**:
1. Operador completa revisión manual y aprueba
2. Se corrigen datos inconsistentes

**Campos actualizados**:
- `policy_status = 'ACTIVE'` ó `'FROZEN'` (según operador)
- `review_completed_at = NOW()`
- `review_completed_by = 'user_id'`
- Registro en auditoría

---

### 2.5 ACTIVE ó FROZEN ó MANUAL_REVIEW → CANCELLED

**Disparadores**:
1. **Anulación masiva** (archivo ANULACION_MASIVA): sistema cancela automáticamente
2. **Ausencia en stock** (deduplicación de stock): sistema cancela automáticamente
3. **Acción manual**: operador cancela póliza

**Campos actualizados**:
- `policy_status = 'CANCELLED'`
- `cancellation_reason = 'Ausente en stock'` | `'Anulación masiva'` | `'Cancelación manual'`
- `cancellation_date = NOW()` ó fecha específica (si se proporciona)
- `updated_at = NOW()`

**Ejemplo 1: Ausencia en stock**
```go
// Durante procesamiento de nuevo stock
CancelMissingStockPolicies() {
  newStockSet := {("12345678", "V3-50K"), ("23456789", "V4-100K")}
  previousActive := db.Query("SELECT * FROM policies WHERE policy_status = 'ACTIVE'")
  
  toCancel := previousActive - newStockSet
  // toCancel = {("11111111", "V3-50K")} — póliza antigua que no aparece
  
  for each pair in toCancel {
    UPDATE policies
    SET policy_status = 'CANCELLED',
        cancellation_reason = 'Ausente en stock',
        cancellation_date = NOW()
    WHERE identification_number = '11111111' AND plan = 'V3-50K'
    
    INSERT INTO audit_logs (action, reason, ...)
    VALUES ('CANCEL_MISSING_STOCK', 'Ausente en stock MAPFRE junio 2026', ...)
  }
}
```

**Ejemplo 2: Anulación masiva**
```go
// Archivo ANULACION_MASIVA insertado
ApplyMapfreCancellationsToStock() {
  cancellations := db.Query(
    "SELECT identification_number, plan FROM policy_cancellations"
  )
  
  for each (id, plan) in cancellations {
    UPDATE policies
    SET policy_status = 'CANCELLED',
        cancellation_reason = 'Anulación masiva',
        cancellation_date = NOW()
    WHERE identification_number = id AND plan = plan
    
    INSERT INTO audit_logs (action, reason, ...)
  }
}
```

---

## 3. Esquema de BD (policies)

```sql
CREATE TABLE policies (
  -- Identificadores
  id INT PRIMARY KEY AUTO_INCREMENT,
  identification_number VARCHAR(15) NOT NULL,
  policyholder_name VARCHAR(150) NOT NULL,
  birthdate DATE NOT NULL,
  
  -- Producto
  plan VARCHAR(50) NOT NULL,
  prime_annual DECIMAL(10, 2) NOT NULL,
  coverage_start_date DATE NOT NULL,
  product_format_id INT NOT NULL,
  
  -- Estado (CICLO DE VIDA)
  policy_status ENUM('ACTIVE', 'FROZEN', 'MANUAL_REVIEW', 'CANCELLED') DEFAULT 'ACTIVE',
  policy_status_changed_at TIMESTAMP,
  
  -- Cancelación (si aplica)
  cancellation_reason VARCHAR(255),
  cancellation_date DATE,
  
  -- Revisión manual (si aplica)
  review_reason VARCHAR(255),
  review_completed_at TIMESTAMP,
  review_completed_by VARCHAR(100),
  
  -- Auditoría
  processed_file_id INT,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  
  -- Índices
  INDEX idx_identification (identification_number),
  INDEX idx_plan (plan),
  INDEX idx_status (policy_status),
  INDEX idx_product_format (product_format_id),
  UNIQUE KEY uk_policy (identification_number, plan, coverage_start_date, product_format_id)
);
```

---

## 4. Eventos del Ciclo de Vida

### 4.1 Eventos Principales

| Evento | Descripción | Origen | Datos Registrados |
|--------|---|---|---|
| **CREATED** | Póliza insertada en BD | INSERT archivo | file_id, row_number, timestamp |
| **STATUS_CHANGED** | Cambio de estado manual | Usuario / API | from_status, to_status, reason, user_id |
| **FROZEN** | Transición a FROZEN | Sistema / Manual | reason, timestamp |
| **MANUAL_REVIEW_TRIGGERED** | Requiere revisión | Validación | issue_code, reason |
| **REVIEW_COMPLETED** | Revisión manual completada | Usuario | approved_status, reviewer_id, notes |
| **CANCELLED** | Póliza cancelada | Sistema / Manual | cancellation_reason, date |
| **REACTIVATED** | Póliza reactivada desde FROZEN | Manual | reason, user_id |

### 4.2 Timeline de Evento Típico

**Escenario**: Archivo STOCK ingesta póliza, luego es detectada inconsistencia

```
2026-06-01 10:15:00 — CREATED
  Póliza insertada desde STOCK_JUNIO_2026_MAPFRE.xlsx
  policy_status = ACTIVE
  file_id = 1001

2026-06-05 14:30:00 — MANUAL_REVIEW_TRIGGERED
  Operador nota prima 20% superior a catálogo
  policy_status = ACTIVE → MANUAL_REVIEW
  review_reason = "Prima 20% desviada del plan"

2026-06-07 09:45:00 — REVIEW_COMPLETED
  Operador aprueba tras revisar con cliente
  policy_status = MANUAL_REVIEW → ACTIVE
  review_completed_by = "operator_123"
  review_completed_at = 2026-06-07 09:45:00

2026-08-04 18:15:00 — CANCELLED (Stock ausencia)
  Nuevo stock MAPFRE no incluye esta póliza
  policy_status = ACTIVE → CANCELLED
  cancellation_reason = "Ausente en stock"
  cancellation_date = 2026-08-04
  file_id = 1050 (nuevo stock)
```

---

## 5. Auditoría

### 5.1 Tabla audit_logs

```sql
CREATE TABLE audit_logs (
  id INT PRIMARY KEY AUTO_INCREMENT,
  
  -- Referencia a póliza
  policy_id INT NOT NULL,
  identification_number VARCHAR(15),
  plan VARCHAR(50),
  
  -- Acción
  action ENUM('CREATED', 'STATUS_CHANGED', 'FROZEN', 'CANCELLED', 'MANUAL_REVIEW_TRIGGERED', 'REVIEW_COMPLETED', 'REACTIVATED'),
  
  -- Detalles
  from_status VARCHAR(50),
  to_status VARCHAR(50),
  reason VARCHAR(500),
  details JSON,
  
  -- Usuario / Sistema
  user_id VARCHAR(100),
  system_process VARCHAR(100),
  
  -- Referencias
  processed_file_id INT,
  related_audit_id INT,
  
  -- Timestamp
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  
  INDEX idx_policy (policy_id),
  INDEX idx_action (action),
  INDEX idx_timestamp (created_at)
);
```

### 5.2 Ejemplo de Auditoría

```json
[
  {
    "id": 5001,
    "policy_id": 50001,
    "identification_number": "12345678",
    "plan": "V3-50K",
    "action": "CREATED",
    "from_status": null,
    "to_status": "ACTIVE",
    "reason": "Inserted from STOCK_JUNIO_2026_MAPFRE.xlsx",
    "details": {
      "source_file": "STOCK_JUNIO_2026_MAPFRE.xlsx",
      "row_number": 10,
      "processed_file_id": 1001
    },
    "user_id": null,
    "system_process": "processor.InsertPolicies",
    "processed_file_id": 1001,
    "created_at": "2026-06-01T10:15:00Z"
  },
  {
    "id": 5002,
    "policy_id": 50001,
    "identification_number": "12345678",
    "plan": "V3-50K",
    "action": "MANUAL_REVIEW_TRIGGERED",
    "from_status": "ACTIVE",
    "to_status": "MANUAL_REVIEW",
    "reason": "Prima 20% desviada del plan",
    "details": {
      "issue_code": "PRIMA_DEVIATION_HIGH",
      "prima_catalog": 8500,
      "prima_file": 10200,
      "deviation_percent": 20
    },
    "user_id": null,
    "system_process": "processor.ValidatePolicies",
    "processed_file_id": 1001,
    "created_at": "2026-06-05T14:30:00Z"
  },
  {
    "id": 5003,
    "policy_id": 50001,
    "identification_number": "12345678",
    "plan": "V3-50K",
    "action": "REVIEW_COMPLETED",
    "from_status": "MANUAL_REVIEW",
    "to_status": "ACTIVE",
    "reason": "Approved after client verification",
    "details": {
      "reviewer_notes": "Cliente confirmó precio especial (10,200 Bs)",
      "resolution_type": "approved"
    },
    "user_id": "operator_123",
    "system_process": null,
    "processed_file_id": null,
    "created_at": "2026-06-07T09:45:00Z"
  },
  {
    "id": 5004,
    "policy_id": 50001,
    "identification_number": "12345678",
    "plan": "V3-50K",
    "action": "CANCELLED",
    "from_status": "ACTIVE",
    "to_status": "CANCELLED",
    "reason": "Ausente en stock",
    "details": {
      "cancellation_type": "missing_from_stock",
      "processed_file_id": 1050,
      "new_stock_file": "STOCK_AGOSTO_2026_MAPFRE.xlsx"
    },
    "user_id": null,
    "system_process": "processor.CancelMissingStockPolicies",
    "processed_file_id": 1050,
    "created_at": "2026-08-04T18:15:00Z"
  }
]
```

---

## 6. Queries Comunes de Auditoría

### 6.1 Historial de Póliza

```sql
SELECT *
FROM audit_logs
WHERE policy_id = 50001
ORDER BY created_at ASC;
```

### 6.2 Pólizas Canceladas Recientemente

```sql
SELECT p.*, a.reason, a.cancellation_date
FROM policies p
LEFT JOIN audit_logs a ON p.id = a.policy_id AND a.action = 'CANCELLED'
WHERE p.policy_status = 'CANCELLED'
  AND p.updated_at >= NOW() - INTERVAL 7 DAY
ORDER BY p.updated_at DESC;
```

### 6.3 Pólizas en Revisión Manual Más de 7 Días

```sql
SELECT p.*, a.review_reason, a.policy_status_changed_at
FROM policies p
LEFT JOIN audit_logs a ON p.id = a.policy_id AND a.action = 'MANUAL_REVIEW_TRIGGERED'
WHERE p.policy_status = 'MANUAL_REVIEW'
  AND (NOW() - INTERVAL 7 DAY) > a.created_at
ORDER BY a.created_at ASC;
```

### 6.4 Actividad de Cancelación por Día

```sql
SELECT 
  DATE(a.created_at) as cancel_date,
  COUNT(*) as policies_cancelled,
  GROUP_CONCAT(DISTINCT a.reason) as reasons
FROM audit_logs a
WHERE a.action = 'CANCELLED'
GROUP BY DATE(a.created_at)
ORDER BY cancel_date DESC;
```

---

## 7. Estados Especiales (Edge Cases)

### 7.1 Póliza Duplicada en Archivo

**Escenario**: Mismo DNI+plan aparece 2 veces en archivo INCLUSION

**Comportamiento**:
1. Primera ocurrencia: Insertada normalmente
2. Segunda ocurrencia: Detectada como DUPLICADO_EN_ARCHIVO (INFORMATIVO, no bloquea)
   - Se inserta igual (o con UPDATE si existe UNIQUE)
   - Se registra en issue como informativo
   - No cambia estado de póliza automáticamente

**Auditoría**:
```json
{
  "action": "CREATED",
  "reason": "Inserted from INCLUSION_JUNIO_2026.xlsx (duplicate detection: 2nd occurrence)",
  "details": {
    "issue_tag": "DUPLICADO_EN_ARCHIVO",
    "first_occurrence_row": 50,
    "second_occurrence_row": 150
  }
}
```

---

### 7.2 Póliza Bloqueada en Validación

**Escenario**: Póliza falla validación bloqueadora (edad, prima, etc.)

**Resultado**:
- **NO se inserta** en BD (archivo entero va a ERROR)
- **NO hay póliza** para cambiar de estado (no existe registro)
- **Se registra** solo en reporte de validación (JSON + XLSX)
- **Email** informativo al operador

**Auditoría**: NO hay entry en audit_logs (nunca se insertó)

---

### 7.3 Póliza Revertida a Stock Anterior

**Escenario**: Nuevo stock omite póliza que estaba ACTIVE

**Secuencia**:
```
Stock Junio 2026: Póliza A (ACTIVE)
Stock Julio 2026: Póliza A presente (sin cambios, status=ACTIVE)
Stock Agosto 2026: Póliza A AUSENTE
  ↓
CancelMissingStockPolicies() ejecutada
  ↓
Póliza A: ACTIVE → CANCELLED ("Ausente en stock")

Auditoría:
  action: CANCELLED
  reason: Ausente en stock
  processed_file_id: 1050 (stock agosto)
```

---

## 8. Reportes y Dashboards (Futuros)

### 8.1 Métricas de Estado

```
Total Pólizas: 45,000
  ├─ ACTIVE: 40,000 (88.9%)
  ├─ FROZEN: 2,500 (5.6%)
  ├─ MANUAL_REVIEW: 1,200 (2.7%)
  └─ CANCELLED: 1,300 (2.9%)

Cancelaciones (últimos 30 días): 234
  ├─ Ausentes en stock: 150 (64%)
  ├─ Anulación masiva: 60 (26%)
  ├─ Cancelación manual: 24 (10%)

Pólizas en Revisión Manual > 7 días: 45
  ├─ Prima desviada: 30 (67%)
  ├─ Datos inconsistentes: 12 (27%)
  ├─ Otro: 3 (6%)
```

### 8.2 SLA de Revisión Manual

```
Pólizas en MANUAL_REVIEW:
  ├─ < 24h: 800 (67%)
  ├─ 24-72h: 300 (25%)
  ├─ 72h+: 100 (8%) ← Alertar

Resolución promedio: 2.3 días
Tasa aprobación: 85%
Tasa rechazo: 15%
```

---

## 9. Transiciones Prohibidas

Las siguientes transiciones **NO son permitidas** y deben rechazarse:

| De | A | Razón |
|---|---|---|
| CANCELLED | ACTIVE | Póliza cancelada no se puede reactiva |
| CANCELLED | FROZEN | Póliza cancelada no se puede congelar |
| CANCELLED | MANUAL_REVIEW | Póliza cancelada no requiere revisión |
| ACTIVE/FROZEN | ACTIVE* | Redundante (sin cambio) |

*Si el operador intenta transición redundante, se registra como no-op (sin cambio).

---

## 10. Flujo de Decisión: Cambio de Estado

```
¿Quién solicita cambio de estado?
    ├─ SISTEMA
    │  ├─ ValidatePolicies() detecta inconsistencia
    │  │  └─ policy_status = MANUAL_REVIEW
    │  ├─ CancelMissingStockPolicies()
    │  │  └─ policy_status = CANCELLED, reason = "Ausente en stock"
    │  └─ ApplyMapfreCancellationsToStock()
    │     └─ policy_status = CANCELLED, reason = "Anulación masiva"
    │
    └─ OPERADOR (vía API/UI futuro)
       ├─ Suspender póliza
       │  └─ policy_status = ACTIVE → FROZEN
       ├─ Reactivar póliza
       │  └─ policy_status = FROZEN → ACTIVE
       ├─ Enviar a revisión manual
       │  └─ policy_status = ACTIVE/FROZEN → MANUAL_REVIEW
       └─ Cancelar póliza
          └─ policy_status = ANY (no CANCELLED) → CANCELLED
```

---

## 11. Ejemplo Completo: Póliza MAPFRE con Inconsistencia

### Fila 1: Creación Normal
```
2026-06-01 — Archivo STOCK_JUNIO_2026_MAPFRE.xlsx procesado
Póliza 50001: DNI 12345678, Plan V3-50K, Prima 8500
  → policy_status = ACTIVE
  → audit: CREATED from file 1001
```

### Fila 2: Archivo Nuevo con Prima Desviada
```
2026-06-05 — Archivo INCLUSION_JUNIO_2026_ACTUALIZACION.xlsx procesado
Fila con: DNI 12345678, Plan V3-50K, Prima 10200
  → Validación: Prima 10200 ≠ catálogo 8500 (20% desv.)
  → Issue tag: REVISAR PRIMA (PLAN) (BLOQUEADOR)
  → Archivo entero va a ERROR
  → Póliza 50001 NO se modifica (sigue ACTIVE)
  → Operador recibe email con error
```

### Fila 3: Operador Contacta Cliente
```
2026-06-07 — Operador verifica con cliente
Cliente confirma: "Prima especial de 10,200 por volumen"
  → Operador marca póliza 50001 como MANUAL_REVIEW
  → policy_status = ACTIVE → MANUAL_REVIEW
  → review_reason = "Prima desviada (cliente confirmó precio especial)"
  → audit: MANUAL_REVIEW_TRIGGERED
```

### Fila 4: Operador Aprueba Revisión
```
2026-06-07 — Operador completa revisión
  → policy_status = MANUAL_REVIEW → ACTIVE
  → review_completed_at = NOW()
  → review_completed_by = "operator_123"
  → audit: REVIEW_COMPLETED with notes
```

### Fila 5: Nuevo Stock Omite Póliza
```
2026-08-04 — Archivo STOCK_AGOSTO_2026_MAPFRE.xlsx procesado
Stock nuevo NO incluye DNI 12345678, Plan V3-50K
  → CancelMissingStockPolicies() detecta ausencia
  → policy_status = ACTIVE → CANCELLED
  → cancellation_reason = "Ausente en stock"
  → cancellation_date = 2026-08-04
  → audit: CANCELLED due to missing from stock
```

### Auditoría Completa de Póliza 50001

```json
{
  "policy": {
    "id": 50001,
    "identification_number": "12345678",
    "plan": "V3-50K",
    "current_status": "CANCELLED",
    "created_at": "2026-06-01T10:15:00Z",
    "last_status_change": "2026-08-04T18:15:00Z"
  },
  "audit_trail": [
    {
      "timestamp": "2026-06-01T10:15:00Z",
      "action": "CREATED",
      "status_transition": "null → ACTIVE",
      "reason": "Inserted from STOCK_JUNIO_2026_MAPFRE.xlsx",
      "actor": "system"
    },
    {
      "timestamp": "2026-06-05T14:30:00Z",
      "action": "MANUAL_REVIEW_TRIGGERED",
      "status_transition": "ACTIVE → MANUAL_REVIEW",
      "reason": "Prima 20% desviada del plan",
      "actor": "system"
    },
    {
      "timestamp": "2026-06-07T09:45:00Z",
      "action": "REVIEW_COMPLETED",
      "status_transition": "MANUAL_REVIEW → ACTIVE",
      "reason": "Approved after client verification",
      "actor": "operator_123",
      "notes": "Cliente confirmó precio especial (10,200 Bs)"
    },
    {
      "timestamp": "2026-08-04T18:15:00Z",
      "action": "CANCELLED",
      "status_transition": "ACTIVE → CANCELLED",
      "reason": "Ausente en stock",
      "actor": "system",
      "processed_file_id": 1050
    }
  ]
}
```

