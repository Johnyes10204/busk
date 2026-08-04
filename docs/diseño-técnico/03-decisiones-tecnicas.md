# Decisiones Técnicas y Trade-Offs

## 1. Go como Backend

### Decisión

Implementar API HTTP y pipeline de procesamiento en **Go 1.23**.

### Razones

#### **Performance**

- Compilación a binario nativo sin VM/JIT overhead
- Startup time < 100ms
- Bajo consumo de memoria baseline
- Goroutines eficientes para worker pool (overhead ~2-4 KB cada una)
- Manejo de I/O concurrente sin hilos del SO (netpoller)

#### **Simplicidad**

- Lenguaje minimalista: pocos conceptos, sintaxis clara
- Estándar library robusta: `encoding/json`, `database/sql`, `net/http`, `crypto/sha256`
- No requiere frameworks heavy (sin ORM, sin DI container)
- Error handling explícito evita sorpresas

#### **Producción**

- Go es usado en sistemas críticos (Docker, Kubernetes, etcd, Consul)
- Binario único, sin dependencias de runtime
- Deployment trivial: copiar executable a servidor, restart
- Hot reload natural con blue-green deploy

#### **Integración SFTP**

- Librería `golang.com/x/crypto/ssh` + SFTP es de primera clase
- Reliable connection pooling y manejo de timeouts

### Trade-Offs

| Aspecto | Ventaja | Desventaja |
|---------|---------|-----------|
| **Tipado estático** | Errores en compile-time | Menos flexibilidad que Python/JS |
| **Concurrencia explícita** | Predictible, debuggable | Requiere pensamiento cuidadoso (goroutines) |
| **Ecosistema** | Maduro para backend | Menos librerías de ML/ciencia que Python |
| **Syntax** | Minimalista | Más verbose en algunos idiomas (error checking) |

---

## 2. React 19 + TypeScript para Frontend

### Decisión

Consola admin con **React 19**, **TypeScript**, **Vite**.

### Razones

#### **Developer Experience**

- TypeScript evita errores en tiempo de compilación
- Hot reload con Vite (~20ms)
- React Hooks permiten lógica reusable sin clases
- Componentes funcionales simples

#### **Single-Page App Ideal**

- Operadores necesitan monitoreo en tiempo real (polling /progress)
- Múltiples tabs (Operación, Productos, Archivos, Pólizas)
- Cambios de estado sin refresh de página
- JSON API-first design se acopla perfectamente

#### **Deployment**

- Compilación a estáticos (HTML/CSS/JS)
- Servible vía CDN o mismo servidor que API
- Fallback proxy: `/api/v1` redirige a backend

### Trade-Offs

| Aspecto | Ventaja | Desventaja |
|---------|---------|-----------|
| **Learning curve** | Familiar a web devs | Curva de Hooks/useState |
| **Build size** | ~120-150 KB gzipped | No es tan ligero como Svelte |
| **SPA rendering** | Rápido, smooth UX | SEO no es prioritario (admin panel) |
| **CSS** | Custom, flexible | Requiere disciplina sin framework |

---

## 3. MySQL como Base de Datos Principal

### Decisión

Usar **MySQL 8.0+** (o MariaDB compatible) como repositorio de autoridad.

### Razones

#### **Confiabilidad**

- ACID transactions para InsertPolicies (atomicidad)
- Backups bien conocidos (mysqldump, binlog)
- Replicación nativa (master-slave)
- Wide industry adoption

#### **Esquema Relacional Natural**

- Productos y formatos son catálogos (actualización rara)
- Políticas es tabla de hechos con índices por product_id, credit_number, document_number
- Archivos procesados son logs de auditoría
- Parámetros tunables son configuración (producto/global)

#### **Query Performance**

- Índices simples (product_id, file_hash, status, credit_number)
- Squirrel query builder genera SQL predictible
- Page-based search es trivial (`OFFSET`, `LIMIT`)
- Conteo y agregaciones rápidas

### Trade-Offs

| Aspecto | Ventaja | Desventaja |
|---------|---------|-----------|
| **Schema flexibilidad** | Estructura clara | Cambios require migrations |
| **JSON columns** | Puede almacenar mappings/rules | Queries dentro JSON más lento |
| **Escalado horizontal** | Sharding posible | No nativo (requiere app logic) |
| **Full-text search** | Índices FTS disponibles | No es Elasticsearch |

#### **Por qué NO NoSQL**

- **MongoDB:** Schemas flexibles but sin ACID en multi-document (hasta 4.0). Busk requiere transacciones.
- **DynamoDB:** Serverless atractivo pero locked-in a AWS. Escalado requiere cuidado.
- **Cassandra:** Eventual consistency overkill. Write-heavy pero no read patterns complejos.

---

## 4. Worker Pool en lugar de Monolith Sincrónico

### Decisión

Diseño **asincrónico basado en worker pool** (canal + N goroutines) en lugar de procesar archivos secuencialmente en el request HTTP.

### Razones

#### **No Bloquear Operador**

```
Malo:  POST /scan → esperar 10 minutos → response (timeout probable)
Bueno: POST /scan → enqueue archivos inmediatamente → response 202 Accepted
        GET /progress → monitoreo en tiempo real
```

#### **Paralelismo Real**

- Procesar múltiples archivos simultáneamente
- PROCESSOR_WORKERS configurable (default 2, tuneable para HW)
- Saturar CPU/I/O sin hilos del SO innecesarios

#### **Recuperación ante Fallos**

- Panic en worker no mata API
- Archivo marcado ERROR y recuperable vía retry
- MarkStaleFilesAsError al startup limpia interrupciones

#### **Observabilidad**

- ProgressInfo en memoria permite dashboard real-time
- Status transitions explícitas (QUEUED → PROCESSING → PROCESSED|ERROR)

### Trade-Offs

| Aspecto | Ventaja | Desventaja |
|---------|---------|-----------|
| **Latencia** | No bloquear operador | Resultado no inmediato |
| **Complejidad** | Concurrencia mínima (canal) | Debugging de goroutines requiere care |
| **Monitoreo** | Status en /progress | Operador debe revisar activamente |
| **Resource control** | Buffered channel 256 limita memory | Si queue llena, rechazar archivo |

---

## 5. SFTP para Ingesta Remota

### Decisión

Usar **protocolo SFTP** (SSH File Transfer) para leer archivos desde servidores remotos de aseguradoras.

### Razones

#### **Seguridad**

- Encriptación SSH, autenticación (key-based o password)
- Más seguro que FTP plain, SFTP telnet
- Firewalls permiten puerto 22 (SSH) casi siempre

#### **Movimiento Atómico**

- Renombrar archivo remotamente: `SFTP RENAME archivo ERROR/archivo` es atomic
- No requiere descargar, procesar, re-subir

#### **Compatibilidad**

- Servidores SFTP ampliamente soportados (OpenSSH, Windows SFTP)
- Clientes también: Filezilla, command-line sftp, librerías en todos los lenguajes

#### **Alternativa Local para Testing**

- Modo `--scan-local /path/dir` para desarrollo
- Misma lógica de procesamiento, sin SFTP overhead

### Trade-Offs

| Aspecto | Ventaja | Desventaja |
|---------|---------|-----------|
| **Latencia** | Fast para WAN | Más lento que NFS |
| **Throughput** | Suficiente para XML/CSV | No optimizado para streaming gigabytes |
| **Seguridad** | Encriptado | Credenciales en env var (requiere secrets mgmt) |
| **Mantenimiento** | Protocolo estable | SSH hang risk si servidor timeout (mitigado) |

---

## 6. Archivado en Disco Local

### Decisión

Guardar copia de cada archivo ingested en `FILES_ARCHIVE_DIR` (SHA-256 como nombre).
Guardar reportes de validación en `REPORTS_ARCHIVE_DIR`.

### Razones

#### **Auditoría**

- Trail inmutable de qué se procesó y cuándo
- SHA-256 previene manipulación
- Recuperable si DB se corrompe

#### **Dedup**

- Verificar si archivo duplicado sin re-parsear
- SHA-256 es fast (O(n) una sola pasada)

#### **Reportes Pesados**

- Archivos enormes pueden generar validation JSON > max_allowed_packet (MySQL default 4MB)
- Guardar XLSX en disco evita truncamiento
- API sirve desde disco directamente (eficiente)

#### **Recuperación**

- Si S3/cloud no disponible, disk local es fallback simple
- No requiere credenciales de API adicionales

### Trade-Offs

| Aspecto | Ventaja | Desventaja |
|---------|---------|-----------|
| **Portabilidad** | Disk es universal | Requiere storage local suficiente |
| **Escalado** | Crece con volumen | No distribuido (único servidor) |
| **Backup** | rsync trivial | Requiere mantenimiento del sysadmin |
| **Cloud** | Evita vendor lock-in | S3 podría ser más resiliente |

#### **Por qué NO S3 Directamente**

- Añade complejidad: IAM roles, bucket policies
- Latencia de red: S3 PutObject más lento que disk write
- Costos: small deployments (10K pólizas/mes) más caro en S3
- Simplicidad: local disk es suficiente hasta escala significativa

---

## 7. Notificaciones Email (SendGrid)

### Decisión

Usar **SendGrid API** para enviar correos con adjuntos (CSV/XLSX de validaciones).

### Razones

#### **Fiabilidad**

- Servicio dedicado maneja reintentos, bounces
- No requiere servidor SMTP local (reduce dependency)
- 99.9% uptime garantizado

#### **Adjuntos**

- API nativa para archivos adjuntos (vs SMTP con codificación MIME)
- Busk genera reportes XLSX y envía directamente

#### **Escalado**

- SendGrid escala automáticamente
- No preocuparse por rate limiting del dominio

#### **Fallback Silencioso**

- Si `SENDGRID_API_KEY` no set → notificaciones omitidas (no error)
- Permite deployment sin email temporalmente

### Trade-Offs

| Aspecto | Ventaja | Desventaja |
|---------|---------|-----------|
| **Acoplamiento** | Desacoplado: interfaz | Requiere cuenta SendGrid |
| **Costo** | Free tier para <100/día | Pago para volumen alto |
| **Latencia** | Async, no bloquea processor | Puede tomar segundos |
| **Debugging** | Logs de SendGrid | No control total del envío |

#### **Por qué NO SMTP Local**

- Servidor SMTP requiere admin/mantenimiento
- Deliverability issues (SPF/DKIM/DMARC)
- Rebotes no procesados automáticamente

---

## 8. Patrón Repository (Abstracción de Store)

### Decisión

Crear interfaz `Store` que encapsula todas las operaciones de DB.
Processor depende de Store, no de driver SQL directo.

```go
type Store interface {
  InsertPolicies(policies []model.PolicyRecord) error
  ListPoliciesByProduct(productID string, status string, limit int) []model.PolicyRecord
  FindProductFormatCandidates(fileName string) []model.Product
  AddFileRecord(r model.FileProcessRecord)
  // ... ~30 métodos
}
```

### Razones

#### **Testabilidad**

- Mock Store para unit tests sin DB real
- Processor tests no requieren MySQL

#### **Maintainability**

- Cambios de schema localizados en Store
- Processor no conoce SQL

#### **Flexibility**

- Futuro: cambiar MySQL a PostgreSQL sin tocar Processor
- Caching layer fácil de interponer

### Trade-Offs

| Aspecto | Ventaja | Desventaja |
|---------|---------|-----------|
| **Abstracción** | Desacoplamiento | Más métodos a escribir |
| **Queries** | Centralizadas | Overhead pequeño de indirection |

---

## 9. Validación Declarativa + Reglas de Negocio

### Decisión

Dos capas de validación:

1. **Declarativa:** `rules_json` en formato (type="required_not_empty", type="number_gte", etc.)
2. **Procedural:** `applyDiagramRules()` en Go para lógica compleja (plan↔prima MAPFRE, deuda Bolívar)

### Razones

#### **Configurabilidad**

- Reglas simples (requerido, rango) sin código
- Operador puede cambiar vía API → POST /products/formats

#### **Mantenibilidad**

- Reglas complejas (negocio específico) en procedural Go
- Evita JSON complicado / DSL casero

#### **Auditoría**

- Cada validación genera nota (tag: "REVISAR PLAN", etc.)
- Informe Excel/CSV enumera todas las incidencias por fila
- Archivo ERROR tiene validación report completo (no persiste pólizas)

### Trade-Offs

| Aspecto | Ventaja | Desventaja |
|---------|---------|-----------|
| **Flexibilidad** | Mezclar declarativo + código | Aprender ambos formatos |
| **Performance** | Validación compilada (Go) | JSON parse overhead |
| **UX** | Mensajes localizados (Spanish) | Cambiar idioma requiere code change |

---

## 10. File-Level Gate (Atomicidad de Carga)

### Decisión

Si **cualquier fila** tiene blocking issues → **NO se persisten pólizas del archivo completo**.

```go
if policiesRowSetHasBlockingIssues(policies) {
  // Generar informe de validación
  // Status = ERROR
  // NO INSERT INTO policies
  return rec  // Salir
}

// Solo si no hay blocking issues:
if err := s.store.InsertPolicies(policies); err != nil { ... }
```

### Razones

#### **Garantía de Calidad**

- Operador puede rechazar lote completo si hay dudas
- No se carga datos parcialmente dudosos

#### **Auditoría**

- Archivo → estado PROCESSED o ERROR (nunca parcial)
- Reporte enumera qué falló exactamente

#### **Recuperación**

- Si operador corrige archivo → retry simple
- No mixtura de pólizas válidas + inválidas en misma carga

### Trade-Offs

| Aspecto | Ventaja | Desventaja |
|---------|---------|-----------|
| **Strictness** | Garantía de calidad | Puede rechazar archivos con pocos errores |
| **Recovery** | Operador controla | Requiere reintento manual |
| **Performance** | Una transacción | Parsing completo incluso si error temprano |

#### **Alternativa Rechazada: Per-Row Skips**

```go
// Malo: insertar válidas, ignorar inválidas
for _, p := range policies {
  if policyHasBlockingIssues(p) { continue }
  insert(p)  // Problema: mixtura de cargas
}
```

Rechazado porque hace auditoría imposible.

---

## 11. Stock Cancellations (Auto-cancelar Faltantes)

### Decisión

Cuando un archivo STOCK es procesado exitosamente:
- Pólizas históricas del producto NO presentes en stock actual → automáticamente CANCELLED

```go
if isStockProduct(selectedProduct.Code) {
  currentCredits := extractCreditsFromPolicies(policies)
  cancelled, _ := s.store.CancelMissingStockPolicies(
    selectedProduct.ID, rec.ID, currentCredits,
  )
  log.Printf("stock cancelaciones=%d", cancelled)
}
```

### Razones

#### **Datos Frescos**

- Stock define "verdad" en momento T
- Créditos no en stock = cliente pagó, póliza caducó, etc.

#### **Automatización**

- Sin intervención manual de operador
- Evita pólizas "zombie" vigentes indefinidamente

#### **Auditoría**

- Cada cancelación automática se loguea
- Rastreable: "cancelada por stock file_id=X"

### Trade-Offs

| Aspecto | Ventaja | Desventaja |
|---------|---------|-----------|
| **Automatización** | Reduce trabajo manual | Requiere confianza en stock |
| **Data integrity** | Pólizas sincronizadas | Posible cancelación accidental |

---

## 12. Dedup por SHA-256 de Contenido

### Decisión

Calcular SHA-256 de archivo completo. Si ya procesado (status PROCESSED o SKIPPED) → archivo actual = SKIPPED.

```go
hasher := sha256.New()
io.Copy(io.MultiWriter(tmp, archive, hasher), remoteFile)
fileHash := hex.EncodeToString(hasher.Sum(nil))

if s.store.FileHashAlreadyProcessed("", fileHash) {
  return nil, fileHash, archivePath, "", errDuplicateFileHash
}
```

### Razones

#### **Prevenir Re-procesamiento**

- Operador ejecuta scan múltiples veces
- Archivo no cambiado → omitir silenciosamente
- Evita duplicar pólizas en BD

#### **Robustez**

- Base en contenido, no en nombre (archivo renombrado sigue detectado)
- Resistente a tampering (SHA-256 collision unlikely)

### Trade-Offs

| Aspecto | Ventaja | Desventaja |
|---------|---------|-----------|
| **Confiabilidad** | Robusto contra cambios de nombre | I/O overhead (calcular hash) |
| **Complejidad** | Simple (una pasada) | Requiere tabla dedup en BD |

---

## Resumen: Matriz de Decisiones

| Componente | Tecnología | Razón Principal | Alternativa Rechazada |
|------------|-----------|-----------------|----------------------|
| Backend | Go 1.23 | Performance, simplicity | Python (más overhead), Node (menos typed) |
| API HTTP | stdlib http | Suficiente, sin deps | Gin (framework, complejidad) |
| Query Builder | Squirrel | Type-safe, no ORM | GORM (overhead), escribir SQL |
| Frontend | React 19 + TS | SPA ideal, dev experience | Vue (menos adoption), Svelte (new) |
| Build Frontend | Vite | Fast, modern | Webpack (lento), Create React App (outdated) |
| DB | MySQL 8.0+ | ACID, reliable, conocido | NoSQL (eventual consistency), PostgreSQL (overkill) |
| SFTP | golang.com/x/crypto/ssh | Native, seguro | FTP (inseguro), HTTP upload (mas lento) |
| Archive Storage | Disk local | Simple, auditable | S3 (complejidad, costo) |
| Email | SendGrid | Fiable, adjuntos | SMTP local (mantenimiento) |
| Concurrency | Worker pool (goroutines) | Real parallelism | Sync request (bloquea), queues externas (Redis) |
| Validación | Declarativa + Procedural | Flexible, mantenible | Pure declarative (limita), Pure procedural (no configurable) |
| Atomicidad | File-level gate | Calidad garantizada | Per-row skips (auditoría imposible) |

