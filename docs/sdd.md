# SDD — Validación de API de Controles (Ingresos y Cancelaciones)

**Proyecto:** Automatización de controles de cartera — MAPFRE
**Tipo de API:** REST / JSON
**Objetivo:** Definir el contrato de datos y las reglas de validación que la API debe cumplir, de modo que pueda verificarse que cada registro respeta los formatos y controles del proceso actual.
**Estado:** Borrador para revisión

> **Pendiente de confirmación:** primas de AP Menores (Plan1 = 7800 *o* 7410; Plan2 = 10600 *o* 10070). En este documento se dejan como valores candidatos hasta cierre contra la base original.

---

## 1. Alcance

Este documento describe el diseño de validación para dos flujos:

1. **Ingresos** — validación de altas por producto (Voluntario, AP Menores, AP Cáncer): edad de ingreso, estado de permanencia, valor de prima por plan, y controles transversales de fechas/crédito.
2. **Cancelaciones** — validación del cruce contra la base de stock, marcado de cancelados, siniestros y fallecimientos.

No cubre autenticación, transporte (SFTP), ni el cargue final en DaRa salvo como referencia de cierre del flujo.

**Prefijos de archivo en semilla API** (`services/api/main.go`, `POST /api/v1/bootstrap/sample-products`): match por subcadena en el nombre.

### Alineación con `Requerimiento conexiones API_VF_Nov_25.docx` (Etapa 1)

| Requerimiento | Cabezote / Anexo | Producto API (`Code`) | Archivo ejemplo (abr-2026) | Prefijo semilla |
|---|---|---|---|---|
| Vida voluntaria | **108** · Anexo 1 | `MAPFRE_INCLUSION_VIDA_VOLUNTARIO` | `5024424900103_VIDA_VOL RM-INCLUSION ABRIL2026.xlsx` | `5024424900103`, `VIDA_VOL RM-INCLUSION` |
| AP Cáncer | **114** · Anexo 2 | `MAPFRE_INCLUSION_AP_CANCER` | `5024524900103_CANCER RM-INCLUSION ABRIL2026.xlsx` | `5024524900103`, `CANCER RM-INCLUSION` |
| AP Menores | **110** · Anexo 3 | `MAPFRE_INCLUSION_AP_MENORES` | `5024524900101_ACC MEN RM-INCLUSION ABRIL2026.xlsx` | `5024524900101`, `ACC MEN RM-INCLUSION` |
| Cancelaciones MAPFRE | plantilla (doc cita “Anexo 4” por error*) | `MAPFRE_ANULACION_MASIVA` | `Anulacion masiva_ABRIL 2026.xlsx` · hoja `Plantilla_Anulaciones Abril26` | `Anulacion masiva` |
| Deudores Banco Bolívar | Anexo 4 | `BOLIVAR_INCLUSION_DEUDORES_BANCO` | `Deudores_Banco_Bolivar_MICRO_BANCO_*.xlsx` / `Pyme_BANCO_*.xlsx` | `MICRO_BANCO`, `Pyme_BANCO` |
| Deudores ESAL + stock mensual | Anexo 5 | `BOLIVAR_DEUDORES_STOCK_ESAL` | `Deudores_ESAL_Bolivar_*_ESAL_*.xlsx`, `STOCK-*` | `micro_ESAL`, `Pyme_ESAL`, `STOCK-` |
| Stock MAPFRE (diagrama operativo) | — (fuera de Etapa 1 voluntario) | `MAPFRE_STOCK_CARTERA` | `STOCK_MAPFRE_*.xlsx` | `STOCK_MAPFRE` |

\*El docx numera “Anexo 4” tanto para **cancelaciones MAPFRE** como para **Deudores Banco Bolívar**; en operación real las cancelaciones usan la plantilla `Anulacion masiva_*` (columnas `FECHA PROYECTADA FIN DE VIGENCIA`, `FECHA DE ACTIVACIÓN`), no la estructura Bolívar.

**Controles alineados (Etapa 1):**

| Control | Doc | Semilla / processor |
|---|---|---|
| Edad Vida 18–75+364 | Anexo 1 · cabezote 108 | `age_min=18`, `age_max=75.997`, `age_max_days_before_birthday=1` |
| Edad AP 18–65+364 | Anexos 2–3 · cabezotes 114/110 | `age_max=65.997` |
| Primas Vida Plan1/2 | 8600 / 17100 | `mapfreVidaStockAllowedPremiums` + `mapfre_plan.go` |
| Primas AP Cáncer | 8500/8075, 13000/12350 | `mapfreCancerAllowedPremiums` |
| Primas AP Menores | 7800/7410, 10600/10070 | `mapfreAccMenAllowedPremiums` |
| Valor asegurado Vida col L | `VALORARECONOCER` | mapeo `insured_amount` ← `VALORARECONOCER` |
| Bolívar prima = deuda × % | Anexos 4–5 | reglas E.1–E.10 en processor |
| Bolívar prima 0 | “en revisión” en doc | API congela (`freeze_on_zero_premium`) |
| Cancelación MAPFRE ii | Fin proyectado y activación mismo día del mes | `mapfreCancelacionViolaciones` |
| Cancelación MAPFRE iii | Fin proyectado en mes OBS / nombre archivo | `mapfreCancelacionViolaciones` |
| Cancelación MAPFRE C.1.e | Crédito OP BT en stock MAPFRE; documento, póliza grupo y activación coherentes | `mapfreCancelacionViolacionesStock` + `ApplyMapfreStockCancellation` |

**Pendiente de implementación completa:** vías C.2–C.4 del diagrama (stock terminadas/siniestros), consolidación C.5, permanencia MAPFRE.

---

## 2. Convenciones de formato

| Concepto | Formato | Ejemplo |
|---|---|---|
| Fecha estándar | `YYYY-MM-DD` (ISO 8601) | `2026-06-04` |
| Fecha activación mensual / periodo | `MM/DD/YYYY` | `06/04/2026` |
| Fecha en mora | número de días (entero) | `45` |
| Edad | entero en años | `34` |
| Valor / prima | entero (sin separador de miles, sin decimales) | `6600` |
| Número de póliza | cadena alfanumérica | `"VID0012345"` |
| Plan | entero (`1` o `2`) | `1` |
| Producto | enum: `VOLUNTARIO`, `AP_MENORES`, `AP_CANCER` | `"AP_CANCER"` |

Toda fecha en el cuerpo JSON viaja en ISO 8601. Los formatos `MM/DD/YYYY` y "días" aplican a columnas internas derivadas (equivalentes a `TECHAACTMENSUAL/PERIODO` y `TECHAINMOROSO`); la API debe poder emitirlas o validarlas según el campo.

---

## 3. Esquema de datos — Ingresos

### 3.1 Objeto `RegistroIngreso`

| Campo | Tipo | Requerido | Descripción |
|---|---|---|---|
| `producto` | string (enum) | Sí | `VOLUNTARIO` \| `AP_MENORES` \| `AP_CANCER` |
| `hojaOrigen` | string | Sí | `VIDA VF` \| `ACCIDENTES M` \| `CANCER VF` |
| `documentoCliente` | string | Sí | Identificación del asegurado |
| `edad` | integer | Sí | Edad al ingreso, en años |
| `estadoPermanencia` | string | Sí | Estado según tabla del producto |
| `plan` | integer (enum) | Sí | `1` \| `2` |
| `valorPrima` | integer | Sí | Prima asociada al plan |
| `fechaVigencia` | date (ISO) | Sí | Inicio de vigencia |
| `fechaActivacion` | date (ISO) | Sí | Fecha de activación |
| `fechaFinAgenda` | date (ISO) | No | `PLANVENC` / fin de agenda |
| `numeroCredito` | string | Condicional | Requerido según regla de crédito |

### 3.2 Reglas de validación por producto

| Producto | Edad ingreso mín. | Edad ingreso máx. | Planes válidos | Valor prima esperado |
|---|---|---|---|---|
| **VOLUNTARIO** | 18 | 75 años + 364 días | 1, 2 | Plan1 = 6600 *(SDD borrador)* / **17100** Plan2 — en **semilla API** y archivos reales: **8600** / **17100** (`mapfre_inclusion_vida_voluntario` y `mapfre_stock_cartera`, mismo catálogo) |
| **AP_MENORES** | 18 | 65 años + 364 días | 1, 2 | Plan1 = 7800 *o* 7410 / Plan2 = 10600 *o* 10070 *(confirmar)* — semilla: `mapfre_inclusion_ap_menores` |
| **AP_CANCER** | 18 | 65 años + 364 días | 1–2 | Plan1 = 13000 *(SDD)* — semilla incluye **8500, 8075, 12000, 12350, 13000** según tarifario y muestras en `INCLUSION-CANCER-MAPFRE.xlsx` |

> "65 años + 364 días" significa: el asegurado no debe haber cumplido aún 66 años a la fecha de vigencia. Validación recomendada: `fechaVigencia < fechaNacimiento + (66 años)`.

### 3.3 Controles transversales (Ingresos)

| Control | Regla | Resultado si falla |
|---|---|---|
| Edad de ingreso | `edadMin <= edad <= edadMax` del producto | `REPORTAR_NOVEDAD` |
| Estado de permanencia | `estadoPermanencia` ∈ tabla del producto | `REPORTAR_NOVEDAD` |
| Valor de prima | `valorPrima == valorEsperado[producto][plan]` | `REPORTAR_NOVEDAD` |
| Fecha de vigencia | dentro del límite de activación del estado | `REPORTAR_NOVEDAD` |
| Fecha fin agenda | `fechaActivacion`/`PLANVENC` coincide con vigencia/mora/riesgo | `REPORTAR_NOVEDAD` |
| Control plazo | diferencia de fechas válida (formato d/m/a) | `REPORTAR_NOVEDAD` |
| Número de crédito | formato condicional según columna | `REPORTAR_NOVEDAD` |

---

## 4. Esquema de datos — Cancelaciones

### 4.1 Objeto `RegistroCancelacion`

| Campo | Tipo | Requerido | Descripción |
|---|---|---|---|
| `numeroPoliza` | string | Sí | Póliza actual |
| `numeroPolizaGrupoAnterior` | string | No | Póliza del grupo anterior (cruce) |
| `fechaActivacion` | date (ISO) | Sí | Fecha de activación insertada |
| `fechaProyectadaFinVigencia` | date (ISO) | Sí | Fin de vigencia proyectado |
| `estadoStock` | string (enum) | Sí | `ACTIVO` \| `CANCELADO` |
| `observacion` | string | No | Ej. `cancelar/MM/AAAA`, `Ex fallecido` |
| `avisoSiniestro` | string | No | Marca de siniestro |

### 4.2 Reglas de validación (Cancelaciones)

| Control | Regla | Resultado |
|---|---|---|
| Fecha fin de vigencia | proyectada al mes siguiente o mismo mes según regla | `REPORTAR_NOVEDAD` si fuera de rango |
| Cruce de pólizas | `numeroPoliza` / `numeroPolizaGrupoAnterior` existe en base de stock | marcar coincidencia |
| Marcado de cancelados | si está en archivo masivo → `estadoStock = CANCELADO` + observación `cancelar/MM/AAAA` | actualizar stock |
| Siniestros | si hay póliza cancelada por siniestro → escribir `avisoSiniestro` y marcar en stock | actualizar stock |
| Fallecimiento | observación `Ex fallecido` / `fallecido` | actualizar stock |
| Estructura final | el registro cumple el esquema de columnas definido | rechazar si falta columna |

---

## 5. Endpoints propuestos

| Método | Ruta | Descripción |
|---|---|---|
| `POST` | `/v1/ingresos/validar` | Valida un lote de registros de ingreso |
| `POST` | `/v1/cancelaciones/validar` | Valida un lote de cancelaciones contra stock |
| `GET` | `/v1/catalogos/primas` | Devuelve la tabla de primas vigentes por producto/plan |

---

## 6. Ejemplos de request / response

### 6.1 Ingreso válido

**Request** — `POST /v1/ingresos/validar`
```json
{
  "registros": [
    {
      "producto": "VOLUNTARIO",
      "hojaOrigen": "VIDA VF",
      "documentoCliente": "1023456789",
      "edad": 40,
      "estadoPermanencia": "ACTIVO",
      "plan": 1,
      "valorPrima": 6600,
      "fechaVigencia": "2026-06-01",
      "fechaActivacion": "2026-06-01",
      "numeroCredito": "CR-00045"
    }
  ]
}
```

**Response** — `200 OK`
```json
{
  "resumen": { "total": 1, "ok": 1, "conNovedad": 0 },
  "resultados": [
    {
      "documentoCliente": "1023456789",
      "estado": "OK",
      "novedades": []
    }
  ]
}
```

### 6.2 Ingreso con novedades (edad y prima fuera de regla)

**Request** — `POST /v1/ingresos/validar`
```json
{
  "registros": [
    {
      "producto": "AP_CANCER",
      "hojaOrigen": "CANCER VF",
      "documentoCliente": "1099887766",
      "edad": 67,
      "estadoPermanencia": "ACTIVO",
      "plan": 1,
      "valorPrima": 12000,
      "fechaVigencia": "2026-06-01",
      "fechaActivacion": "2026-06-01"
    }
  ]
}
```

**Response** — `200 OK`
```json
{
  "resumen": { "total": 1, "ok": 0, "conNovedad": 1 },
  "resultados": [
    {
      "documentoCliente": "1099887766",
      "estado": "REPORTAR_NOVEDAD",
      "novedades": [
        {
          "campo": "edad",
          "regla": "edad <= 65 años + 364 días",
          "valorRecibido": 67,
          "mensaje": "Edad de ingreso fuera del rango permitido para AP_CANCER"
        },
        {
          "campo": "valorPrima",
          "regla": "valorPrima == 13000",
          "valorRecibido": 12000,
          "mensaje": "Prima no coincide con el valor esperado del Plan 1"
        }
      ]
    }
  ]
}
```

### 6.3 Cancelación con marcado en stock

**Request** — `POST /v1/cancelaciones/validar`
```json
{
  "registros": [
    {
      "numeroPoliza": "VID0012345",
      "numeroPolizaGrupoAnterior": "VID0009999",
      "fechaActivacion": "2025-07-01",
      "fechaProyectadaFinVigencia": "2026-07-01",
      "estadoStock": "ACTIVO",
      "observacion": "cancelar/06/2026"
    }
  ]
}
```

**Response** — `200 OK`
```json
{
  "resumen": { "total": 1, "actualizados": 1, "novedades": 0 },
  "resultados": [
    {
      "numeroPoliza": "VID0012345",
      "estadoStock": "CANCELADO",
      "observacion": "cancelar/06/2026",
      "cruceStock": "ENCONTRADO",
      "novedades": []
    }
  ]
}
```

---

## 7. Catálogo de novedades

| Código | Descripción |
|---|---|
| `EDAD_FUERA_RANGO` | Edad de ingreso fuera de los límites del producto |
| `ESTADO_NO_VALIDO` | Estado de permanencia no existe en la tabla |
| `PRIMA_NO_COINCIDE` | Valor de prima distinto al esperado para producto/plan |
| `FECHA_VIGENCIA_INVALIDA` | Fecha de vigencia fuera del límite de activación |
| `FECHA_FIN_AGENDA_INVALIDA` | Fin de agenda no coincide con vigencia/mora/riesgo |
| `PLAZO_INVALIDO` | Diferencia de fechas fuera de regla |
| `CREDITO_FORMATO_INVALIDO` | Número de crédito con formato incorrecto |
| `POLIZA_NO_ENCONTRADA` | No hay cruce en base de stock |
| `ESTRUCTURA_INVALIDA` | Falta una columna requerida del esquema |

---

## 8. Pendientes / decisiones abiertas

1. **Primas AP Menores:** confirmar Plan1 (7800 / 7410) y Plan2 (10600 / 10070).
2. **Estado de permanencia:** definir la lista exacta de valores válidos por producto (la tabla del diagrama no es legible al 100%).
3. **Regla de número de crédito:** especificar la condición exacta que lo hace requerido y su formato.
4. **Edad de permanencia:** excluida temporalmente por decisión de revisión.