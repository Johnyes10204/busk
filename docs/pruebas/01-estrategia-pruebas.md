# 01. Estrategia de Pruebas — Busk Seguros

## Resumen Ejecutivo

Busk Seguros implementa **143 tests unitarios puros** (sin BD, SFTP, HTTP) que cubren:
- Validación de planes, primas y valores asegurados (MAPFRE)
- Parsing de fechas en múltiples formatos y contextos
- Cálculos de primas, tasas y plazos (Bolívar)
- Cancelaciones masivas y validaciones de stock
- Generación de reportes XLSX

## Alcance

### Productos Cubiertos
- **MAPFRE:** Vida Voluntario (PLAN 1/2/3), AP Menores, AP Cáncer, Stock, Anulación Masiva
- **BOLÍVAR:** Deudores Banco (Micro/Pyme), Deudores ESAL (Micro/Pyme), Stock

### Qué se Prueba
| Área | Cobertura |
|------|-----------|
| Validaciones producto | Planes, primas, valores asegurados |
| Parsing fechas | DMY (único válido), ISO, Excel serial, ambiguas |
| Cálculos | Primas esperadas, tasas factor, plazos crédito |
| Mapeo datos | Extracción columnas, alias, porcentajes |
| Cancelaciones MAPFRE | Anulación masiva, stock match |
| Edad | Límites 18-75.997, interpolación activación |
| Reportes | XLSX cliente/email, consolidación |
| Etiquetado | Incidencias vs informativos |

### Qué NO se Prueba
- HTTP handlers (main.go routes) → **Mitigation:** Postman collection
- Frontend React → **Mitigation:** Manual QA
- SFTP real → **Mitigation:** tools/sftpconnect para conectividad
- MySQL → **Mitigation:** Migrations automáticas al startup
- Email SendGrid → **Mitigation:** Silenciado si env var vacía

## Niveles de Prueba

### Unitarias (143 tests)
- **Definición:** Funciones aisladas, entrada/salida determinística
- **Ubicación:** `internal/processor/*_test.go` (17 archivos), store, validationnotes
- **Característica:** Todas PURAS — no tocan BD, SFTP, global state
- **Ejecución:** `go test ./...` (~2 segundos)

### Integración (Manual)
- **Requisitos:** MySQL + API ejecutando
- **Escenarios:** /bootstrap, /scan, /progress, /files/retry, /validation-report
- **Herramienta:** Postman collection en `docs/postman/`

### E2E (Manual)
- **Flujo:** SFTP → scan → policies en BD → reportes → email
- **Herramienta:** Frontend Admin o curl + Postman

## Criterios de Entrada/Salida

**Entrada:**
- Go 1.23+ para unitarias
- MySQL + SFTP + API para integración
- Todo anterior + Frontend para E2E

**Salida:**
- Unitarias: PASS/FAIL
- Integración: Archivo PROCESSED, pólizas en BD, email enviado
- E2E: Flujo completo sin excepciones, reportes descargables

## Cobertura Estimada

| Módulo | Tests | Cobertura | Estado |
|--------|-------|-----------|--------|
| processor (core validation) | 100+ | ~70% | ALTA |
| store (reporting) | 20+ | ~50% | MEDIA |
| validationnotes | 5+ | ~80% | ALTA |
| main.go (HTTP) | 0 | 0% | NO PROBADO |
| config | 0 | 0% | NO PROBADO |
| model | 0 | 0% | NO PROBADO |

## Riesgos y Mitigaciones

| Riesgo | Mitigación |
|--------|-----------|
| Plan/prima mismatch tag debe ser `REVISAR PRIMA (PLAN)` | TestValidarPlanMapfre_PrimaNoCoincidePlan + CLAUDE.md |
| Parsing ambiguo de fechas (03-06-26) | TestParseBolivarFechaInclusion_030626EsDMY valida DMY único |
| Collision prefijos MAPFRE/Bolívar | TestSeedFilePrefixes_NoCrossInsurer |
| Bloqueo nivel archivo (1 fila falla = TODO falla) | TestValidationReport_* valida consolidación |
| Prima con observación no emite incidencia | TestApplyBolivarDiagramRules_Tasa23ObservacionSuavizaPrima |

## Matriz Trazabilidad (Requisitos → Tests)

### MAPFRE Vida
- Plan válido → TestValidarPlanMapfre_PorNombrePlan
- Prima ↔ Plan → TestValidarPlanMapfre_PrimaNoCoincidePlan
- Valor asegurado obligatorio → TestValidarPlanMapfre_ValorAseguradoObligatorio
- Valor asegurado ↔ Plan → TestValidarPlanMapfre_ValorAseguradoNoCoincide

### MAPFRE ACC Menores
- Plan 1: dos primas válidas (7800, 7410) → TestValidarPlanMapfre_AccPlan1DosPrimas

### MAPFRE Anulación Masiva
- Fin proyectado ≠ activación → TestMapfreCancelacionViolaciones_mismoDia
- Mes archivo ↔ observación → TestMapfreMesEtiquetaCancelacion_obsYArchivo
- Póliza debe estar en stock → TestMapfreCancelacionViolacionesStock_noEnStock

### Bolívar Deudores
- Tasa factor parsing → TestBolivarTasaFactorDesdeRaw
- Prima esperada → TestBolivarPrimaEsperada_Anexo4
- Prima con observación E.4 → TestApplyBolivarDiagramRules_Tasa23ObservacionSuavizaPrima
- Prima sin observación es incidencia → TestApplyBolivarDiagramRules_Tasa23SinObservacionIncidencia
- Edad 18-75.997 → TestEdadMax75Anos364Dias, TestEdad18Inclusive
- Edad solo bloquea si deuda > 20M → TestBolivarEdadFueraRango_SoloConDeudaMayor20M
- Fecha inclusión DMY → TestParseBolivarFechaInclusion_DMY_Enero20

## Conclusión

Cobertura **exhaustiva en lógica core** (validación, parsing, transformación). **Gaps en HTTP, BD integración, frontend** mitigados con QA manual + Postman.

**Mejoras futuras:**
1. Tests de integración con testcontainers MySQL
2. Jest para frontend React
3. Fixtures XLSX reales en test directories
4. CI/CD con `go test -v -race -cover`
