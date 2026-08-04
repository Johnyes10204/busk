# Pruebas — Busk Seguros

Documentación completa de estrategia, ejecución y resultados de pruebas para el sistema de procesamiento de pólizas de seguros.

## 📋 Contenidos

### [01-estrategia-pruebas.md](01-estrategia-pruebas.md)
- **Alcance:** Qué se prueba y qué no
- **Cobertura:** Productos MAPFRE y Bolívar
- **Niveles:** Unitarias (143 tests), Integración, E2E
- **Riesgos:** Matriz de riesgos y mitigaciones
- **Trazabilidad:** Requisitos → Tests

### [02-casos-prueba.md](02-casos-prueba.md)
- **150+ casos de prueba** detallados
- **Secciones:**
  - Planes MAPFRE (11 casos)
  - Parsing de fechas (10 casos)
  - Primas y cálculos Bolívar (4 casos)
  - Edad y rango (4 casos)
  - Cancelaciones MAPFRE (5 casos)
  - Reportes (3 casos)
  - Etiquetado (3 casos)
  - Archivos/Prefijos (2 casos)
- **Estructura:** ID, descripción, precondiciones, steps, resultado esperado
- **Críticos marcados:** ⚠️ para reglas de dominio

### [03-ejecucion-pruebas.md](03-ejecucion-pruebas.md)
- **Cómo ejecutar pruebas unitarias:** `go test ./...`
- **Integración con Postman:** Collection lista en `/docs/postman/`
- **E2E manual:** Setup API + Frontend + SFTP, verificar flujo completo
- **Frontend QA:** Checklist de funcionalidades
- **Troubleshooting:** Errores comunes y soluciones

### [04-cobertura-calidad.md](04-cobertura-calidad.md)
- **Cobertura por módulo:** 55% global (70% processor, 40% store)
- **Complejidad ciclomática:** Funciones ALTA/MEDIA/BAJA
- **Métricas:** Linting, vet, deadcode
- **Falsos negativos/positivos:** Bugs potenciales no detectados
- **Mejoras prioritarias:** HTTP tests, BD integration, Jest frontend

### [05-resultados-pruebas.md](05-resultados-pruebas.md)
- **Sumario:** 143/143 tests PASS (100%), ~2.5 segundos
- **Distribución:** Por área funcional y archivo
- **Tests críticos:** 5 tests que protegen reglas de dominio
- **Interpretación de fallos:** Cómo debuggear si algo falla
- **Flakiness:** Estado (0 flaky), prevención futura
- **Comandos rápida referencia:** go test flags comunes

---

## 🎯 Guía por Rol

### Desarrollador (Frontend/Backend)

1. **Antes de hacer cambios:** Lee [01-estrategia-pruebas.md](01-estrategia-pruebas.md) para entender qué está cubierto
2. **Si modificas validaciones:** Verifica tests en [02-casos-prueba.md](02-casos-prueba.md) relevantes a tu cambio
3. **Para correr tests localmente:** Sigue [03-ejecucion-pruebas.md](03-ejecucion-pruebas.md) sección 1 (Unitarias)
4. **Si test falla:** Consulta [05-resultados-pruebas.md](05-resultados-pruebas.md) sección 2 (Interpretación de fallos)

### QA / Tester

1. **Entender qué se prueba:** [01-estrategia-pruebas.md](01-estrategia-pruebas.md) alcance
2. **Casos de prueba:** [02-casos-prueba.md](02-casos-prueba.md) — 150+ casos para manual testing
3. **Ejecutar E2E:** [03-ejecucion-pruebas.md](03-ejecucion-pruebas.md) sección 3 (E2E)
4. **Verificar cobertura:** [04-cobertura-calidad.md](04-cobertura-calidad.md) para saber dónde hay gaps

### DevOps / SRE

1. **CI/CD setup:** [03-ejecucion-pruebas.md](03-ejecucion-pruebas.md) sección 5 (CI/CD futuro)
2. **Coverage thresholds:** [04-cobertura-calidad.md](04-cobertura-calidad.md) sección 4.3
3. **Monitoreo:** [05-resultados-pruebas.md](05-resultados-pruebas.md) sección 3 (Flakiness monitoring)
4. **Mejoras:** [04-cobertura-calidad.md](04-cobertura-calidad.md) sección 6 (Roadmap)

### Arquitecto / Tech Lead

1. **Visión de cobertura:** [04-cobertura-calidad.md](04-cobertura-calidad.md) sección 1 (Por módulo) + sección 5 (Matriz riesgos)
2. **Críticos a proteger:** [05-resultados-pruebas.md](05-resultados-pruebas.md) sección 3 (Tests críticos)
3. **Deuda técnica:** [04-cobertura-calidad.md](04-cobertura-calidad.md) sección 6 (Mejoras prioritarias)
4. **Decisiones de design:** [01-estrategia-pruebas.md](01-estrategia-pruebas.md) sección "Qué NO se prueba" (justificación de gaps)

---

## 📊 Estadísticas Globales

| Métrica | Valor |
|---------|-------|
| **Tests unitarios** | 143 |
| **Estado** | 100% PASS, 0% FAIL |
| **Tiempo ejecución** | ~2.5 segundos |
| **Cobertura global** | ~55% |
| **Cobertura processor** | 70% |
| **Cobertura store** | 40% |
| **Flakiness** | 0 (determinísticos) |
| **Casos de prueba documentados** | 150+ |

---

## ✅ Tests Críticos

Estos 5 tests protegen reglas de dominio. Si fallan = regresión de negocio:

1. **TestValidarPlanMapfre_PrimaNoCoincidePlan** — Tag "REVISAR PRIMA (PLAN)" (nunca "REVISAR PLAN")
2. **TestParseBolivarFechaInclusion_030626EsDMY** — Parsing 03-06-26 como 3 junio (DMY único)
3. **TestSeedFilePrefixes_CoverageDownloads** — 19 archivos + 11 prefijos (lote completo)
4. **TestSeedFilePrefixes_NoCrossInsurer** — MAPFRE prefixes NO matchean Bolívar
5. **TestApplyBolivarDiagramRules_Tasa23ObservacionSuavizaPrima** — Prima con observación E.4 = nota (no incidencia)

---

## 🚀 Comando Rápido

```bash
# Ejecutar TODOS los tests
cd services/api
go test ./...

# Verbose + coverage
go test -v -cover ./...

# Coverage HTML
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# Test específico
go test -run TestValidarPlanMapfre_PrimaNoCoincidePlan ./...

# Pattern (todos los Bolívar)
go test -run "TestBolivar*" ./...

# Race detection
go test -race ./...
```

---

## 📝 Próximas Mejoras

**Priority 1: HTTP handlers (main.go)**
- Impacto: +400 LOC, 0% → ~50% coverage
- Tool: httptest
- Effort: Medium

**Priority 2: BD Integration (store/*)**
- Impacto: +300 LOC, 40% → ~80% coverage
- Tool: testcontainers MySQL
- Effort: Medium

**Priority 3: Frontend (React)**
- Impacto: +1400 LOC, 0% → ~60% coverage
- Tool: Jest + React Testing Library
- Effort: High

---

## 📚 Referencias

- **Código:** `/services/api/internal/processor/*_test.go`
- **Postman:** `/docs/postman/Busk_Seguros_API.postman_collection.json`
- **Runbooks:** `/docs/specs/RUNBOOK_API_PASO_A_PASO.md`
- **CLAUDE.md:** Reglas de dominio (etiquetas, validaciones críticas)

---

**Versión:** 1.0  
**Última actualización:** 2026-08-04  
**Mantenedor:** Equipo de Desarrollo  
**Próxima revisión:** Después de cambios en core validation
