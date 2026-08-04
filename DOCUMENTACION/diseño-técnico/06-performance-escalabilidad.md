# Performance, Escalabilidad y Tunables

## 1. Arquitectura de Performance

Busk es diseñado para máquina única con workers concurrentes, no distribuido.

```
         ┌──────────────────────────────────────────┐
         │    Single Server (1 máquina)             │
         │                                          │
         │  ┌─────────────────────────────────┐    │
         │  │  Go API + Worker Pool           │    │
         │  │  • 2-8 workers (configurable)   │    │
         │  │  • 256-job queue buffer         │    │
         │  │  • ~50-100 MB RAM baseline      │    │
         │  └─────────────────────────────────┘    │
         │                  ↕                       │
         │  ┌─────────────────────────────────┐    │
         │  │  MySQL 8.0+ (InnoDB)            │    │
         │  │  • Índices en product_id,       │    │
         │  │    credit_number, file_hash     │    │
         │  │  • Connection pool (25 open)    │    │
         │  └─────────────────────────────────┘    │
         │                  ↕                       │
         │  ┌─────────────────────────────────┐    │
         │  │  Disk Storage (LOCAL)           │    │
         │  │  • FILES_ARCHIVE_DIR            │    │
         │  │  • REPORTS_ARCHIVE_DIR          │    │
         │  │  • I/O bottleneck potencial     │    │
         │  └─────────────────────────────────┘    │
         │                  ↕                       │
         │  ┌─────────────────────────────────┐    │
         │  │  SFTP Bridge                    │    │
         │  │  • Rate limite por conexión     │    │
         │  │  • Timeout: 30s (default SSH)   │    │
         │  └─────────────────────────────────┘    │
         │                                          │
         └──────────────────────────────────────────┘
```

---

## 2. Throughput Teórico

### Supuestos

- Archivo promedio: 500 filas, 3 MB XLSX
- Procesamiento por fila: ~5 ms (validación + reglas)
- I/O por archivo: ~1 s (descarga SFTP + guardado local)
- Inserción 500 filas: ~500 ms

**Por archivo:** 1 s (I/O) + 2.5 s (validación: 500 × 5ms) + 0.5 s (insert) + 0.5 s (email) = ~4.5 s

**Con 2 workers:**
- Worker 1: procesa archivo A (4.5 s)
- Worker 2: procesa archivo B (4.5 s en paralelo)
- Throughput: 2 archivos / 4.5 s = **0.44 archivos/segundo = ~26 archivos/minuto**

### Con Optimizaciones

- Archivo pequeño (100 filas): ~1.5 s → **80 archivos/min (4 workers)**
- Archivo grande (2000 filas): ~10 s → **6 archivos/min (2 workers)**

---

## 3. Configuración de Workers

### 3.1 Variable de Entorno

```bash
export PROCESSOR_WORKERS=4   # default: 2
```

### 3.2 Recomendaciones por Hardware

| CPU Cores | RAM | Workers Recomendado | Casos de Uso |
|-----------|-----|-------------------|--------------|
| 1-2 | 1-2 GB | 1-2 | Dev, small deployments |
| 4 | 4-8 GB | 3-4 | Medium production |
| 8 | 16+ GB | 6-8 | High-volume production |
| 16+ | 32+ GB | 10-16 | Enterprise (pero ver bottleneck) |

### 3.3 Impacto de Aumentar Workers

```
Workers=1:  Baseline throughput
Workers=2:  ~1.9x throughput (casi lineal)
Workers=4:  ~3.5x throughput (con contención MySQL)
Workers=8:  ~4.5x throughput (hit MySQL/disk bottleneck)
```

**A partir de 8 workers, mejora diminuye** (ley de Amdahl: 20-30% del tiempo es serializado).

---

## 4. Tuning de MySQL

### 4.1 Parámetros Clave

```sql
-- /etc/mysql/my.cnf

[mysqld]
# Buffer pool: 75% de RAM disponible
innodb_buffer_pool_size = 12G            # Para 16 GB RAM

# I/O
innodb_io_capacity = 2000                # IOPS del disk
innodb_io_capacity_max = 4000            # IOPS máximo

# Connections
max_connections = 100                    # Busk usa ~25
max_allowed_packet = 64M                 # Reportes XLSX grandes

# Query cache (MySQL 5.7) - disabled en 8.0+
query_cache_size = 0                     # No usar

# Logging
slow_query_log = ON
long_query_time = 2                      # Log queries > 2s

# Replicación (si aplica)
server-id = 1
log_bin = mysql-bin
binlog_format = ROW
```

### 4.2 Índices Críticos

Verificar que existen:

```sql
-- En tabla policies
SHOW INDEX FROM policies;

-- Deben existir:
-- • idx_product_status (product_id, policy_status)
-- • idx_credit (credit_number)
-- • idx_doc (document_number)
-- • idx_file (file_id)

-- Si faltan, crear:
ALTER TABLE policies 
ADD INDEX idx_product_status (product_id, policy_status),
ADD INDEX idx_credit (credit_number),
ADD INDEX idx_doc (document_number),
ADD INDEX idx_file (file_id);

-- En tabla product_formats (para matching)
ALTER TABLE product_formats
ADD INDEX idx_file_prefix (file_prefix),
ADD INDEX idx_active_prefix (active, file_prefix);
```

### 4.3 Monitoreo

```bash
# Terminal 1: Ver queries lentas
mysql -u root -p -e "SELECT * FROM INFORMATION_SCHEMA.PROCESSLIST WHERE time > 5 AND command != 'Sleep';"

# Terminal 2: Ver status
mysqladmin -u root -p status -i 2

# Ver tamaño de tablas
SELECT table_name, ROUND(((data_length + index_length) / 1024 / 1024), 2) AS size_mb
FROM information_schema.TABLES
WHERE table_schema = 'busk'
ORDER BY size_mb DESC;
```

---

## 5. Tuning de I/O en Disco

### 5.1 Almacenamiento de Archivos

Archivos se guardan en `FILES_ARCHIVE_DIR` (default `./data/files-archive/`).

**Problema:** SSD podría llenar rápidamente si no hay rotación.

```bash
# Limpieza automática (cleanup.sh)
#!/bin/bash
find /var/busk/archive -name "file_*" -mtime +90 -delete  # Borrar > 90 días
```

### 5.2 Particionamiento (Si Base Crece)

Para > 1M pólizas, particionar tabla `policies`:

```sql
ALTER TABLE policies 
PARTITION BY RANGE (YEAR(created_at)) (
  PARTITION p2024 VALUES LESS THAN (2025),
  PARTITION p2025 VALUES LESS THAN (2026),
  PARTITION p2026 VALUES LESS THAN (2027),
  PARTITION p2099 VALUES LESS THAN MAXVALUE
);
```

### 5.3 Compresión de Archivos Archivados

```bash
# Comprimir reportes que tienen > 30 días
find /var/busk/reports-archive -name "*.xlsx" -mtime +30 -exec gzip {} \;

# Actualizar referencia en BD:
UPDATE processed_files 
SET report_archive_path = CONCAT(report_archive_path, '.gz')
WHERE DATE(processed_at) < DATE_SUB(NOW(), INTERVAL 30 DAY);
```

---

## 6. Reducción de Latencia

### 6.1 Paralelismo de Validación

**Actual:** Validar fila por fila (secuencial, ~5ms por fila).

**Optimización:** Validar N filas en paralelo.

```go
// Futuro: procesar filas en chunks
type validationJob struct {
  rowStart int
  rowEnd   int
  values   []map[string]string
}

// Distribuir chunks a worker goroutines
var wg sync.WaitGroup
for i := 0; i < len(rows); i += 100 {  // Chunks de 100 filas
  chunk := &validationJob{rowStart: i, rowEnd: i + 100}
  wg.Add(1)
  go func(job *validationJob) {
    defer wg.Done()
    for j := job.rowStart; j < job.rowEnd; j++ {
      // Validar fila j
    }
  }(chunk)
}
wg.Wait()
```

Impacto: ~2x speedup en archivos grandes (1000+ filas).

### 6.2 Caching de Reglas Producto

**Actual:** Buscar producto cada vez → query MySQL.

**Futuro:** Cache en memoria (con TTL).

```go
type productCache struct {
  mu      sync.RWMutex
  cache   map[string]model.Product
  ttl     time.Duration
  lastRefresh time.Time
}

func (c *productCache) Get(productID string) model.Product {
  c.mu.RLock()
  defer c.mu.RUnlock()
  
  if time.Since(c.lastRefresh) < c.ttl {
    return c.cache[productID]
  }
  
  // Refresh desde BD
  c.mu.RUnlock()
  c.refresh()
  c.mu.RLock()
  return c.cache[productID]
}
```

Impacto: ~10-20% latencia reducida en matching.

---

## 7. Límites de Escalado

### 7.1 Throughput Máximo en 1 Servidor

Con optimizaciones y 16 workers:

```
Scenario 1: Archivos pequeños (100 filas)
  → ~150 archivos/min = 15K pólizas/min

Scenario 2: Archivos grandes (2000 filas)
  → ~10 archivos/min = 20K pólizas/min (pero CPU saturada)

Scenario 3: Archivos muy grandes (5000 filas)
  → ~3 archivos/min = 15K pólizas/min (Memory, CPU, Disk I/O contenciosos)
```

### 7.2 Bottlenecks Identificados

| Component | Bottleneck | Síntoma | Solución |
|-----------|-----------|---------|---------|
| **MySQL** | Connection pool | "too many connections" | Aumentar max_connections, reduce workers |
| **Disk I/O** | SFTP + local archive | 100% disk utilización | SSD, async writes, cloud storage |
| **CPU** | Parsing XLSX grandes | 100% CPU, runing queue | Reduce workers, optimize rules |
| **RAM** | Archivos > 500 MB | OOM killer | Streaming parser, no load full en memory |
| **Network** | SFTP ancho de banda | Descarga slow | Nearline SFTP, compresión (gzip) |

### 7.3 Beyond Single Server

Para > 50K pólizas/minuto, requiere **arquitectura distribuida**:

```
                    ┌──────────────────────────────┐
                    │   Load Balancer (nginx)      │
                    └──────────────┬───────────────┘
                                   │
        ┌──────────────────────────┼──────────────────────────┐
        │                          │                          │
        v                          v                          v
   ┌─────────────┐           ┌──────────────┐          ┌──────────────┐
   │ Busk API #1 │           │ Busk API #2  │          │ Busk API #3  │
   │ Workers: 8  │           │ Workers: 8   │          │ Workers: 8   │
   └──────────────┘          └──────────────┘          └──────────────┘
         │                         │                        │
         └─────────────────────────┼────────────────────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    │                             │
                    v                             v
          ┌──────────────────┐         ┌──────────────────┐
          │   MySQL Master   │         │   MySQL Slave    │
          │   (write)        │         │   (read-only)    │
          │                  │         │  Replication     │
          │  [40 connections]│         │  [40 connections]│
          └──────────────────┘         └──────────────────┘
                    │
         ┌──────────┴──────────┐
         │                     │
         v                     v
   [Shared Storage]      [Shared Storage]
   FILES_ARCHIVE        REPORTS_ARCHIVE
   (NFS / Glusterfs)    (NFS / Glusterfs)
```

**Trade-offs:**
- Complejidad de deployment ↑↑
- Costo infrastructure ↑↑
- Availability ↑
- Throughput máximo: ~100K pólizas/min (3 servidores)

---

## 8. Profiling y Debugging

### 8.1 Go Profiling

```bash
# CPU profiling (runtime)
go run -cpuprofile=cpu.prof services/api/main.go

# Memory profiling
go run -memprofile=mem.prof services/api/main.go

# Analizar
go tool pprof cpu.prof
(pprof) top10      # Top 10 functions por CPU
(pprof) list processByName  # Código línea-a-línea
```

### 8.2 Benchmarking

```bash
cd services/api/internal/processor

# Benchmark validación
go test -bench=BenchmarkValidateFile -benchtime=10s

# Con memoria
go test -bench=BenchmarkValidateFile -benchmem
```

Ejemplo output:
```
BenchmarkValidateFile/500rows-8    300 ns/row  5000 B/row
BenchmarkValidateFile/5000rows-8   4800 ns/row 5200 B/row
```

### 8.3 Tracing de Requests

Agregar contexto y timing:

```go
func (s *Service) processOne(src fileSource, job queuedJob) model.FileProcessRecord {
  start := time.Now()
  
  s.updateProgress(job.ID, job.FileName, "PROCESSING", 5, "iniciando", "", "")
  t1 := time.Now()
  
  // Stage 1: Identify product
  products := s.store.FindProductFormatCandidates(job.FileName)
  t2 := time.Now()
  
  // Stage 2-4: Validate file
  policies, fileHash, _, selectedProductID, err := validateFile(...)
  t3 := time.Now()
  
  // Stage 5: Insert policies
  if err := s.store.InsertPolicies(policies); err != nil {
    // ...
  }
  t4 := time.Now()
  
  log.Printf("[timing] file_id=%s stages: product_match=%dms validate=%dms insert=%dms total=%dms",
    job.ID,
    t2.Sub(t1).Milliseconds(),
    t3.Sub(t2).Milliseconds(),
    t4.Sub(t3).Milliseconds(),
    time.Since(start).Milliseconds(),
  )
  
  return rec
}
```

---

## 9. Monitoreo en Producción

### 9.1 Métricas Clave

Exportar a Prometheus:

```go
var (
  processedFilesTotal = prometheus.NewCounterVec(
    prometheus.CounterOpts{
      Name: "busk_processed_files_total",
      Help: "Total archivos procesados",
    },
    []string{"status"},  // PROCESSED, ERROR, SKIPPED
  )
  
  policiesInserted = prometheus.NewCounter(
    prometheus.CounterOpts{
      Name: "busk_policies_inserted_total",
      Help: "Total pólizas insertadas",
    },
  )
  
  processingDurationSeconds = prometheus.NewHistogram(
    prometheus.HistogramOpts{
      Name: "busk_processing_duration_seconds",
      Help: "Duración de procesamiento de archivos",
      Buckets: []float64{1, 2, 5, 10, 30, 60, 120},  // 1s, 2s, ..., 2m
    },
  )
  
  queueSize = prometheus.NewGauge(
    prometheus.GaugeOpts{
      Name: "busk_job_queue_size",
      Help: "Tamaño actual de la cola de trabajo",
    },
  )
)
```

Endpoint Prometheus:

```go
mux.Handle("/metrics", promhttp.Handler())
```

### 9.2 Alertas (Prometheus)

```yaml
groups:
  - name: busk
    rules:
      - alert: HighErrorRate
        expr: rate(busk_processed_files_total{status="ERROR"}[5m]) > 0.1
        for: 5m
        annotations:
          summary: "Error rate > 10% en últimos 5 minutos"
      
      - alert: QueueOverflow
        expr: busk_job_queue_size > 200
        for: 2m
        annotations:
          summary: "Cola de trabajo cerca de capacidad máxima"
      
      - alert: SlowProcessing
        expr: histogram_quantile(0.99, busk_processing_duration_seconds) > 30
        for: 10m
        annotations:
          summary: "P99 de latencia > 30 segundos"
```

### 9.3 Dashboards Grafana

Paneles recomendados:

```json
{
  "panels": [
    { "title": "Files Processed / minute", "expr": "rate(busk_processed_files_total[1m])" },
    { "title": "Policies Inserted / minute", "expr": "rate(busk_policies_inserted_total[1m])" },
    { "title": "Error Rate %", "expr": "rate(busk_processed_files_total{status='ERROR'}[5m]) * 100" },
    { "title": "Processing Duration P50/P95/P99", "expr": "histogram_quantile([0.5, 0.95, 0.99], busk_processing_duration_seconds)" },
    { "title": "Queue Size", "expr": "busk_job_queue_size" },
    { "title": "MySQL Connections", "expr": "busk_mysql_open_connections" },
    { "title": "Disk I/O (bytes/sec)", "expr": "rate(node_disk_io_bytes_written_total[1m])" }
  ]
}
```

---

## 10. Checklist de Optimización

### Para Development

- [ ] Usar 2 workers (default)
- [ ] MySQL local (dev docker container)
- [ ] Archivos pequeños para testing
- [ ] Memory profiler para verificar leaks

### Para Staging

- [ ] Aumentar a 4-6 workers
- [ ] MySQL con índices verificados
- [ ] Archivos realistas (tamaño/cantidad)
- [ ] Load test con 10-20 archivos concurrentes

### Para Producción

- [ ] Tuning MySQL (buffer pool, connections, slow queries)
- [ ] SSD para FILES_ARCHIVE_DIR
- [ ] SFTP nearline si posible
- [ ] Monitoreo Prometheus + Grafana
- [ ] Alertas configuradas
- [ ] Backup automático MySQL (diario)
- [ ] Rotación de archivos antiguos (90+ días)
- [ ] HTTPS/TLS en proxy
- [ ] Rate limiting en load balancer

---

## 11. Resumen de Capacidades

| Métrica | Valor |
|---------|-------|
| **Throughput máximo (1 servidor, 16 workers)** | ~150 archivos/min = 30K pólizas/min |
| **Latencia archivo pequeño (100 filas)** | ~1.5 s |
| **Latencia archivo grande (2000 filas)** | ~10 s |
| **Memory baseline API** | ~50-100 MB |
| **Memory por worker** | ~5-10 MB |
| **DB connections por API** | ~25 (pool size) |
| **File queue capacity** | 256 jobs |
| **Max page size (políticas)** | 200 |
| **Replication lag (MySQL)** | Tipicamente < 1s |
| **SFTP download speed** | Limitado por red |
| **Email delivery** | ~2s via SendGrid |

---

## 12. Roadmap de Escalabilidad

### Fase 1 (Actual)
- [ ] Single server, 2-8 workers
- [ ] MySQL local o nearline
- [ ] Disk archive local

### Fase 2 (10K pólizas/min)
- [ ] Multi-worker tuning (8-16)
- [ ] MySQL master-slave replication
- [ ] Prometheus monitoring

### Fase 3 (50K pólizas/min)
- [ ] 3x servidores Busk API
- [ ] Kafka/Redis job queue (si needed)
- [ ] MySQL read replicas (3-5)
- [ ] NFS/Glusterfs para archive

### Fase 4 (100K+ pólizas/min)
- [ ] Kubernetes cluster
- [ ] Multi-region deployment
- [ ] S3 para archivado
- [ ] Distributed tracing (Jaeger)

