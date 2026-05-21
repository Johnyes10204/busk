# Proceso según diagrama PDF (solo fuente oficial)

**Fuente única:** `docs/inputs/pdf/Diagrama proceso actual Busk aj.pdf` (pág. 1 MAPFRE, pág. 2 BOLÍVAR).

Transcripción revisada contra **render del PDF** (imagen de alta resolución). Los **códigos numéricos** de planes deben **contrastarse siempre** con el original impreso: en diagramas densos pueden confundirse dígitos (p. ej. 15003 vs 15503).

---

## Cobertura frente al diagrama

| Bloque en el PDF | ¿Reflejado en este MD? |
|------------------|-------------------------|
| MAPFRE — cabecera (Inicio, VPN, SFTP, descarga Busk, script, `CONTROLES_MES_AÑO`) | Sí, §A |
| MAPFRE — controles **ingresos** (tres hojas: VIDA VP, ACCIDENTES M, CANCER VP) | Sí, §B (edades, planes, fechas comunes) |
| MAPFRE — notas laterales (nombres archivo, fórmulas Excel, filas que no cumplen) | **Parcial:** el PDF lleva notas **numeradas** (aprox. 1–13) con texto y fórmulas; aquí **no** se transcribe cada nota literal (habría que copiarlas desde el PDF). |
| MAPFRE — controles **cancelaciones** (cuatro caminos + cierre) | Sí, §C (antes estaba simplificado; ahora alineado a los **cuatro** caminos del diagrama) |
| MAPFRE — inclusión final bases stock y columnas de salida | Sí, §C.11–C.12 |
| BOLÍVAR — adquisición, nota 1 archivos | Sí, §D |
| BOLÍVAR — reglas centrales Banco / ESAL | Sí, §E |
| BOLÍVAR — cierre SFTP + correo | Sí, §F |

---

## MAPFRE — Página 1

### A. Recepción y preparación (fila superior del diagrama)

| Paso | Descripción en el diagrama |
|------|----------------------------|
| A.0 | **Inicio** → VPN del cliente → carpeta **SFTP**. |
| A.1 | Descargar archivos de bases a carpeta local **Busk**. |
| A.2 | Abrir bases de **inclusión** y el **archivo Script**; trabajar con **CONTROLES_MES_AÑO**. |
| A.3 | Copiar datos a la **hoja correspondiente** del script y **ejecutar** el script. |
| A.4 | **Guardar** como **`CONTROLES_MES_AÑO`**. |

Tras esto el flujo abre **`CONTROLES_MES_AÑO`** y entra en **APLICACIÓN DE CONTROLES INGRESOS**.

---

### B. Aplicación de controles de **ingresos**

Tres ramas en paralelo (hojas **AS/VIDA VP**, **AS/HOGAR ACCIDENTES M**, **AS/CANCER VP**).

#### B.1 Edades de **ingreso**

La edad se valida con **`FECHA DE NACIMIENTO`** y la **fecha de activación** (en API: `activation_date` o, si no existe la columna, `coverage_start_date` / `FECHA INICIO DE VIGENCIA`; en stock MAPFRE puede ser `FECHAACTIVACION`). Se calculan los **años cumplidos en el día de activación** y se comparan con `age_min` / `age_max` del producto. Tope de ingreso (ej. 75,997 → **75 años 364 días**): la activación puede ser **hasta el día anterior** al cumpleaños 76 (`age_max_days_before_birthday` = 1). La tolerancia de **2 días** (`mapfre_date_tolerance_days`) aplica solo a **vigencias/plazo**, no a la edad.

| Rama | Mínimo | Máximo | Si no cumple |
|------|--------|--------|--------------|
| VIDA VP | 18 años | 75 años 364 días | **Reportar novedad** |
| ACCIDENTES M (AP Monorisc) | 18 años | 65 años 364 días | Reportar novedad |
| CANCER VP | 18 años | **65** años 364 días | Reportar novedad |

#### B.2 Edades de **permanencia**

| Rama | Regla | Si no cumple |
|------|--------|--------------|
| VIDA VP | Según **tabla** del diagrama | Reportar novedad |
| ACCIDENTES M | 18 años a 70 años 364 días | Reportar novedad |
| CANCER VP | 18 años a 70 años 364 días | Reportar novedad |

#### B.3 **Planes** (códigos; validar en el PDF los dígitos exactos)

| Rama | Plan 1 | Plan 2 | Si no cumple |
|------|--------|--------|--------------|
| VIDA VP | **15503** (verificar en PDF) | **17101** | Otro valor → reportar novedad |
| ACCIDENTES M | **7800** y **11410** | **11800** y **13379** | Otro valor → reportar novedad |
| CANCER VP | **4503** | **12001** | Otro valor → reportar novedad |

**Implementación API (`validarPlanMapfre`):** el control de plan usa **`plan_name`** (Plan 1 / Plan 2), no `plan_code`. La prima del archivo debe coincidir con el tarifario como **prima mensual** o como **prima total ÷ `PLAZO INICIAL` (meses)** cuando el plazo es &gt; 0 (misma tolerancia de centavos). Si cuadra, plan y prima se consideran válidos para la carga.

#### B.4 Validaciones **comunes** posteriores a planes (diagrama)

| Tema | Qué indica el diagrama |
|------|-------------------------|
| **Fecha de vigencia** | Debe alinearse con el **mes que se está facturando**. |
| **Fecha / fin de vigencia (cálculo)** | **`FEC_INACTIVACION` + `PLAZO INICIAL`** debe dar **`FECHAFINVIGENCIADEREGISTRO REAL`** (coherencia de plazo según rotulación del diagrama). |
| **Corto plazo** | Nodo **CORTO PLAZO** en el flujo (revisión según rama del diagrama). |
| **Número de crédito** | Validación de que **no lleva decimales** y cumple **formatos Excel** indicados. |
| **Notas 1–9** (margen) | Convenciones de **nombre de archivo** (ej. `Poliza_…_CANCER…`), fórmulas sobre campos tipo **`PRIMAINICIALRESERV`** / **`PRIMA`**, e indicaciones para **marcar filas** que no cumplen criterios. |

---

### C. Aplicación de controles de **cancelaciones**

El diagrama no es un solo bloque lineal: describe **cuatro vías** y luego consolidación y salida.

#### C.1 Vía **anulación masiva**

| Paso | Descripción en el diagrama |
|------|----------------------------|
| C.1.a | Abrir archivo **anulación-masiva**. |
| C.1.b | **Insertar** fechas de activación. |
| C.1.c | Validar que la **fecha fin proyectada** corresponda al **mes de facturación**. |
| C.1.d | Cálculos con **fórmulas** en columnas nuevas y comprobación de **diferencias en sumas**. |
| C.1.e | Cruzar **número de póliza** con archivo **BANCO SANTAL** (número de crédito) y validar **`NUMER POLIZA GRUPO ANTERIOR`**. |

#### C.2 Vía **bases de stock**

| Paso | Descripción en el diagrama |
|------|----------------------------|
| C.2.a | Abrir **base de stock**. |
| C.2.b | Filtrar por vigencia del **mes en curso**. |
| C.2.c | Si **`prima a` ≥ 0**: escribir en observaciones texto del tipo **«Termina en el mes que se está trabajando»**. |

#### C.3 Vía **terminadas**

| Paso | Descripción en el diagrama |
|------|----------------------------|
| C.3.a | Abrir hoja **Reportes**, filtrar vigencia del mes en curso. |
| C.3.b | Copiar registros a hoja **Terminadas**. |
| C.3.c | **Eliminar** esos registros de la base de stock principal. |

#### C.4 Vía **siniestros**

| Paso | Descripción en el diagrama |
|------|----------------------------|
| C.4.a | Abrir archivo de **siniestros**. |
| C.4.b | Buscar pólizas identificadas en la **base de stock**. |
| C.4.c | En observaciones: **«AVISO DE SINIESTRO…»**, **«Área de Siniestro»**; poner **`prima a` = 0**. |

#### C.5 Consolidación, marcas y envío (cierre MAPFRE en el diagrama)

| Paso | Descripción en el diagrama |
|------|----------------------------|
| C.5.a | Incorporar **nuevas inclusiones** reportadas en bases de stock **108, 110 y 115** (códigos que figuran en el diagrama; **verificar** en PDF por si el dibujo usa otras variantes). |
| C.5.b | Actualizar columna de observación con texto tipo **«emitida [MES]»**. |
| C.5.c | **Guardar** archivo de stock validado y **subir** a **SFTP**. |
| C.5.d | El diagrama incluye además **ingeniería de datos**, **cargue a Galileo** y **correo** al cliente con resúmenes (mismo cierre operativo que la versión anterior del diagrama). |

#### C.6 Tabla de **columnas** requeridas (bloque final del diagrama MAPFRE)

Columnas mencionadas en la tabla de estructura de salida: **`POLIZA`**, **`NUMER_POLIZA_GRUPO`**, **`ID_CLIENTE`**, **`FECHA_OPERACION_MANTTO`**, **`TIPO_TRANSACCION`**, **`VAL_PRIMA`**, **`IDENTIFICADOR_ANULACION`**.

---

## BOLÍVAR — Página 2

Título del proceso en el diagrama: **APLICACION DE CONTROLES CANCELACION** (sic en el original).

### D. Adquisición de archivos

| Paso | Descripción |
|------|-------------|
| D.1 | **Inicio** → VPN cliente → carpeta **SFTP del banco**. |
| D.2 | Descargar bases a carpeta local **Busk** → **Fin** de esta fase. |

**Nota (1)** del PDF (ejemplos de nombres citados en el diagrama):

- `Poliza_102_103_105_109_135842_31122023_micro_ESAL_MES.xlsx`
- `Poliza_108_114_122_202478_31122023_Pyme_ESAL_MES.xlsx`
- `1000004553301_MICRO_BANCO_MES_V1.xlsx`
- `1000004553301_Pyme_BANCO_MES_V1.xlsx`

---

### E. Controles (dos estructuras en paralelo)

1. **Estructura `Deudores_Banco_Bolivar_Inclusiones`**
2. **Estructura `Deudores_ESAL_Bolivar_Inclusiones`**

Ambas comparten la misma lógica en el diagrama.

| ID | Regla | Detalle según el diagrama |
|----|--------|---------------------------|
| E.1 | Prima desde deuda | **`DEUDA INICIAL` × %** = **`PRIMA MENSUAL`**. Código: `bolivar_rules.go` (`bolivarPrimaEsperada`). |
| E.2 | Plazo calculado | Plazo en meses desde fechas de adjudicación y vencimiento (`bolivarPlazoCalculadoMeses`). |
| E.3 | Plazo vs prima | Columna del diagrama; **no implementada** (el PDF no define la fórmula de comparación). |
| E.4 | ¿Hay diferencia? | Si **sí** en E.1 o E.5 → la **observación** no vacía justifica; si está vacía → incidencia. |
| E.5 | Plazo en días (**Nota 2**) | Días entre vencimiento y fin esperado según plazo calculado; **0** = correcto; si no, rama E.4. Tolerancia opcional vía `bolivar_plazo_dias_tolerance` (0 = estricto PDF). |
| E.6 | Edad | Desglosar **`FECHA DE NACIMIENTO`** en años, meses y días. |
| E.7 | Rango de edad | Entre **18 años** y **74 años 364 días** (equivalente a menor de 75 años según el texto del diagrama). |
| E.8 | ¿Cumple edad? | Si **no** y **deuda > 20M** → **incidencia de edad** (bloquea carga). Si **no** y deuda **≤ 20M** → **no** se reporta edad fuera de rango (la fila continúa). No hay incidencia aparte solo por monto. **Nota 3**: excepciones formales de edad de ingreso. |
| E.9 | `OP BT` | Formato condicional para **duplicados**; debe ser **ID único**. |
| E.10 | `FECHA VENCIMIENTO ACTUAL` | Fechas en **mes/día/año** (MDY); ambigüedad se resuelve en silencio (sin nota en informe). Solo se reporta si la fecha es inválida o dispara regla (vencimiento &lt; mes facturación + prima &gt; 0 → revisar prima; prima = 0 → cancelación sin nota de fecha). |

---

### F. Cierre BOLÍVAR

| Paso | Descripción |
|------|-------------|
| F.1 | Ingresar carpeta **SFTP Bolívar**. |
| F.2 | Guardar bases **con controles** aplicados. |
| F.3 | Correo al cliente con **novedades** encontradas e **inventario de mora**. |
| F.4 | **Fin**. |

---

## Lo que este MD **no** sustituye

1. **Texto literal** de cada **círculo amarillo** (notas 1–13 MAPFRE, notas 2–3 BOLÍVAR): hay que abrir el PDF y copiar fórmulas y matices palabra por palabra si van a implementarse.
2. **Verificación óptica** de todos los **códigos numéricos** de planes (transcripción automática puede equivocarse en un dígito).
3. **Orden exacto** de cada flecha entre sub-nodos dentro de una misma caja: aquí se resume la **intención** del diagrama, no un grafo nodo a nodo exportable.

Con lo anterior, el documento **sí** intenta cubrir **todos los bloques** visibles en las dos páginas; la **fidelidad al 100 %** frente al PDF requiere una pasada humana con el archivo original abierto al lado.
