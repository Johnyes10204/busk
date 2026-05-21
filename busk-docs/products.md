# Catálogo de Productos y Estructuras (Anexos)

El sistema soporta 5 productos principales, cada uno basado en la estructura de los archivos anexos proporcionados.

## 1. MAPFRE - Vida Voluntaria (Anexo 1)
- **Prefijo Sugerido**: `MAPFRE_VIDA_`
- **Columnas Clave**:
  - `NUM DOCUM`: Llave primaria.
  - `FECHA NAC`: Para validación de edad (18-75 años).
  - `PRIMA MENSUAL`: Valores válidos ($8,600 o $17,100).
- **Características**: Archivo extenso (>100 columnas) con datos de beneficiarios y tomador.

## 2. MAPFRE - AP Cáncer (Anexo 2)
- **Prefijo Sugerido**: `MAPFRE_CANCER_`
- **Columnas Clave**:
  - `NUM DOCUM`: Llave primaria.
  - `FECHA NAC`: Validación de edad (18-70 años).
  - `VALOR PRIMA`: $8,500 o $13,000.

## 3. MAPFRE - AP Menores (Anexo 3)
- **Prefijo Sugerido**: `MAPFRE_MENORES_`
- **Columnas Clave**:
  - `NUM DOCUM`: Llave primaria.
  - `PRIMA`: $7,800, $7,410, $10,600 o $10,070.

## 4. BOLIVAR - Deudores Banco (Anexo 4)
- **Prefijo Sugerido**: `BOLIVAR_BANCO_`
- **Columnas Clave**:
  - `NUMERO DE CREDITO`: Llave primaria.
  - `DEUDA INICIAL`: Base para cálculo de prima.
  - `PRIMA MENSUAL`: Validada mediante `DEUDA * TASA`.
  - `PLAZO`: Validado contra fechas de adjudicación y vencimiento.

## 5. BOLIVAR - Deudores ESAL (Anexo 5)
- **Prefijo Sugerido**: `BOLIVAR_ESAL_`
- **Columnas Clave**:
  - Similar a Deudores Banco pero con tasa y reglas específicas para Entidades Sin Ánimo de Lucro (ESAL).
  - Validación de créditos > $20M (Requiere revisión manual).
