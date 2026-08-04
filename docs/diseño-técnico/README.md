# Documentación de Diseño Técnico: Busk Seguros

**Versión:** 1.0 | **Fecha:** Agosto 2025 | **Estado:** Actualizado

Esta carpeta contiene 6 documentos markdown profesionales que describen exhaustivamente la arquitectura, componentes, decisiones técnicas, patrones de diseño, seguridad y performance de **Busk Seguros**, una plataforma integral de ingesta, validación y materialización de pólizas de seguros.

---

## Documentos

### 1. [01-arquitectura.md](./01-arquitectura.md) — 520 líneas

**Propósito:** Visión holística del sistema, componentes principales, flujo end-to-end.

**Contenido:**
- Resumen ejecutivo y stack tecnológico
- Diagrama ASCII de la arquitectura
- Descripción de componentes (API Go, Processor, Store, Frontend React, SFTP, MySQL)
- Flujo completo de procesamiento (13 stages)
- Interacciones clave entre módulos
- Patrones de seguridad básicos
- Capacidades generales

**Leer si:** Necesitas entender el sistema en su totalidad o explicar a stakeholders.

---

### 2. [02-componentes.md](./02-componentes.md) — 1.083 líneas

**Propósito:** Detalles técnicos profundos de cada componente.

**Contenido:**
- **API HTTP:** Handlers por categoría (health, productos, formatos, procesamiento, archivos, pólizas)
- **Processor Service:** Inicialización, job loop, validación de filas, file-level gate
- **Store (MySQL):** Schema, operaciones CRUD, queries complejas, validación
- **Frontend React:** Estructura, tabs, API calls, auto-refresh
- **SFTP Client:** Conexión, descarga, movimiento de archivos
- **Notificación (SendGrid):** Configuración y envío de correos

**Código:** Ejemplos Go y React inline.

**Leer si:** Trabajas en desarrollo, debugging, o necesitas entender implementación específica.

---

### 3. [03-decisiones-tecnicas.md](./03-decisiones-tecnicas.md) — 547 líneas

**Propósito:** Justificar por qué cada tecnología; trade-offs y alternativas rechazadas.

**Contenido:**
- Go 1.23 como backend (performance, simplicity, producción)
- React 19 + TypeScript + Vite (SPA, dev experience)
- MySQL 8.0+ (ACID, confiabilidad)
- Worker pool asincrónico (paralelismo, recuperación ante fallos)
- SFTP para ingesta remota (seguridad, movimiento atómico)
- Archivado en disco local (auditoría, dedup, reportes pesados)
- SendGrid para notificaciones (fiabilidad, escalado)
- Repository pattern (testabilidad, maintainability)
- Validación declarativa + procedural (configurabilidad, complejidad)
- File-level gate (atomicidad de carga)
- Stock cancellations automáticas
- Dedup por SHA-256
- Matriz resumen de decisiones

**Leer si:** Quieres entender la filosofía de diseño o justificar cambios arquitectónicos.

---

### 4. [04-patrones-diseño.md](./04-patrones-diseño.md) — 699 líneas

**Propósito:** Patrones reutilizables empleados y los problemas que resuelven.

**Contenido:**
1. **Worker Pool Pattern** — Paralelismo, resource control
2. **Repository Pattern** — Desacoplamiento, testabilidad
3. **Pipeline Pattern** — Separación de concerns, early exit
4. **Strategy Pattern** — Validación declarativa + procedural
5. **Builder Pattern** — Construcción incremental de reportes
6. **Observer Pattern** — Real-time progress tracking
7. **Adapter Pattern** — SFTP vs. Local file sources
8. **Chain of Responsibility** — Validación multi-capa

Cada patrón incluye:
- Descripción
- Implementación en Busk (código)
- Problema resuelto
- Variantes y trade-offs

**Leer si:** Buscas patrones reutilizables o quieres refactorizar componentes.

---

### 5. [05-seguridad.md](./05-seguridad.md) — 612 líneas

**Propósito:** Identificar riesgos de seguridad y mitigaciones implementadas.

**Contenido:**
1. **Validación de Entrada** — HTTP sanitization, JSON parsing, mapeo canónico
2. **Inyección SQL** — Parameterized queries (Squirrel), audit trail
3. **Path Traversal** — Nombres de archivo validados, SHA-256 para archivos
4. **Datos Sensibles** — Credenciales en env vars (no logs), PII en respuestas
5. **Acceso a Base de Datos** — SSL/TLS, least privilege, connection pooling, timeouts
6. **Autorización** — No implementada en v1 (asumir red privada); futuro OAuth2
7. **Inyección CSV/XLSX** — Datos desde BD (no raw files), escape de fórmulas Excel
8. **Rate Limiting & DoS** — Queue capacity, paginación forzada, timeouts SFTP
9. **Logging & Auditoría** — Todos eventos críticos se loguean, sin passwords
10. **Actualización de Dependencias** — go mod verify, govulncheck
11. **Secrets Management** — Checklist de dónde vive cada secret
12. **Deployment Seguro** — Dockerfile minimal, Network Policies K8s, HTTPS

**Leer si:** Necesitas auditoría de seguridad, preparar producción, o implementar auth.

---

### 6. [06-performance-escalabilidad.md](./06-performance-escalabilidad.md) — 590 líneas

**Propósito:** Optimización de performance, escalado y monitoreo.

**Contenido:**
1. **Arquitectura de Performance** — Single server, bottleneck analysis
2. **Throughput Teórico** — Cálculos: 0.44 archivos/s (2 workers), up to 150 archivos/min (16 workers)
3. **Configuración de Workers** — Recomendaciones por hardware
4. **Tuning de MySQL** — Buffer pool, I/O, connections, índices, monitoreo
5. **Tuning de I/O en Disco** — Rotación de archivos, particionamiento, compresión
6. **Reducción de Latencia** — Validación paralela (futuro), caching de reglas
7. **Límites de Escalado** — Bottlenecks identificados, beyond single server
8. **Profiling & Debugging** — Go pprof, benchmarking, tracing de requests
9. **Monitoreo en Producción** — Prometheus metrics, Grafana dashboards, alertas
10. **Checklist de Optimización** — Development, staging, production
11. **Resumen de Capacidades** — Tabla de valores de rendimiento
12. **Roadmap de Escalabilidad** — 4 fases (1 server → 100K+ pólizas/min)

**Leer si:** Necesitas optimizar performance, preparar para carga, o monitorear producción.

---

## Guía de Lectura por Rol

### Arquitecto / Product Manager
**Orden recomendado:** 01 → 03 → 06 (resumen)
- Entiende componentes, decisiones, roadmap de escalado

### Developer (Backend Go)
**Orden recomendado:** 01 → 02 → 04 → 05
- Detalles técnicos, patrones, seguridad

### Developer (Frontend React)
**Orden recomendado:** 01 (seccion Frontend) → 02 (seccion Frontend) → 04 (Observer)
- API HTTP, componentes React, estado

### DevOps / SRE
**Orden recomendado:** 01 → 06 → 05 (deployment)
- Arquitectura, performance, security, monitoring

### Security / Compliance
**Orden recomendado:** 05 → 03 (decisiones security) → 02 (validación)
- Validaciones, mitigaciones, secrets, HTTPS

### QA / Tester
**Orden recomendado:** 01 → 02 → 04 (test cases por patrón)
- Flujos, componentes, validaciones

---

## Estadísticas

| Aspecto | Valor |
|---------|-------|
| **Total de líneas** | 4.051 |
| **Total de palabras** | ~32.000 |
| **Documentos** | 6 |
| **Código Go ejemplo** | 150+ snippets |
| **Código React ejemplo** | 20+ snippets |
| **Diagramas ASCII** | 8 |
| **Tablas de referencia** | 25+ |
| **Checklist** | 3 (dev/staging/prod) |

---

## Convenciones

### Formato
- **Títulos:** Markdown H1-H6 jerárquicos
- **Código:** Bloques ` ``` ` con language hint (go, tsx, sql, bash, yaml)
- **Énfasis:** `monospace` para código/términos, **bold** para importante, _italic_ para énfasis
- **Listas:** `-` para bullets, `1.` para numeradas
- **Tablas:** Markdown tables con alineación

### Nomenclatura
- **Métodos Go:** CamelCase (ej. `InsertPolicies`, `FindProductFormatCandidates`)
- **Constantes:** UPPER_SNAKE_CASE (ej. `PROCESSOR_WORKERS`, `FILES_ARCHIVE_DIR`)
- **Variables de entorno:** UPPER_SNAKE_CASE (ej. `MYSQL_DSN`, `SFTP_PASSWORD`)
- **Rutas API:** `/api/v1/...` (lowercase con hyphens)
- **Estados:** UPPER_SNAKE_CASE (ej. `PROCESSING`, `MANUAL_REVIEW`)

### Lenguaje
- Español para contenido principal (audiencia hispanohablante)
- Inglés para código (convención Go/React)
- Comentarios de código en español cuando aplica

---

## Actualización y Mantenimiento

Estos documentos deben actualizarse cuando:

1. **Agregar feature:** Actualizar 01 (flujo), 02 (componente), 04 (patrón si aplica)
2. **Refactorizar:** Actualizar 02 (implementación), 04 (patrón), 06 (performance)
3. **Cambio tecnológico:** Actualizar 03 (decisión)
4. **Descubrimiento de seguridad:** Actualizar 05 (mitigación)
5. **Mejora de performance:** Actualizar 06 (tuning, benchmark)

**Versión:** Incrementar en header si cambios significativos.

---

## Cómo Usar Este Material

### Para Documentación Interna
- Copiar/pegar a Confluence, Notion o wiki interna
- Formatear con theme corporativo

### Para Propuesta a Clientes
- Copiar secciones clave a Word/PDF
- Omitir detalles técnicos internos (05, 06)
- Incluir 01 (resumen), 03 (tecnologías), diagrama de 04

### Para Onboarding de Nuevos Devs
- Entregar 01-04 como lectura obligatoria (semana 1)
- Leer 05-06 durante ramp-up (semana 2-3)
- Luego: mergear PR guiados en el código real

### Para Auditoría / Compliance
- Secciones clave: 05 (seguridad), 06 (capacidades), 01 (residencia de datos)
- Generar PDF signado

---

## Referencias Cruzadas

- **CLAUDE.md** (raíz): Instrucciones rápidas de desarrollo
- **services/api/main.go:** Implementación de handlers HTTP
- **services/api/internal/processor/processor.go:** Implementación del pipeline
- **frontend-admin/src/App.tsx:** Interfaz de usuario
- **docs/postman/:** Collection de Postman para testing API

---

## Contacto y Contribuciones

Para sugerencias, correcciones o actualizaciones:

1. Crear issue con tag `docs`
2. Proponer cambios vía PR con `[docs]` en título
3. Revisar con Arquitecto de diseño antes de publicar

---

**Última actualización:** Agosto 2025 | **Próxima revisión:** Diciembre 2025
