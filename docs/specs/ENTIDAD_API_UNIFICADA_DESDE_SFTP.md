# Entidad API unificada (derivada de archivos reales SFTP)

**Objetivo:** proponer una entidad de API única para ingestión/validación de registros, usando los encabezados reales descargados en `tools/sftpconnect/downloads` y las reglas del diagrama.

**Archivos analizados (muestra real):**
- `INCLUSION-VIDA-MAPFRE.xlsx` (46 columnas)
- `INCLUSION-ACCIDEMENOR-MAPFRE.xlsx` (109 columnas)
- `INCLUSION-CANCER-MAPFRE.xlsx` (109 columnas)
- `STOCK-FEBRE-BOLIVAR.XLS` (21-22 columnas por hoja)

---

## 1) Campos comunes detectados

Estos aparecen en más de una aseguradora o son necesarios para reglas núcleo:

- Identificación persona: documento tipo/número, nombres/apellidos.
- Fechas clave: nacimiento, inicio vigencia, fin vigencia, adjudicación/vencimiento crédito.
- Prima y monetarios: prima mensual, deuda inicial, porcentaje/tasa.
- Identificación póliza/crédito: póliza grupo, código seguro, número de crédito / `OP BT`.
- Metadatos de operación: oficina, observaciones, producto/plan.

---

## 2) Mapeo de columnas origen -> campo canónico

## 2.1 Identidad

- `document_type`: `TIPOIDENTIFICACIONAFILIADO` | `TIPO DOC` | `TIPO DOCUMENTO`
- `document_number`: `IDENTIFICACIONAFILIADO` | `NUM DOCUM` | `COD CEDULA` | `IDENTIFICACION`
- `first_name`: `NOMBRES` | `NOMBRE AP`
- `last_name_1`: `APELLIDOPATERNO` | `APELLIDO1 AP` | `APELLIDO PATERNO`
- `last_name_2`: `APELLIDOMATERNO` | `APELLIDO2 AP` | `APELLIDO MATERNO`
- `gender`: `SEXO` | `GENERO`
- `birth_date`: `FECHANACIMIENTO` | `FECHA NAC` | `FECHA DE NACIMIENTO`

## 2.2 Póliza / crédito

- `policy_group_number`: `POLIZA GRUPO`
- `policy_code`: `CODIGO SEGURO`
- `plan_name`: `NOMBREPLAN` | `PLAN`
- `plan_code`: `COD PLAN` (cuando exista)
- `credit_number`: `NUMEROPRESTAMO` | `NUMERO DE CREDITO` | `OP BT`

## 2.3 Fechas de vigencia / crédito

- `activation_date`: `FECHAACTIVACION` | `FECHA INICIO DE VIGENCIA` (edad de ingreso al activar) | Bolívar: `FECHA ADJUDICACION`
- `coverage_start_date`: `FECHA INICIO DE VIGENCIA` (vigencias; si no hay `activation_date`, también sirve para edad)
- `coverage_end_date`: `FECHAFINVIGENCIADERIESGO REAL` | `FECHAFINVIGENCIADERIESGO REAL`
- `initial_term_months`: `PLAZO INICIAL`
- `loan_award_date`: `FECHA ADJUDICACION`
- `loan_due_date_current`: `FECHA VENCIMIENTO ACTUAL`

## 2.4 Valores

- `monthly_premium`: `PRIMA` | `PRIMAMENSUALPERIODO` | `PRIMA MENSUAL`
- `initial_debt_amount`: `DEUDA INICIAL`
- `rate_percent`: `%`
- `insured_amount`: `VALOR ASEGURADO`

## 2.5 Contacto / auxiliares

- `email`: `CORREO`
- `phone`: `CELULAR` | `TELEFONO`
- `address`: `DIRECCIÓN` | `DIRECCION`
- `postal_code`: `POSTAL`
- `office`: `OFICINA`
- `observations`: `OBSERVACIONES ...`

---

## 3) Reglas y campos usados por regla (para motor de validación)

## 3.1 MAPFRE

- `MAPFRE_AGE_ENTRY`
  - Campos: `birth_date`, `coverage_start_date` (o fecha de proceso), `product_code`
- `MAPFRE_AGE_STAY`
  - Campos: `birth_date`, `coverage_start_date`, `product_code`
- `MAPFRE_PLAN_ALLOWED`
  - Campos: `plan_code` o `monthly_premium`, `product_code`
- `MAPFRE_EFFECTIVE_MONTH`
  - Campos: `coverage_start_date`, `billing_month`
- `MAPFRE_END_DATE_CONSISTENCY`
  - Campos: `coverage_start_date`/`FEC_INACTIVACION`, `initial_term_months`, `coverage_end_date`
- `MAPFRE_CREDIT_FORMAT`
  - Campos: `credit_number`

## 3.2 BOLIVAR

- `BOLIVAR_RATE_PREMIUM_MATCH`
  - Campos: `initial_debt_amount`, `rate_percent`, `monthly_premium`
- `BOLIVAR_TERM_CONSISTENCY`
  - Campos: `loan_award_date`, `loan_due_date_current`, `monthly_premium` (según hoja), `calculated_term` (derivado)
- `BOLIVAR_AGE_RANGE`
  - Campos: `birth_date`, `loan_award_date` o fecha de proceso
- `BOLIVAR_DEBT_OVER_20M`
  - Campos: `initial_debt_amount`
- `BOLIVAR_CREDIT_UNIQUE`
  - Campos: `credit_number` (mapeado desde `OP BT`)
- `BOLIVAR_DUE_DATE_NOT_PAST`
  - Campos: `loan_due_date_current`, `billing_month`

---

## 4) Entidad API propuesta (payload canónico)

```json
{
  "source": {
    "insurer": "MAPFRE|BOLIVAR",
    "product_code": "MAPFRE_VIDA|MAPFRE_ACC_MEN|MAPFRE_CANCER|BOLIVAR_BANCO|BOLIVAR_ESAL",
    "file_name": "string",
    "sheet_name": "string",
    "row_number": 0
  },
  "person": {
    "document_type": "string",
    "document_number": "string",
    "first_name": "string",
    "last_name_1": "string",
    "last_name_2": "string",
    "gender": "M|F|N/A|string",
    "birth_date": "YYYY-MM-DD"
  },
  "policy": {
    "policy_group_number": "string",
    "policy_code": "string",
    "plan_name": "string",
    "plan_code": "string",
    "coverage_start_date": "YYYY-MM-DD",
    "coverage_end_date": "YYYY-MM-DD",
    "initial_term_months": 0,
    "monthly_premium": 0,
    "insured_amount": 0
  },
  "credit": {
    "credit_number": "string",
    "loan_award_date": "YYYY-MM-DD",
    "loan_due_date_current": "YYYY-MM-DD",
    "initial_debt_amount": 0,
    "rate_percent": 0
  },
  "contact": {
    "email": "string",
    "phone": "string",
    "address": "string",
    "postal_code": "string",
    "office": "string"
  },
  "ops": {
    "billing_month": "YYYY-MM",
    "observations": "string"
  },
  "raw": {
    "columns": {},
    "normalization_notes": []
  }
}
```

---

## 5) Recomendación de implementación API

- Endpoint de ingesta por lote: `POST /api/v1/files/ingest`
  - Normaliza columna origen -> campo canónico.
  - Guarda `raw.columns` para trazabilidad 100%.
- Endpoint de validación: `POST /api/v1/files/:id/validate`
  - Ejecuta reglas por `source.product_code`.
- Endpoint de diff/resultado: `GET /api/v1/files/:id/results`
  - Devuelve errores por fila + regla + campos implicados.

---

## 6) Hallazgos de calidad de datos (importantes)

- Hay encabezados repetidos en MAPFRE (`PLAN`, `TXT NOMBREB1`, `TXT APELLIDO1B1` y otros en algunas hojas).
- En Bolívar aparece duplicada `FECHA VENCIMIENTO ACTUAL`.
- Existen variantes ortográficas (`DIRECCIÓN` vs `DIRECCION`, `NUMEROPRESTAMO` vs `NUMERO DE CREDITO`).

Esto obliga a una capa de **normalización robusta** por alias antes de validar reglas.
