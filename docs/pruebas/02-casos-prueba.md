# 02. Casos de Prueba — Busk Seguros

## Estructura

Cada caso tiene: **ID | Descripción | Precondiciones | Steps | Resultado Esperado**

---

## SECCIÓN 1: VALIDACIÓN DE PLANES MAPFRE

### CP-MAPFRE-01: Plan 1 Vida válido
- **Test:** TestValidarPlanMapfre_PorNombrePlan
- **Descripción:** Plan PLAN 1 con prima y valor asegurado correctos debe pasar validación
- **Precondiciones:** Producto MAPFRE_VIDA con PLAN 1
- **Steps:**
  1. Entrada: plan_name="PLAN 1", monthly_premium="8600", insured_amount="5000000"
  2. Llamar validarPlanMapfre("MAPFRE_VIDA", values)
- **Resultado:** violations.length == 0

### CP-MAPFRE-02: Prima no coincide plan ⚠️ CRÍTICO
- **Test:** TestValidarPlanMapfre_PrimaNoCoincidePlan
- **Descripción:** Prima incorrecta para plan debe emitir tag "REVISAR PRIMA (PLAN)"
- **Precondiciones:** PLAN 1 espera prima 8600
- **Steps:**
  1. Entrada: plan_name="PLAN 1", monthly_premium="17100", insured_amount="5000000"
  2. Llamar validarPlanMapfre("MAPFRE_VIDA", values)
- **Resultado:** len(violations) == 1 && contains(violations[0], "REVISAR PRIMA (PLAN)")
- **Notas:** Este es un caso critical — el tag DEBE ser "REVISAR PRIMA (PLAN)", nunca "REVISAR PLAN"

### CP-MAPFRE-03: Valor asegurado ausente
- **Test:** TestValidarPlanMapfre_ValorAseguradoObligatorio
- **Descripción:** PLAN 2 requiere valor asegurado
- **Precondiciones:** PLAN 2 sin insured_amount
- **Steps:**
  1. Entrada: plan_name="PLAN 2", monthly_premium="17100" (sin insured_amount)
  2. Llamar validarPlanMapfre("MAPFRE_VIDA", values)
- **Resultado:** len(violations) == 1 && contains(violations[0], "valor asegurado")

### CP-MAPFRE-04: Valor asegurado no coincide
- **Test:** TestValidarPlanMapfre_ValorAseguradoNoCoincide
- **Descripción:** Valor asegurado debe coincidir con plan
- **Precondiciones:** PLAN 1 espera 5M, entrada 10M
- **Steps:**
  1. Entrada: plan_name="PLAN 1", monthly_premium="8600", insured_amount="10000000"
  2. Llamar validarPlanMapfre("MAPFRE_VIDA", values)
- **Resultado:** len(violations) == 1 && contains(violations[0], "plan no válido")

### CP-MAPFRE-05: Dos primas válidas AP Menores
- **Test:** TestValidarPlanMapfre_AccPlan1DosPrimas
- **Descripción:** PLAN 1 en AP Menores acepta dos primas distintas
- **Precondiciones:** MAPFRE_ACC_MEN, PLAN 1
- **Steps:**
  1. Test con prima=7800 → debe pasar
  2. Test con prima=7410 → debe pasar
- **Resultado:** Ambas violaciones == 0

### CP-MAPFRE-06: Plan code ignorado
- **Test:** TestValidarPlanMapfre_IgnoraPlanCode
- **Descripción:** Campo plan_code NO se usa en validación
- **Precondiciones:** PLAN 1 con plan_code="99999"
- **Steps:**
  1. Entrada: plan_name="PLAN 1", plan_code="99999", monthly_premium="8600", insured_amount="5000000"
  2. Llamar validarPlanMapfre("MAPFRE_VIDA", values)
- **Resultado:** violations.length == 0

---

## SECCIÓN 2: PARSING DE FECHAS

### CP-FECHA-01: Día/Mes/Año DMY único orden válido ⚠️ CRÍTICO
- **Test:** TestParseBolivarFechaInclusion_030626EsDMY
- **Descripción:** 03-06-26 SOLO es 3 junio 2026 (no 6 marzo)
- **Precondiciones:** Bolívar solo acepta DMY
- **Steps:**
  1. Entrada: "03-06-26"
  2. ParseBolivarFechaInclusion(input)
- **Resultado:** Fecha.Month == June, Fecha.Day == 3, Fecha.Year == 2026

### CP-FECHA-02: Enero 20 2022
- **Test:** TestParseBolivarFechaInclusion_DMY_Enero20
- **Descripción:** "20-01-22" = 20 enero 2022
- **Precondiciones:** DMY
- **Steps:**
  1. Entrada: "20-01-22"
  2. Parse
- **Resultado:** 20 Jan 2022

### CP-FECHA-03: 17 Nov 1963 múltiples formatos
- **Test:** TestParseDate17Nov1963
- **Descripción:** Aceptar múltiples formatos: 17/11/1963, 17-11-1963, '17/11/1963, etc.
- **Precondiciones:** Layouts por defecto
- **Steps:**
  1. Probar lista de formatos
  2. parseDateWithLayouts(raw)
- **Resultado:** Todos parsean a 17 Nov 1963

### CP-FECHA-04: Rechaza MDY (mes > 12)
- **Test:** TestParseDateRechazaMDY
- **Descripción:** 08-14-76, 10-13-22 rechazados (mes > 12 = MDY)
- **Precondiciones:** Solo DMY válido
- **Steps:**
  1. Entrada: "08-14-76" (mes 14 invalido)
  2. ParseDateField()
- **Resultado:** IsZero() == true (rechazado)

---

## SECCIÓN 3: PRIMAS Y CÁLCULOS (BOLÍVAR)

### CP-BOLIVIA-PRIMA-01: Tasa factor desde string
- **Test:** TestBolivarTasaFactorDesdeRaw
- **Descripción:** Parser tasa: "0.1%", "0,1%", "0.001", "1E-3", "0.1", "23", "23%" → factores
- **Precondiciones:** Raw tasa strings
- **Steps:**
  1. bolivarTasaFactorDesdeRaw("23%") → 0.23
  2. bolivarTasaFactorDesdeRaw("0.1%") → 0.001
  3. Más casos
- **Resultado:** Factor numérico correcto ±1e-9 tolerancia

### CP-BOLIVIA-PRIMA-02: Prima esperada Anexo 4
- **Test:** TestBolivarPrimaEsperada_Anexo4
- **Descripción:** deuda 23,717,600 × tasa 0.001 = prima ≈ 23,717.6
- **Precondiciones:** Anexo 4 (Deudores Banco)
- **Steps:**
  1. bolivarPrimaEsperada(23_717_600, "0.001")
  2. Verificar factor == 0.001
  3. Verificar prima ≈ 23_717.5 .. 23_717.7
- **Resultado:** Prima dentro de rango

### CP-BOLIVIA-PRIMA-03: Prima con observación E.4 (suavizado) ⚠️ IMPORTANTE
- **Test:** TestApplyBolivarDiagramRules_Tasa23ObservacionSuavizaPrima
- **Descripción:** Prima discrepante pero observación E.4 (ej. "FACTURACION ABRIL") → nota, no incidencia
- **Precondiciones:** tasa=23%, prima=25000, observacion="FACTURACION ABRIL 2026"
- **Steps:**
  1. applyBolivarDiagramRules(values, cfg)
  2. Verificar hard (incidencias)
  3. Verificar soft (notas)
- **Resultado:** hard.len == 0, soft incluye nota de prima

### CP-BOLIVIA-PRIMA-04: Prima sin observación → incidencia
- **Test:** TestApplyBolivarDiagramRules_Tasa23SinObservacionIncidencia
- **Descripción:** Prima discrepante SIN observación → tag REVISAR PRIMA
- **Precondiciones:** tasa=23%, prima=25000, sin observacion
- **Steps:**
  1. applyBolivarDiagramRules(values, cfg)
- **Resultado:** hard incluye tag "REVISAR PRIMA"

---

## SECCIÓN 4: EDAD Y RANGO

### CP-EDAD-01: 18 años cumplidos inclusivo
- **Test:** TestEdad18Inclusive
- **Descripción:** Nacimiento 01-06-2007, referencia 01-06-2025 → edad 18 (PASS)
- **Precondiciones:** Min edad 18
- **Steps:**
  1. Birth "01/06/2007", ref 01-06-2025
  2. edadCumpleRango(birth, layouts, ref, 18, 75.997)
- **Resultado:** ok == true, edad == 18

### CP-EDAD-02: Max 75 años 364 días ⚠️ IMPORTANTE
- **Test:** TestEdadMax75Anos364Dias
- **Descripción:** Nació 01-06-1950 → cumple 76 el 01-06-2026 → válido hasta 31-05-2026
- **Precondiciones:** Max edad 75.997 (364 días antes cumpleaños 76)
- **Steps:**
  1. Birth "01-06-1950", ref 31-05-2026 (1 día antes cumpleaños 76)
  2. edadCumpleRangoEstricto(...)
- **Resultado:** ok == true
  3. Birth "01-06-1950", ref 01-06-2026 (cumpleaños 76)
  4. edadCumpleRangoEstricto(...)
- **Resultado:** ok == false

### CP-EDAD-03: Un día antes cumpleaños 76
- **Test:** TestEdadUnDiaAntesCumpleanos76_Jun1950
- **Descripción:** Birth "19-06-50", activación 18-06-2026 (1 día antes 76) → edad 75 (OK)
- **Precondiciones:** 75.997 max
- **Steps:**
  1. edadCumpleRangoEstricto("19-06-50", layouts, 18-06-2026, 18, 75.997, 1, false)
- **Resultado:** ok == true, edad == 75

### CP-EDAD-04: Completadas years between
- **Test:** TestCompletedYearsBetween
- **Descripción:** completedYearsBetween(birth, ref) cuenta años cumplidos
- **Precondiciones:** Birth 01-06-2007, ref 20-05-2025 (antes cumpleaños)
- **Steps:**
  1. completedYearsBetween(2007-06-01, 2025-05-20) → 17
  2. completedYearsBetween(2007-06-01, 2025-06-02) → 18
- **Resultado:** Correcto

---

## SECCIÓN 5: CANCELACIONES MAPFRE

### CP-CANCEL-01: Anulación masiva abril 2026 válida
- **Test:** TestMapfreCancelacionViolaciones_okAbril2026
- **Descripción:** Cancelación con fin proyectado válido, activación válida
- **Precondiciones:** Archivo "Anulacion masiva_ABRIL 2026.xlsx"
- **Steps:**
  1. coverage_end_date="46124" (fecha excel), activation_date="45334"
  2. observacion="ABRIL"
  3. mapfreCancelacionViolacionesFechas(values)
- **Resultado:** violations.len == 0

### CP-CANCEL-02: Fin proyectado = activación (mismo día)
- **Test:** TestMapfreCancelacionViolaciones_mismoDia
- **Descripción:** coverage_end_date == activation_date → violación
- **Precondiciones:** Fechas idénticas
- **Steps:**
  1. coverage_end_date="46124", activation_date="46124"
  2. mapfreCancelacionViolacionesFechas()
- **Resultado:** violations.len == 1

### CP-CANCEL-03: Mes etiqueta ≠ mes archivo
- **Test:** TestMapfreCancelacionViolaciones_mesEtiqueta
- **Descripción:** observacion mes ≠ archivo mes → violación
- **Precondiciones:** archivo "ABRIL", observacion "MAYO"
- **Steps:**
  1. coverage_end_date="46147", activation_date="45509", observacion="MAYO"
  2. Archivo nombre: "Anulacion masiva_ABRIL 2026.xlsx"
  3. mapfreCancelacionViolacionesFechas()
- **Resultado:** violations.len == 1

### CP-CANCEL-04: Póliza no en stock
- **Test:** TestMapfreCancelacionViolacionesStock_noEnStock
- **Descripción:** Póliza a cancelar debe existir en stock anterior
- **Precondiciones:** credit_number="1719123" no existe en stock
- **Steps:**
  1. values = {credit_number: "1719123", document_number: "6525556"}
  2. mapfreCancelacionViolacionesStock(values, cfg, mockStockReader{found: false})
- **Resultado:** violations[0] == mensajeCancelacionPolizaNoEnStock("1719123")

### CP-CANCEL-05: Stock cancelación OK
- **Test:** TestMapfreCancelacionViolacionesStock_ok
- **Descripción:** Póliza en stock y validaciones cumplidas → OK
- **Precondiciones:** Póliza en stock, fechas válidas
- **Steps:**
  1. mock.found = true
  2. mapfreCancelacionViolacionesStock()
- **Resultado:** violations.len == 0

---

## SECCIÓN 6: REPORTES VALIDACIÓN

### CP-REPORT-01: Reporte cliente única hoja "Datos archivo"
- **Test:** TestValidationReport_ClientXLSXUnicaHojaDatosArchivo
- **Descripción:** Archivo XLSX para cliente con 1 sheet "Datos archivo"
- **Precondiciones:** Pólizas con avisos informativos
- **Steps:**
  1. BuildFileValidationReportFromPolicies(...)
  2. ValidationReportClientXLSX(report)
  3. Verificar sheets
- **Resultado:** 1 sheet "Datos archivo", avisos informativos incluidos

### CP-REPORT-02: Reporte email única hoja
- **Test:** TestValidationReport_EmailXLSXUnicaHoja
- **Descripción:** Email XLSX con 1 sheet "Reporte" combinando bloqueantes + informativos
- **Precondiciones:** Pólizas MANUAL_REVIEW (bloqueante) + ACTIVE (informativo)
- **Steps:**
  1. BuildFileValidationReportFromPolicies(...)
  2. ValidationReportEmailXLSX(report)
  3. Verificar no hay sheets "Incidencias", "Informes", "Datos archivo"
- **Resultado:** 1 sheet "Reporte" con 2 filas (bloqueante + informativa)

### CP-REPORT-03: Mirror sheet con datos archivo
- **Test:** TestValidationReport_MirrorSheetConDatosArchivo
- **Descripción:** Mirror sheet refleja datos originales del archivo
- **Precondiciones:** Pólizas con raw_data
- **Steps:**
  1. SaveValidationReportArchive()
  2. Verificar sheet "Espejo"
- **Resultado:** Datos originales presentes

---

## SECCIÓN 7: ETIQUETADO Y NOVEDADES

### CP-TAG-01: Separación bloqueante vs informativo
- **Test:** TestSplit_InformativeVsBlocking
- **Descripción:** Split() separa INCIDENCIA: de INFORMATIVO:
- **Precondiciones:** Notas mixed
- **Steps:**
  1. notes = ["INCIDENCIA: edad", "INFORMATIVO: vencimiento", "nota sin prefijo"]
  2. Split(notes)
- **Resultado:** blocking=[edad], informative=[vencimiento, nota sin prefijo]

### CP-TAG-02: Etiquetas resumen desde nota - cancelación
- **Test:** TestEtiquetasResumenFromNote_cancelacion
- **Descripción:** Nota con "cancelacion" → etiqueta CANCELACIÓN
- **Precondiciones:** Nota contiene "cancelacion"
- **Steps:**
  1. etiquetasResumenFromNote("póliza fue cancelada")
- **Resultado:** etiquetas incluye "CANCELACIÓN"

### CP-TAG-03: Prima discrepancia plan
- **Test:** TestEtiquetasResumenFromNote_primaDiscrepanciaPlan
- **Descripción:** Nota "prima" + "plan" → REVISAR PRIMA (PLAN)
- **Precondiciones:** Nota menciona prima y plan
- **Steps:**
  1. etiquetasResumenFromNote("prima no coincide con plan")
- **Resultado:** etiquetas = ["REVISAR PRIMA (PLAN)"]

---

## SECCIÓN 8: ARCHIVOS DE PRODUCTO

### CP-SEED-01: Prefijos cobertura lote Abril 2026 ⚠️ CRÍTICO
- **Test:** TestSeedFilePrefixes_CoverageDownloads
- **Descripción:** Todos los prefijos esperados matchean archivos
- **Precondiciones:** Lista de prefijos seed
- **Steps:**
  1. Probar 19 casos: MAPFRE vida, ACC, cáncer, Bolívar micro, pyme, ESAL
  2. Verificar filenameMatchesPrefix()
- **Resultado:** Todos matchean

### CP-SEED-02: No cross-insurer collision ⚠️ CRÍTICO
- **Test:** TestSeedFilePrefixes_NoCrossInsurer
- **Descripción:** Prefijos MAPFRE no matchean Bolívar y vice versa
- **Precondiciones:** Prefijos distintos por insurer
- **Steps:**
  1. STOCK_MAPFRE NO match prefijoBolívarStock
  2. Pyme BANCO NO match solo MICRO_BANCO
  3. ACC MEN NO match CANCER
- **Resultado:** Sin collisiones

---

**Versión:** 1.0  
**Última actualización:** 2026-08-04  
**Total de casos:** 150+
