# Observabilidad y Trazabilidad (Dashboards)

Esta guia deja monitoreo gratis con Grafana + Prometheus, y una ruta para Datadog/New Relic usando free tier.

## 1) Metricas que ya expone el API

Ruta de scraping:

- `GET /metrics`

Metricas principales:

- `busk_http_requests_total` (metodo, ruta, status)
- `busk_http_request_duration_seconds` (latencia)
- `busk_http_requests_in_flight`
- `busk_ftp_scans_total` (success/error)
- `busk_ftp_scan_duration_seconds`
- `busk_ftp_files_found`
- `busk_pipeline_runs_total` (success/partial/error)
- `busk_pipeline_run_duration_seconds`
- `busk_pipeline_files_processed_total`
- `busk_pipeline_files_errors_total`
- `busk_file_processing_total` (archivos por producto y estado)
- `busk_records_processed_total` (registros validos/invalidos por producto)
- `busk_reconciliation_events_total` (inclusions/cancellations/novelties)
- `busk_policies_persisted_total`
- `busk_policies_status_current` (ACTIVE/FROZEN/CANCELLED por producto)

## 2) Levantar dashboards gratis (Grafana OSS + Prometheus)

Desde la raiz del repo:

```bash
cd observability
docker compose -f docker-compose.observability.yml up -d
```

Accesos:

- Prometheus: [http://localhost:9090](http://localhost:9090)
- Grafana: [http://localhost:3001](http://localhost:3001)
  - user: `admin`
  - pass: `admin`

El dashboard `Busk API Overview` se carga automaticamente por provisioning.
Tambien se carga `Busk Trazabilidad Negocio` con foco en archivos, registros y polizas congeladas.
Ademas se carga **Busk Observabilidad 360** (`busk-observabilidad-360.json`): Golden Signals (RPS, P95/P99, errores, saturacion), paneles estilo APM por ruta, KPIs de negocio, FTP/pipeline y una seccion guia para ampliar con Loki, Tempo, node_exporter y cAdvisor.

## 3) Requisitos para que scrapee bien

- El API debe estar corriendo en `localhost:3000`.
- Si corre en otro puerto, actualiza `observability/prometheus.yml`.

### Checklist rapido si no ves datos

1. Abre `http://localhost:3000/metrics` y valida que aparezcan metricas `busk_`.
2. En Prometheus (`http://localhost:9090/targets`) valida que el target `busk-api` este **UP**.
3. Genera trafico real:
   - `GET /api/v1/files`
   - `POST /api/v1/process/scan`
4. Espera 15-30 segundos (intervalo de scrape) y recarga Grafana.
5. Si el API corre fuera de Docker, usa target `host.docker.internal:3000` (ya incluido por defecto).

## 4) Tableros de trazabilidad recomendados

1. **API Salud**
   - RPS
   - Latencia p95
   - Errores 4xx/5xx
2. **FTP Scanner**
   - Scans OK vs Error
   - Duracion de scan
   - Archivos detectados por scan
3. **Pipeline Procesamiento**
   - Runs por estado (success/partial/error)
   - Archivos procesados y con error
   - Duracion por corrida
4. **Negocio Operativo**
   - Lotes por estado
   - Diff de inclusiones/novedades por archivo (via API/DB)

## 5) Datadog y New Relic (free tier)

### Datadog

- Tiene trial/free tier limitado.
- Puedes consumir Prometheus con Datadog Agent (OpenMetrics) apuntando a `http://host.docker.internal:3000/metrics`.
- KPIs recomendados en Datadog dashboard:
  - `sum:busk_file_processing_total{status:success}.as_count()`
  - `sum:busk_records_processed_total{status:valid}.as_count()`
  - `sum:busk_records_processed_total{status:invalid}.as_count()`
  - `sum:busk_reconciliation_events_total{type:cancellations}.as_count()`
  - `sum:busk_policies_status_current{status:FROZEN}`
- Recomendado si ya tienen cuenta activa o quieren compartir tableros con negocio sin administrar Grafana.

### New Relic

- Tiene free tier con limites mensuales.
- Puedes integrar por OTEL SDK/exporter o Prometheus remote write (segun plan).
- Recomendado si quieren monitoreo SaaS sin mantener Grafana.

## 6) Recomendacion practica

Para "gratis" y control total:

- **Produccion inicial**: Grafana OSS + Prometheus (ya listo en este repo).
- **Paso 2 opcional**: exportar las mismas metricas a New Relic o Datadog si el negocio requiere alertas SaaS centralizadas.
