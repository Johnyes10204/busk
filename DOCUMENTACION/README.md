# DOCUMENTACIÓN — Busk Seguros

Documentación exhaustiva del sistema de procesamiento de pólizas de seguros: Arquitectura, Especificación Funcional y Pruebas.

**Versión:** 2026-08-04  
**Estado:** ✅ Completa y estable  
**Listo para:** Word/PDF con Claude Desktop

---

## 📚 Estructura

```
DOCUMENTACION/
├── README.md (este archivo)
├── diseño-técnico/
│   ├── README.md (índice + guía de lectura)
│   ├── 01-arquitectura.md
│   ├── 02-componentes.md
│   ├── 03-decisiones-tecnicas.md
│   ├── 04-patrones-diseño.md
│   ├── 05-seguridad.md
│   └── 06-performance-escalabilidad.md
│
├── especificacion-funcional/
│   ├── README.md (índice + guía de lectura)
│   ├── 01-flujos-negocio.md
│   ├── 02-reglas-validacion.md
│   ├── 03-casos-uso.md
│   ├── 04-api-endpoints.md
│   ├── 05-mapeos-columnas.md
│   └── 06-ciclo-vida-poliza.md
│
└── pruebas/
    ├── README.md (índice + guía de lectura)
    ├── 01-estrategia-pruebas.md
    ├── 02-casos-prueba.md
    ├── 03-ejecucion-pruebas.md
    ├── 04-cobertura-calidad.md
    └── 05-resultados-pruebas.md
```

---

## 🎯 Empezar Aquí

### ¿Quién Eres?

**👨‍💻 Desarrollador**
1. Lee: [`diseño-técnico/README.md`](diseño-técnico/README.md)
2. Revisa: [`diseño-técnico/01-arquitectura.md`](diseño-técnico/01-arquitectura.md) para entender flujo end-to-end
3. Detalle: [`diseño-técnico/02-componentes.md`](diseño-técnico/02-componentes.md) para tu área

**🧪 QA / Tester**
1. Lee: [`pruebas/README.md`](pruebas/README.md)
2. Casos: [`pruebas/02-casos-prueba.md`](pruebas/02-casos-prueba.md) — 150+ casos detallados
3. Cómo ejecutar: [`pruebas/03-ejecucion-pruebas.md`](pruebas/03-ejecucion-pruebas.md)

**📋 PM / Product**
1. Flujos: [`especificacion-funcional/01-flujos-negocio.md`](especificacion-funcional/01-flujos-negocio.md) — 8 flujos por producto
2. Reglas: [`especificacion-funcional/02-reglas-validacion.md`](especificacion-funcional/02-reglas-validacion.md) — 34 validaciones
3. Casos uso: [`especificacion-funcional/03-casos-uso.md`](especificacion-funcional/03-casos-uso.md) — 9 escenarios reales

**🏗️ Arquitecto / Tech Lead**
1. Visión: [`diseño-técnico/README.md`](diseño-técnico/README.md) — índice por rol
2. Decisiones: [`diseño-técnico/03-decisiones-tecnicas.md`](diseño-técnico/03-decisiones-tecnicas.md) — trade-offs
3. Cobertura: [`pruebas/04-cobertura-calidad.md`](pruebas/04-cobertura-calidad.md) — deuda técnica

**🚀 DevOps / SRE**
1. Performance: [`diseño-técnico/06-performance-escalabilidad.md`](diseño-técnico/06-performance-escalabilidad.md)
2. Seguridad: [`diseño-técnico/05-seguridad.md`](diseño-técnico/05-seguridad.md)
3. CI/CD: [`pruebas/03-ejecucion-pruebas.md`](pruebas/03-ejecucion-pruebas.md) sección 5

---

## 📊 Estadísticas

| Métrica | Valor |
|---------|-------|
| **Total documentos** | 20 Markdown files |
| **Total líneas** | ~12,500 |
| **Total palabras** | ~65,000 |
| **Tablas** | 50+ |
| **Diagramas ASCII** | 8+ |
| **Code snippets** | 200+ |
| **Casos de prueba** | 150+ |
| **Tests unitarios documentados** | 143 |
| **Cobertura global** | 55% (70% processor, 40% store) |

---

## 🎯 Las 3 Secciones

### [1️⃣ Diseño Técnico](diseño-técnico/README.md)

**Qué:** Arquitectura, componentes, decisiones tecnológicas, patrones, seguridad, performance.

**Documentos:**
- `01-arquitectura.md` — Componentes + flujo end-to-end completo (13 stages)
- `02-componentes.md` — API Go, Frontend React, MySQL, SFTP, SendGrid (150+ snippets)
- `03-decisiones-tecnicas.md` — Por qué Go, React, MySQL; trade-offs (12 decisiones)
- `04-patrones-diseño.md` — 8 patrones: Worker Pool, Repository, Pipeline, etc.
- `05-seguridad.md` — 12 vectores de seguridad + mitigaciones implementadas
- `06-performance-escalabilidad.md` — Throughput, tunables, bottlenecks, roadmap

**Para:** Developers, Architects, DevOps

---

### [2️⃣ Especificación Funcional](especificacion-funcional/README.md)

**Qué:** Flujos de negocio, validaciones, casos de uso, API, mapeos de datos, ciclo de vida.

**Documentos:**
- `01-flujos-negocio.md` — 8 flujos: MAPFRE (Vida, ACC, Cáncer, Stock, Anulación) + Bolívar (Banco, ESAL)
- `02-reglas-validacion.md` — 34 validaciones, el GATE crítico (1 fila falla = archivo falla)
- `03-casos-uso.md` — 9 escenarios: stock exitoso, errores, cancelación, SFTP timeout, email falla
- `04-api-endpoints.md` — 11 endpoints (request/response/codes/ejemplos)
- `05-mapeos-columnas.md` — Mapeos XLSX→BD para 8 productos (alias, transformaciones)
- `06-ciclo-vida-poliza.md` — Estados (ACTIVE, FROZEN, MANUAL_REVIEW, CANCELLED) + transiciones

**Para:** PMs, QA, Developers, Business Analysts

---

### [3️⃣ Pruebas](pruebas/README.md)

**Qué:** Estrategia de pruebas, 150+ casos, ejecución (unitarias/integración/E2E), cobertura, resultados.

**Documentos:**
- `01-estrategia-pruebas.md` — Alcance, niveles, cobertura, riesgos, trazabilidad
- `02-casos-prueba.md` — 150+ casos (MAPFRE plans, Bolívar primas, fechas, edad, cancelaciones, reportes)
- `03-ejecucion-pruebas.md` — Cómo ejecutar: `go test`, Postman, E2E, frontend, troubleshooting
- `04-cobertura-calidad.md` — 55% global, por módulo, mejoras prioritarias (HTTP, BD, React)
- `05-resultados-pruebas.md` — 143/143 tests PASS, flakiness 0, comandos rápida referencia

**Para:** QA, Developers, DevOps, Architects

---

## ⚠️ Tests Críticos (5)

Si estos fallan → regresión de negocio:

1. **TestValidarPlanMapfre_PrimaNoCoincidePlan**  
   Tag "REVISAR PRIMA (PLAN)" (nunca "REVISAR PLAN")

2. **TestParseBolivarFechaInclusion_030626EsDMY**  
   Parsing 03-06-26 como 3 junio 2026 (DMY único, no MDY)

3. **TestSeedFilePrefixes_CoverageDownloads**  
   19 archivos + 11 prefijos (lote completo abril 2026)

4. **TestSeedFilePrefixes_NoCrossInsurer**  
   MAPFRE prefixes NO matchean Bolívar

5. **TestApplyBolivarDiagramRules_Tasa23ObservacionSuavizaPrima**  
   Prima con observación E.4 = nota (no incidencia)

---

## 🚀 Comandos Rápidos

```bash
# Ejecutar TODOS los tests
cd services/api && go test ./...

# Tests específico
go test -run TestValidarPlanMapfre_PrimaNoCoincidePlan ./...

# Coverage HTML
go test -coverprofile=c.out ./... && go tool cover -html=c.out

# Pattern (todos los Bolívar)
go test -run "TestBolivar*" ./...

# Race detection
go test -race ./...
```

---

## 📝 Cómo Usar Esta Documentación

### Como Referencia en Desarrollo
```bash
# Buscar regla específica
grep -r "REVISAR PRIMA" DOCUMENTACION/

# Buscar caso de prueba
grep -r "Prima discrepancia" DOCUMENTACION/pruebas/

# Buscar endpoint
grep -r "GET /api/v1/products" DOCUMENTACION/
```

### Para Generar Word/PDF
1. Copiar todos los MDs de esta carpeta
2. Llevar a **Claude Desktop**
3. Usar prompt para generar Word profesional con:
   - Portada, índice, estilos corporativos
   - Tablas con filas alternas
   - Numeración páginas
   - Todos los diagramas ASCII preservados

---

## 🔄 Actualización de Documentación

Cuando cambies código en áreas críticas:

| Cambio | Actualizar |
|--------|-----------|
| Validación plan MAPFRE | `especificacion-funcional/02-reglas-validacion.md` |
| Parsing de fechas | `especificacion-funcional/02-reglas-validacion.md` + `pruebas/02-casos-prueba.md` |
| Prima calculation Bolívar | `especificacion-funcional/02-reglas-validacion.md` + `pruebas/02-casos-prueba.md` |
| Nuevo endpoint API | `especificacion-funcional/04-api-endpoints.md` |
| Nuevo flujo de negocio | `especificacion-funcional/01-flujos-negocio.md` |
| Tests nuevos | `pruebas/02-casos-prueba.md` + `pruebas/05-resultados-pruebas.md` |

---

## 📚 Referencias Externas

- **Código:** `/services/api/internal/`
- **CLAUDE.md:** Reglas de dominio inmutables
- **Postman:** `/docs/postman/Busk_Seguros_API.postman_collection.json`
- **Runbooks:** `/docs/specs/RUNBOOK_API_PASO_A_PASO.md`
- **Docsify:** http://localhost:3000 (en desarrollo)

---

## ✅ Checklist de Lectura

- [ ] Leí el README de mi rol (Dev/QA/PM/Architect/DevOps)
- [ ] Identifiqué los 5 tests críticos
- [ ] Encontré mi área de interés en los 3 documentos principales
- [ ] Guardé los comandos rápidos útiles para mi trabajo

---

**Versión:** 1.0  
**Última actualización:** 2026-08-04  
**Estado:** ✅ Completa, versionada en Git, lista para Word  
**Mantenedor:** Equipo de Desarrollo
