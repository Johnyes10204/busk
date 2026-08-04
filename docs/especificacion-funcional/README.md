# Especificación Funcional — Busk Seguros

## Descripción General

Este directorio contiene la especificación funcional completa de **Busk Seguros**, un sistema de ingesta de pólizas de seguros (XLSX/XLS/CSV) desde SFTP, validación contra reglas de negocio específicas por producto, y persistencia en MySQL.

La especificación es **exhaustiva, profesional y lista para transposición a documentos Word**. Se divide en 6 módulos que cubren todos los aspectos del sistema.

---

## Estructura de Documentos

### 1. **01-flujos-negocio.md** (451 líneas)

Describe los flujos end-to-end de procesamiento para cada producto:

- **Flujo general** de 5 fases: escaneo, identificación, parseo, validación, archivo y notificación
- **Flujos por producto** (8 total):
  - MAPFRE: Vida Voluntario, AP Menores, AP Cáncer, Stock, Anulación Masiva
  - BOLÍVAR: Deudores Banco (Micro/Pyme), Deudores ESAL (Micro/Pyme), Stock
- **Ciclo completo**: ejemplo práctico de ingesta de stock 10k pólizas
- **Identificación**: matching case-insensitive y substring-based
- **Estados de archivo**: QUEUED, PROCESSING, PROCESSED, ERROR, SKIPPED
- **Orquestación**: worker pool, transacciones, concurrencia
- **Tabla comparativa**: resumen de características por producto

**Público objetivo**: Analistas de negocio, product managers, operadores.

---

### 2. **02-reglas-validacion.md** (586 líneas)

Detalla TODAS las reglas de validación del sistema:

- **Arquitectura de validación**: 3 niveles (campo, lógica producto, cross-row)
- **El gate crítico**: por qué si 1 fila falla, el archivo entero NO se persiste
- **Validaciones de campo**:
  - Identificación (DNI/RIF, formato, checksum)
  - Nombres (longitud, caracteres)
  - Fechas (formato DD/MM/YYYY, rango, validez de calendario)
  - Números (decimal, rango, precisión)
  - Status (enum ACTIVA/INACTIVA)
  - Plan (existencia en catálogo)
- **Validaciones de lógica de negocio**:
  - Prima ↔ Plan (MAPFRE): etiquetas diferenciadas (`REVISAR PRIMA (PLAN)` vs `REVISAR PLAN`)
  - Edad (rango por producto)
  - Deuda / Plazo (BOLÍVAR, máximo 120 meses)
  - Exclusión Cáncer (fecha consistencia)
- **Duplicados** (dentro de archivo)
- **Integridad referencial** (póliza existe, responsable válido)
- **Estructura JSON de issue**
- **Generación de reportes** (JSON + XLSX)
- **Orden de validación** (pseudocódigo)
- **Tabla resumen**: todas las validaciones indexadas

**Público objetivo**: Desarrolladores, QA, analistas de validación.

---

### 3. **03-casos-uso.md** (805 líneas)

8 escenarios reales de procesamiento que ilustran el sistema en acción:

1. **Stock exitoso** (10k pólizas): éxito completo con deduplicación
2. **Errores de edad**: bloqueadores que rechazan archivo
3. **Prima incorrecta**: distinción de etiqueta por tipo de error
4. **Anulación masiva**: 50 pólizas canceladas automáticamente
5. **Duplicados detectados**: póliza duplicada en archivo (informativo)
6. **Stock duplicado en BD**: póliza anterior reemplazada
7. **Pólizas faltantes**: cancelación automática por ausencia en stock
8. **SFTP timeout**: excepción y recuperación
9. **Validación parcial**: mezcla de bloqueadores e informativos

Cada caso incluye:
- Contexto y datos
- Pasos detallados
- Código/pseudocódigo
- Salidas JSON
- Emails resultantes
- Estado final en BD

**Público objetivo**: Desarrolladores, operadores, testers de UAT.

---

### 4. **04-api-endpoints.md** (812 líneas)

Especificación OpenAPI-style de todos los endpoints:

#### 2. Products & Formats
- `GET /products` — listar productos
- `GET /product-formats` — listar formatos
- `GET /product-formats/active` — solo activos
- `POST /product-formats/match-test` — probar matching
- `GET /products/allowed-premiums` — catálogo de primas

#### 3. Processing
- `POST /process/scan` — escanear SFTP y encolar
- `GET /process/progress` — progreso en tiempo real

#### 4. Files
- `GET /files` — listar archivos procesados (con paginación)
- `POST /files/retry` — reintentar archivo fallido
- `GET /files/summary` — estadísticas
- `GET /files/validation-report` — reporte JSON
- `GET /files/validation-csv` — reporte CSV
- `GET /files/validation-xlsx` — reporte Excel
- `GET /files/download` — descargar original

#### 5. Policies
- `GET /policies` — listar pólizas (con filtros)
- `POST /policies/search` — búsqueda avanzada

Cada endpoint incluye:
- Request completo (parámetros, body JSON)
- Response exitosa (200 OK)
- Response de error (400, 404, 500, etc.)
- Códigos HTTP
- Ejemplos de uso completo

**Público objetivo**: Desarrolladores frontend, integradores, API consumers.

---

### 5. **05-mapeos-columnas.md** (809 líneas)

Especificación detallada de mapeos XLSX → BD para cada producto:

- **Estructura JSON de mapeo**: alias_mapping + field_rules
- **Campos canónicos globales** (identification_number, policyholder_name, etc.)
- **Campos específicos por producto**:
  - MAPFRE Vida: plan, prime_annual, status
  - MAPFRE AP Menor: beneficiary_identification, policyholder_identification, edades diferenciadas
  - MAPFRE AP Cáncer: exclusion_cancer_date
  - MAPFRE Stock: sin status (todos ACTIVE)
  - MAPFRE Anulación: cancellation_date, cancellation_reason
  - BOLÍVAR Banco/ESAL: outstanding_debt, monthly_payment, debt_currency
  - BOLÍVAR Pyme: business_name adicional
- **Transformaciones globales**:
  - Trim, normalize_spaces
  - Uppercase para IDs/planes/status
  - Parsing decimal con 2 decimales
  - Parsing fecha DD/MM/YYYY
- **Alias comunes** (español + English fallback)
- **Algoritmo de mapeo** (pseudocódigo)
- **Ejemplos completos** con filas XLSX y resultado parseado

**Público objetivo**: Desarrolladores, data engineers, operadores de soporte.

---

### 6. **06-ciclo-vida-poliza.md** (682 líneas)

Ciclo de vida completo de una póliza desde creación hasta cancelación:

- **Estados principales**: ACTIVE, FROZEN, MANUAL_REVIEW, CANCELLED
- **Diagrama de transiciones**: visual + tabla
- **Transiciones detalladas**:
  - ACTIVE ↔ FROZEN
  - ACTIVE/FROZEN → MANUAL_REVIEW
  - MANUAL_REVIEW → ACTIVE/FROZEN
  - ANY → CANCELLED (terminal)
- **Esquema BD**: tabla `policies` con campos de estado, cancelación, revisión
- **Eventos del ciclo**: CREATED, STATUS_CHANGED, FROZEN, CANCELLED, etc.
- **Auditoría**: tabla `audit_logs` con trazabilidad completa
- **Queries comunes**: historial, pólizas en revisión, cancelaciones
- **Edge cases**:
  - Póliza duplicada en archivo
  - Póliza bloqueada en validación
  - Póliza revertida por ausencia en stock
- **Métricas y dashboards** (futuros)
- **Transiciones prohibidas**
- **Ejemplo completo**: póliza con inconsistencia, revisión y cancelación

**Público objetivo**: Desarrolladores backend, analistas de datos, operadores senior.

---

## Navegación Rápida

| Pregunta | Documento |
|----------|-----------|
| ¿Cómo fluye un archivo desde SFTP a BD? | 01-flujos-negocio.md |
| ¿Cuáles son todas las validaciones? | 02-reglas-validacion.md |
| ¿Qué pasa si...? (casos reales) | 03-casos-uso.md |
| ¿Qué endpoints existen? | 04-api-endpoints.md |
| ¿Cómo se mapean las columnas XLSX? | 05-mapeos-columnas.md |
| ¿Cuál es el ciclo de vida de una póliza? | 06-ciclo-vida-poliza.md |

---

## Características Generales

### Cobertura de Productos
- **MAPFRE**: 5 formatos (Vida Voluntario, AP Menores, AP Cáncer, Stock, Anulación)
- **BOLÍVAR**: 3 familias de productos (Deudores Banco, ESAL, Stock) × 2 variantes (Micro/Pyme)
- **Total**: 8 flujos únicos documentados

### Validaciones Documentadas
- **34 reglas de validación** con etiquetas diferenciadas
- **Duplicados, integridad referencial, lógica de negocio**
- **Gate crítico**: por qué falla el archivo entero si hay 1 bloqueador

### Casos de Uso
- **9 escenarios reales**: desde éxito hasta SFTP timeout
- **Código y pseudocódigo** para cada paso
- **Salidas JSON, emails, estado final**

### API Completamente Especificada
- **11 endpoints** con parámetros, responses, códigos HTTP
- **Ejemplos prácticos** de uso end-to-end
- **Paginación, filtros, búsqueda avanzada**

### Auditoría y Ciclo de Vida
- **4 estados de póliza** con transiciones permitidas
- **Auditoría completa** de todas las acciones
- **Queries ejemplificadas**

---

## Cómo Usar Esta Documentación

### Para Product Managers / Stakeholders
1. Leer **01-flujos-negocio.md** para entender end-to-end
2. Leer **03-casos-uso.md** para ver ejemplos concretos

### Para Desarrolladores Backend
1. Leer **02-reglas-validacion.md** para reglas de negocio
2. Leer **05-mapeos-columnas.md** para transformaciones de datos
3. Leer **06-ciclo-vida-poliza.md** para auditoría y estados

### Para Desarrolladores Frontend / API Consumers
1. Leer **04-api-endpoints.md** completamente
2. Consultar **01-flujos-negocio.md** para contexto

### Para QA / Testers
1. Leer **03-casos-uso.md** para crear test cases
2. Leer **02-reglas-validacion.md** para validaciones edge case
3. Consultar **04-api-endpoints.md** para request/response validation

### Para Operadores
1. Leer **01-flujos-negocio.md** (secciones de estados de archivo)
2. Leer **03-casos-uso.md** para entender qué puede fallar
3. Leer **06-ciclo-vida-poliza.md** (sección de transiciones)

---

## Estándares de Documentación

### Formato
- Markdown (.md) con estructura clara (H1, H2, H3, etc.)
- Tablas para comparativas y resúmenes
- JSON/pseudocódigo para ejemplos técnicos
- Diagramas ASCII para flujos visuales

### Lenguaje
- **Español** para textos, comentarios y ejemplos user-facing
- **Inglés** en nombres de campos, códigos, constantes (ej: `ACTIVE`, `CANCELLED`, `identification_number`)
- **Legible para stakeholders no técnicos** donde aplique

### Exhaustividad
- Cada endpoint completamente especificado (request, response, ejemplos)
- Cada validación con código técnico, etiqueta, y descripción
- Cada producto con casos de éxito y error

---

## Convenciones Utilizadas

### Status Codes HTTP
- `200` — Éxito
- `400` — Bad Request (parámetros inválidos)
- `404` — Not Found
- `409` — Conflict
- `500` — Server Error
- `503` — Service Unavailable

### Nombres de Campos BD
- `identification_number` — DNI/Cédula/RIF
- `policy_status` — Estado de póliza (ENUM)
- `prime_annual` — Prima anual en moneda local
- `coverage_start_date` — Fecha inicio cobertura (DD/MM/YYYY)
- Timestamps: `created_at`, `updated_at`, `processed_at`

### Etiquetas de Validación (Issue Tags)
- `REVISAR PRIMA (PLAN)` — Prima incorrecta para plan válido
- `REVISAR PLAN` — Plan incorrecto/inexistente
- `REVISAR PLAZO` — Plazo > 120 meses (BOLÍVAR)
- `DUPLICADO_EN_ARCHIVO` — Póliza duplicada en mismo archivo
- `EDAD_FUERA_RANGO` — Edad fuera de rango permitido

---

## Notas Importantes

1. **El gate es crítico**: Si CUALQUIER fila tiene CUALQUIER bloqueador, el ARCHIVO ENTERO va a ERROR y NO se persiste NADA. Esto garantiza consistencia de inventario.

2. **Prima ↔ Plan diferenciado**: Si falla la validación prima↔plan, el operador DEBE ver `REVISAR PRIMA (PLAN)`, nunca `REVISAR PLAN`, para distinguir si el error es la prima o el código de plan.

3. **Deduplicación temporal de stock**: Pólizas que estaban ACTIVE y no aparecen en nuevo stock son automáticamente canceladas con motivo "Ausente en stock".

4. **Anulación masiva**: Aplica cancelaciones a pólizas STOCK matching solo al final de procesamiento (post-insert).

5. **Auditoría completa**: Cada cambio de estado de póliza es registrado en `audit_logs` con actor, timestamp, razón y detalles.

---

## Control de Versión

| Versión | Fecha | Cambios |
|---------|-------|---------|
| 1.0 | 2026-08-04 | Especificación inicial completa: 6 módulos, 3,745 líneas |

---

## Contacto / Propietario

Especificación desarrollada por **Busk Seguros Product Team**.

Para actualizaciones, correcciones o preguntas, consultar con el equipo de producto.

---

**Última actualización**: 2026-08-04

**Ubicación**: `/docs/especificacion-funcional/`
