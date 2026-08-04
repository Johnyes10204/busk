# Mapeos de Columnas — Busk Seguros

## Descripción General

Cada formato de producto define un mapeo entre:
- **Columnas XLSX** (nombres en el archivo entregado por cliente)
- **Campos canónicos** (representación interna en BD)

El mapeo se almacena en `product_formats.mappings_json` y se aplica durante el parseo del archivo.

---

## 1. Estructura del Mapeo JSON

```json
{
  "alias_mapping": {
    "DNI": "identification_number",
    "Nombre": "policyholder_name",
    "Fecha de Nacimiento": "birthdate",
    "Plan": "plan",
    "Prima Anual": "prime_annual",
    "Fecha Cobertura": "coverage_start_date",
    "Estado": "status"
  },
  "field_rules": {
    "identification_number": {
      "required": true,
      "type": "string",
      "trim": true,
      "uppercase": true
    },
    "policyholder_name": {
      "required": true,
      "type": "string",
      "trim": true,
      "normalize_spaces": true
    },
    "birthdate": {
      "required": true,
      "type": "date",
      "format": "DD/MM/YYYY"
    },
    "prime_annual": {
      "required": true,
      "type": "decimal",
      "precision": 2
    },
    ...
  }
}
```

---

## 2. Campos Canónicos Globales

Todos los productos comparten estos campos base:

| Campo Canónico | Tipo | Obligatorio | Descripción |
|---|---|---|---|
| `identification_number` | string | Sí | DNI/Cédula/RIF del titular |
| `policyholder_name` | string | Sí | Nombre del titular |
| `birthdate` | date (DD/MM/YYYY) | Sí | Fecha de nacimiento |
| `coverage_start_date` | date (DD/MM/YYYY) | Sí | Fecha de inicio de cobertura |
| `policy_status` | string | No | Estado de la póliza (ACTIVE, INACTIVE, etc.) |

---

## 3. MAPFRE — Vida Voluntario

**Identificación**: `5024424900103`, `VOLUNTARIO`

### 3.1 Campos Específicos

| Campo Canónico | Tipo | Obligatorio | Alias Comunes | Transformación |
|---|---|---|---|---|
| `plan` | string | Sí | Plan, Producto, Código Plan | UPPER(); validar contra catálogo |
| `prime_annual` | decimal | Sí | Prima Anual, Prima, Prima Annual | DECIMAL(10,2); validar con plan |
| `status` | string | No | Estado, Status | UPPER(); valores: ACTIVA, INACTIVA |

### 3.2 Mapeo de Ejemplo

```json
{
  "alias_mapping": {
    "DNI": "identification_number",
    "Nombre Completo": "policyholder_name",
    "Fecha Nacimiento": "birthdate",
    "Plan Vigente": "plan",
    "Prima Anual (Bs.)": "prime_annual",
    "Fecha Inicio Cobertura": "coverage_start_date",
    "Estatus": "status"
  },
  "field_rules": {
    "identification_number": {
      "required": true,
      "type": "string",
      "trim": true,
      "uppercase": true,
      "length": "6-15"
    },
    "policyholder_name": {
      "required": true,
      "type": "string",
      "trim": true,
      "length": "3-150"
    },
    "birthdate": {
      "required": true,
      "type": "date",
      "format": "DD/MM/YYYY",
      "range": ["1920-01-01", "TODAY"]
    },
    "plan": {
      "required": true,
      "type": "string",
      "trim": true,
      "uppercase": true,
      "validate_in_catalog": true
    },
    "prime_annual": {
      "required": true,
      "type": "decimal",
      "precision": 2,
      "min": 0,
      "validate_with_plan": true
    },
    "coverage_start_date": {
      "required": true,
      "type": "date",
      "format": "DD/MM/YYYY",
      "range": ["TODAY-2years", "TODAY+30days"]
    },
    "status": {
      "required": false,
      "type": "string",
      "uppercase": true,
      "allowed_values": ["ACTIVA", "INACTIVA"]
    }
  }
}
```

### 3.3 Ejemplo de Fila XLSX

| DNI | Nombre Completo | Fecha Nacimiento | Plan Vigente | Prima Anual (Bs.) | Fecha Inicio Cobertura | Estatus |
|---|---|---|---|---|---|---|
| 12345678 | JUAN PÉREZ | 15/03/1985 | V3-50K | 8500 | 01/06/2026 | ACTIVA |
| V-23456789 | María López García | 20/05/1990 | V4-100K | 12500 | 15/06/2026 | INACTIVA |

**Parseo**:
```
Fila 1:
  identification_number: "12345678"
  policyholder_name: "JUAN PÉREZ"
  birthdate: "15/03/1985"
  plan: "V3-50K"
  prime_annual: 8500.00
  coverage_start_date: "01/06/2026"
  status: "ACTIVA"

Fila 2:
  identification_number: "V-23456789" → normaliza a "V23456789"
  policyholder_name: "MARÍA LÓPEZ GARCÍA"
  birthdate: "20/05/1990"
  plan: "V4-100K"
  prime_annual: 12500.00
  coverage_start_date: "15/06/2026"
  status: "INACTIVA"
```

---

## 4. MAPFRE — AP Menores (Anexo 3)

**Identificación**: `5024524900101`, `ACC MEN`, `ACCIDENTE_MENORES`

### 4.1 Campos Específicos

| Campo Canónico | Tipo | Obligatorio | Alias Comunes | Transformación |
|---|---|---|---|---|
| `beneficiary_identification` | string | Sí | DNI Menor, Cédula Beneficiario | UPPER() |
| `beneficiary_name` | string | Sí | Nombre Menor, Beneficiario | Trim, normalize spaces |
| `beneficiary_birthdate` | date | Sí | Fecha Nac Menor | DD/MM/YYYY |
| `policyholder_identification` | string | Sí | DNI Padre/Madre, DNI Responsable | UPPER() |
| `policyholder_name` | string | Sí | Nombre Responsable | Trim |
| `policyholder_birthdate` | date | Sí | Fecha Nac Responsable | DD/MM/YYYY |
| `plan` | string | Sí | Plan, Código Plan | UPPER(); validar |
| `prime_annual` | decimal | Sí | Prima | DECIMAL(10,2) |

### 4.2 Mapeo de Ejemplo

```json
{
  "alias_mapping": {
    "DNI Menor": "beneficiary_identification",
    "Nombre Menor": "beneficiary_name",
    "Fecha Nac Menor": "beneficiary_birthdate",
    "DNI Responsable": "policyholder_identification",
    "Nombre Responsable": "policyholder_name",
    "Fecha Nac Responsable": "policyholder_birthdate",
    "Plan": "plan",
    "Prima Anual": "prime_annual",
    "Fecha Cobertura": "coverage_start_date"
  },
  "field_rules": {
    "beneficiary_identification": {
      "required": true,
      "type": "string",
      "trim": true,
      "uppercase": true,
      "length": "6-15"
    },
    "beneficiary_name": {
      "required": true,
      "type": "string",
      "trim": true,
      "length": "3-150"
    },
    "beneficiary_birthdate": {
      "required": true,
      "type": "date",
      "format": "DD/MM/YYYY",
      "range": ["TODAY-17years", "TODAY"],
      "age_range": [0, 17]
    },
    "policyholder_identification": {
      "required": true,
      "type": "string",
      "trim": true,
      "uppercase": true,
      "length": "6-15",
      "different_from": "beneficiary_identification"
    },
    "policyholder_name": {
      "required": true,
      "type": "string",
      "trim": true,
      "length": "3-150"
    },
    "policyholder_birthdate": {
      "required": true,
      "type": "date",
      "format": "DD/MM/YYYY",
      "range": ["1920-01-01", "TODAY"],
      "age_range": [18, 100]
    },
    "plan": {
      "required": true,
      "type": "string",
      "trim": true,
      "uppercase": true,
      "validate_in_catalog": true
    },
    "prime_annual": {
      "required": true,
      "type": "decimal",
      "precision": 2,
      "min": 0,
      "validate_with_plan": true
    },
    "coverage_start_date": {
      "required": true,
      "type": "date",
      "format": "DD/MM/YYYY"
    }
  }
}
```

### 4.3 Ejemplo de Fila XLSX

| DNI Menor | Nombre Menor | Fecha Nac Menor | DNI Responsable | Nombre Responsable | Fecha Nac Responsable | Plan | Prima Anual | Fecha Cobertura |
|---|---|---|---|---|---|---|---|---|
| V-12345678 | Carlos Rodríguez | 10/04/2015 | V-23456789 | María López | 15/03/1980 | AP50-MENOR | 5200 | 01/06/2026 |

---

## 5. MAPFRE — AP Cáncer (Anexo 2)

**Identificación**: `5024524900103`, `CANCER`

### 5.1 Campos Específicos

| Campo Canónico | Tipo | Obligatorio | Alias Comunes | Transformación |
|---|---|---|---|---|
| `plan` | string | Sí | Plan Cáncer | UPPER(); validar |
| `prime_annual` | decimal | Sí | Prima | DECIMAL(10,2) |
| `exclusion_cancer_date` | date | No | Fecha Exclusión, Excl. Cáncer | DD/MM/YYYY; si existe ≥ coverage_start |

### 5.2 Mapeo de Ejemplo

```json
{
  "alias_mapping": {
    "DNI": "identification_number",
    "Nombre": "policyholder_name",
    "Fecha Nacimiento": "birthdate",
    "Plan Cáncer": "plan",
    "Prima": "prime_annual",
    "Fecha Inicio": "coverage_start_date",
    "Fecha Exclusión Cáncer": "exclusion_cancer_date"
  },
  "field_rules": {
    "identification_number": {
      "required": true,
      "type": "string",
      "trim": true,
      "uppercase": true,
      "length": "6-15"
    },
    "policyholder_name": {
      "required": true,
      "type": "string",
      "trim": true,
      "length": "3-150"
    },
    "birthdate": {
      "required": true,
      "type": "date",
      "format": "DD/MM/YYYY",
      "age_range": [18, 75]
    },
    "plan": {
      "required": true,
      "type": "string",
      "trim": true,
      "uppercase": true,
      "validate_in_catalog": true
    },
    "prime_annual": {
      "required": true,
      "type": "decimal",
      "precision": 2,
      "validate_with_plan": true
    },
    "coverage_start_date": {
      "required": true,
      "type": "date",
      "format": "DD/MM/YYYY"
    },
    "exclusion_cancer_date": {
      "required": false,
      "type": "date",
      "format": "DD/MM/YYYY",
      "must_be_after": "coverage_start_date"
    }
  }
}
```

---

## 6. MAPFRE — Stock

**Identificación**: `STOCK` (contexto MAPFRE)

### 6.1 Campos Específicos

Idénticos a **Vida Voluntario**, pero sin `status` (todos ACTIVE por defecto).

### 6.2 Mapeo de Ejemplo

```json
{
  "alias_mapping": {
    "DNI": "identification_number",
    "Nombre": "policyholder_name",
    "F. Nac.": "birthdate",
    "Plan": "plan",
    "Prima": "prime_annual",
    "Inicio": "coverage_start_date"
  },
  "field_rules": {
    "identification_number": { "required": true, "type": "string", ... },
    "policyholder_name": { "required": true, "type": "string", ... },
    "birthdate": { "required": true, "type": "date", "format": "DD/MM/YYYY", ... },
    "plan": { "required": true, "type": "string", ... },
    "prime_annual": { "required": true, "type": "decimal", ... },
    "coverage_start_date": { "required": true, "type": "date", ... }
  }
}
```

---

## 7. MAPFRE — Anulación Masiva

**Identificación**: `ANULACION_MASIVA`, `CANCELACION`

### 7.1 Campos Específicos

| Campo Canónico | Tipo | Obligatorio | Alias Comunes | Transformación |
|---|---|---|---|---|
| `identification_number` | string | Sí | DNI, Cédula | UPPER() |
| `plan` | string | Sí | Plan, Código | UPPER() |
| `cancellation_date` | date | Sí | Fecha Anulación | DD/MM/YYYY; ≤ hoy |
| `cancellation_reason` | string | No | Motivo, Razón | Trim |

### 7.2 Mapeo de Ejemplo

```json
{
  "alias_mapping": {
    "DNI": "identification_number",
    "Plan": "plan",
    "Fecha Anulación": "cancellation_date",
    "Motivo": "cancellation_reason"
  },
  "field_rules": {
    "identification_number": {
      "required": true,
      "type": "string",
      "trim": true,
      "uppercase": true,
      "length": "6-15"
    },
    "plan": {
      "required": true,
      "type": "string",
      "trim": true,
      "uppercase": true,
      "validate_exists_in_stock": true
    },
    "cancellation_date": {
      "required": true,
      "type": "date",
      "format": "DD/MM/YYYY",
      "range": ["1920-01-01", "TODAY"]
    },
    "cancellation_reason": {
      "required": false,
      "type": "string",
      "trim": true,
      "max_length": 500
    }
  }
}
```

---

## 8. BOLÍVAR — Deudores Banco Micro

**Identificación**: `DEUDORES_BANCO`, `MICRO` (en nombre)

### 8.1 Campos Específicos

| Campo Canónico | Tipo | Obligatorio | Alias Comunes | Transformación |
|---|---|---|---|---|
| `outstanding_debt` | decimal | Sí | Deuda, Saldo Deudor | DECIMAL(10,2); ≥ 0 |
| `monthly_payment` | decimal | Sí | Cuota, Cuota Mensual | DECIMAL(10,2); > 0 |
| `debt_currency` | string | No | Moneda, Divisa | UPPER(); default: VEF |
| `status` | string | Sí | Estado, Estatus | UPPER(); ACTIVA/INACTIVA |

### 8.2 Mapeo de Ejemplo

```json
{
  "alias_mapping": {
    "DNI": "identification_number",
    "Nombre": "policyholder_name",
    "F. Nac.": "birthdate",
    "Deuda Actual": "outstanding_debt",
    "Cuota Mensual": "monthly_payment",
    "Moneda": "debt_currency",
    "Fecha Inicio": "coverage_start_date",
    "Estado": "status"
  },
  "field_rules": {
    "identification_number": {
      "required": true,
      "type": "string",
      "trim": true,
      "uppercase": true,
      "length": "6-15"
    },
    "policyholder_name": {
      "required": true,
      "type": "string",
      "trim": true,
      "length": "3-150"
    },
    "birthdate": {
      "required": true,
      "type": "date",
      "format": "DD/MM/YYYY",
      "age_range": [18, 75]
    },
    "outstanding_debt": {
      "required": true,
      "type": "decimal",
      "precision": 2,
      "min": 0
    },
    "monthly_payment": {
      "required": true,
      "type": "decimal",
      "precision": 2,
      "min": 0.01,
      "validate_ratio_with_debt": {
        "max_ratio": 120,
        "tag": "REVISAR PLAZO"
      }
    },
    "debt_currency": {
      "required": false,
      "type": "string",
      "uppercase": true,
      "allowed_values": ["VEF", "USD"],
      "default": "VEF"
    },
    "coverage_start_date": {
      "required": true,
      "type": "date",
      "format": "DD/MM/YYYY"
    },
    "status": {
      "required": true,
      "type": "string",
      "uppercase": true,
      "allowed_values": ["ACTIVA", "INACTIVA"]
    }
  }
}
```

### 8.3 Ejemplo de Fila XLSX

| DNI | Nombre | F. Nac. | Deuda Actual | Cuota Mensual | Moneda | Fecha Inicio | Estado |
|---|---|---|---|---|---|---|---|
| 12345678 | JUAN PÉREZ | 15/03/1985 | 100000 | 1050 | VEF | 01/06/2026 | ACTIVA |
| V-23456789 | María López | 20/05/1990 | 50000.50 | 500 | USD | 15/06/2026 | INACTIVA |

**Parseo**:
```
Fila 1:
  identification_number: "12345678"
  policyholder_name: "JUAN PÉREZ"
  birthdate: "15/03/1985"
  outstanding_debt: 100000.00
  monthly_payment: 1050.00
  debt_currency: "VEF"
  coverage_start_date: "01/06/2026"
  status: "ACTIVA"
  plazo_calculated: 95 meses (100000 / 1050 = 95.23 → floor = 95)

Fila 2:
  identification_number: "V23456789"
  policyholder_name: "MARÍA LÓPEZ"
  birthdate: "20/05/1990"
  outstanding_debt: 50000.50
  monthly_payment: 500.00
  debt_currency: "USD"
  coverage_start_date: "15/06/2026"
  status: "INACTIVA"
  plazo_calculated: 100 meses (50000.50 / 500 = 100.00 → floor = 100)
```

---

## 9. BOLÍVAR — Deudores Banco Pyme

**Identificación**: `DEUDORES_BANCO`, `PYME` (en nombre)

### 9.1 Campos Específicos

Idénticos a **Deudores Banco Micro**, con adición de:

| Campo Canónico | Tipo | Obligatorio | Alias Comunes | Transformación |
|---|---|---|---|---|
| `business_name` | string | Sí | Razón Social, Empresa | Trim |

### 9.2 Mapeo de Ejemplo

```json
{
  "alias_mapping": {
    "RIF": "identification_number",
    "Razón Social": "business_name",
    "Nombre Contacto": "policyholder_name",
    "F. Nac. Contacto": "birthdate",
    "Deuda": "outstanding_debt",
    "Cuota": "monthly_payment",
    "Moneda": "debt_currency",
    "Inicio": "coverage_start_date",
    "Status": "status"
  },
  "field_rules": {
    "identification_number": { "required": true, "type": "string", ... },
    "business_name": {
      "required": true,
      "type": "string",
      "trim": true,
      "length": "3-200"
    },
    "policyholder_name": { "required": true, "type": "string", ... },
    "birthdate": { "required": true, "type": "date", ... },
    "outstanding_debt": { "required": true, "type": "decimal", ... },
    "monthly_payment": { "required": true, "type": "decimal", ... },
    "debt_currency": { "required": false, "type": "string", ... },
    "coverage_start_date": { "required": true, "type": "date", ... },
    "status": { "required": true, "type": "string", ... }
  }
}
```

---

## 10. BOLÍVAR — Deudores ESAL Micro

**Identificación**: `DEUDORES_ESAL`, `MICRO`

### 10.1 Campos Específicos

Idénticos a **Deudores Banco Micro**.

---

## 11. BOLÍVAR — Deudores ESAL Pyme

**Identificación**: `DEUDORES_ESAL`, `PYME`

### 11.1 Campos Específicos

Idénticos a **Deudores Banco Pyme** (con `business_name` obligatorio).

---

## 12. BOLÍVAR — Stock

**Identificación**: `STOCK` (contexto BOLÍVAR)

### 12.1 Campos Específicos

Idénticos a **Deudores Banco Micro** (sin `business_name`, sin `debt_currency` opcional).

---

## 13. Reglas de Transformación Global

### 13.1 Trim y Normalizacion de Espacios

```go
// Para campos de texto
value = strings.TrimSpace(value)  // Elimina espacios al inicio/final
value = strings.Join(strings.Fields(value), " ")  // Normaliza espacios múltiples
```

### 13.2 Uppercase (Identificaciones, Planes, Status)

```go
// Para DNI, plan, moneda, status
value = strings.ToUpper(value)
```

### 13.3 Parsing de Números Decimales

```go
// Acepta:
// - "8500", "8500.00", "8.500,00" (locale español)
// Retorna: decimal(10,2) redondeado a 2 decimales
```

### 13.4 Parsing de Fechas

```go
// Formato esperado: DD/MM/YYYY
// Ejemplos: "15/03/1985", "01/06/2026"
// Validación: fecha válida en calendario (no 31/02)
// Rango: según producto
```

---

## 14. Ejemplo Completo: Archivo INCLUSION MAPFRE Vida

### 14.1 Archivo XLSX Entregado

```
┌─────────────┬──────────────────────┬──────────────────┬──────────┬────────────┬──────────────────┬─────────┐
│ DNI         │ Nombre Completo      │ Fecha Nacimiento │ Plan     │ Prima (Bs)│ Fecha Cobertura  │ Estatus │
├─────────────┼──────────────────────┼──────────────────┼──────────┼────────────┼──────────────────┼─────────┤
│ 12345678    │ JUAN  PÉREZ  GARCÍA  │ 15/03/1985       │ v3-50k   │ 8500       │ 01/06/2026       │ Activa  │
│ V-23456789  │ María López          │ 20/05/1990       │ V4-100K  │ 12500      │ 15/06/2026       │ INACTIVA│
│ 34567890 E  │ Carlos Rodríguez     │ 10/04/1950       │ V3-50K   │ 9000       │ 25/06/2026       │ ACTIVA  │
└─────────────┴──────────────────────┴──────────────────┴──────────┴────────────┴──────────────────┴─────────┘
```

### 14.2 Mapeo Aplicado

| Columna XLSX | Campo Canónico | Regla de Transformación |
|---|---|---|
| DNI | identification_number | TRIM, UPPER |
| Nombre Completo | policyholder_name | TRIM, normalize_spaces |
| Fecha Nacimiento | birthdate | Parse DD/MM/YYYY |
| Plan | plan | TRIM, UPPER, validate_catalog |
| Prima (Bs) | prime_annual | Parse decimal, validate_with_plan |
| Fecha Cobertura | coverage_start_date | Parse DD/MM/YYYY, validate_range |
| Estatus | status | TRIM, UPPER, allowed [ACTIVA, INACTIVA] |

### 14.3 Resultado Parseado

```json
[
  {
    "identification_number": "12345678",
    "policyholder_name": "JUAN PÉREZ GARCÍA",
    "birthdate": "15/03/1985",
    "plan": "V3-50K",
    "prime_annual": 8500.00,
    "coverage_start_date": "01/06/2026",
    "status": "ACTIVA"
  },
  {
    "identification_number": "V23456789",
    "policyholder_name": "MARÍA LÓPEZ",
    "birthdate": "20/05/1990",
    "plan": "V4-100K",
    "prime_annual": 12500.00,
    "coverage_start_date": "15/06/2026",
    "status": "INACTIVA"
  },
  {
    "identification_number": "34567890E",
    "policyholder_name": "CARLOS RODRÍGUEZ",
    "birthdate": "10/04/1950",
    "plan": "V3-50K",
    "prime_annual": 9000.00,
    "coverage_start_date": "25/06/2026",
    "status": "ACTIVA"
    // Issue: EDAD_FUERA_RANGO (76 años > 75)
  }
]
```

---

## 15. Alias Comunes por Idioma

### 15.1 Español (Principal)

| Campo Canónico | Alias Comunes |
|---|---|
| identification_number | DNI, Cédula, RIF, Cédula de Identidad, Número Identificación |
| policyholder_name | Nombre, Nombre Completo, Nombres y Apellidos, Asegurado |
| birthdate | Fecha Nacimiento, F. Nac., Fecha Nac., Nac. |
| plan | Plan, Producto, Código Plan, Plan Vigente |
| prime_annual | Prima, Prima Anual, Prima Anual (Bs.), Precio |
| coverage_start_date | Fecha Cobertura, Inicio Cobertura, Fecha Inicio, Vigencia Inicio |
| status | Estado, Estatus, Situación, Status Póliza |
| outstanding_debt | Deuda, Saldo Deudor, Deuda Actual, Saldo |
| monthly_payment | Cuota, Cuota Mensual, Pago Mensual, Cuota Fija |
| debt_currency | Moneda, Divisa, Tipo Moneda |
| business_name | Razón Social, Nombre Empresa, Empresa, RazonSocial |

### 15.2 English (Fallback)

| Campo Canónico | Alias Comunes |
|---|---|
| identification_number | ID, ID Number, DNI, Document ID |
| policyholder_name | Name, Full Name, Policyholder, Insured |
| birthdate | Date of Birth, Birth Date, DOB, Birthdate |
| plan | Plan, Product, Plan Code, Plan ID |
| prime_annual | Premium, Annual Premium, Price |
| coverage_start_date | Coverage Start, Start Date, Effective Date |
| status | Status, State, Situation |
| outstanding_debt | Debt, Outstanding Balance, Current Debt, Balance |
| monthly_payment | Payment, Monthly Payment, Monthly Fee, Installment |
| debt_currency | Currency, Coin |
| business_name | Business Name, Company Name, Company, Legal Name |

---

## 16. Algoritmo de Mapeo

```go
func mapRow(excelRow []interface{}, productFormat *ProductFormat) (*Policy, []Error) {
  mappings := parseJSON(productFormat.MappingsJSON)
  
  // Paso 1: Mapear headers a índices
  headerToIndex := buildHeaderIndex(excelRow, mappings.AliasMapping)
  
  // Paso 2: Extraer columnas
  policy := &Policy{}
  errors := []Error{}
  
  for canonicalField, aliasOptions := range mappings.AliasMapping {
    colIndex := headerToIndex[aliasOptions]
    value := excelRow[colIndex]
    
    // Paso 3: Aplicar transformaciones
    rules := mappings.FieldRules[canonicalField]
    transformedValue, err := applyTransformations(value, rules)
    
    if err != nil {
      errors.append(Error{Code: "TRANSFORM_FAILED", Field: canonicalField})
      continue
    }
    
    // Paso 4: Asignar a struct
    setField(policy, canonicalField, transformedValue)
  }
  
  return policy, errors
}
```

