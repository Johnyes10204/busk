# Lógica de Negocio y Flujo de Datos

## 1. Origen y Identificación
El sistema no solo monitorea carpetas locales, sino que se conecta a un **servidor FTP** para descargar archivos.

### Identificación por Prefijo:
Cada archivo es identificado mediante un **Prefijo o Nombre Clave**. Esto permite al sistema saber instantáneamente qué parametrización de producto aplicar.
Ejemplo: `MAPFRE_VIDA_202403.xlsx` -> Producto: MAPFRE Voluntario Vida.

---

## 2. Validación de Estructura (Síncrona)
Antes de procesar cualquier registro, el sistema verifica que la **estructura del archivo** sea idéntica a la parametrizada (nombres de columnas, orden y tipos).

- **Si cambia la estructura**: Se detiene el proceso y se reporta el error de inmediato sin cargar datos.
- **Si la estructura es válida**: El archivo se registra en la base de datos con estado **"PENDIENTE"** para su procesamiento posterior.

---

## 3. Procesamiento Asíncrono de Registros
Una vez que el archivo es aceptado estructuralmente, un proceso asíncrono recorre cada registro para ejecutar las validaciones de columna (Edad, Plan, Tasa, etc.).

### Flujo de Registro:
1. **Validación de Columna**: Se ejecutan las reglas individuales.
2. **Persistencia**: Se guarda el resultado de cada registro individualmente.
3. **Cruce de Stock**: Se determinan Inclusiones, Cancelaciones y Novedades.

### Reglas Específicas del Diagrama Base (PDF Original):
El Motor Asíncrono debe programar e instanciar obligatoriamente las siguientes reglas funcionales extraídas del modelo de flujo:

**A. Reglas Comunes y de Vida (Mapfre)**
- **Regla de Edad**: Se valida contra el parámetro del producto (ej. *18 a 75 años* para Vida Voluntario, *18 a 70 años* para AP Menores/Cáncer). Un deudor que incumpla se marca con Novedad/Error.
- **Regla de Plan (Tarifario)**: La `PRIMA` (ej. $8,600 o $17,100) debe estar en el diccionario estricto del producto.
- **Control de Siniestros**: Cruce de la Cédula (`NUM DOCUM`) contra la tabla o servicio externo de fallecimientos reportados. Genera marca de alerta inmediata.

**B. Reglas de Crédito (Bolívar)**
- **Control de Tasa**: `DEUDA INICIAL` × `% Tasa` debe ser exactamente igual a la `PRIMA MENSUAL` reportada.
- **Montos Superiores a 20 Millones**: Si `DEUDA INICIAL` > `$20,000,000`, el sistema aprueba la estructura pero levanta bandera `MANUAL_REVIEW_REQUIRED` (No se emite automáticamente).
- **Control de Duplicidad**: Búsqueda en Stock Histórico e Interno para asegurar que la Operación (`OP BT`) no se ha cruzado doblemente en el mismo mes.

---

## 2. Prioridades del Motor de Reglas

| Prioridad | Nivel | Acción ante Falla |
|----------|-------|-------------------|
| **1** | Estructura | Rechazar archivo completo |
| **2** | Datos Críticos | Marcar registro como INVÁLIDO, rechazar archivo |
| **3** | Política de Negocio | Marcar como Novedad, requiere revisión manual |
| **4** | Metadatos | Advertencia de baja prioridad |

---

## 4. Reconciliación y Gestión de Vigencia
Para determinar qué pólizas siguen vigentes, el sistema realiza una **Reconciliación de Stock** al finalizar el procesamiento asíncrono.

### Lógica de Desactivación:
1. **Identificar Vigencia**: Se seleccionan todas las pólizas en la tabla `POLICY` que pertenecen al `product_id` del archivo actual.
2. **Cruce (Diff)**: El sistema busca cuáles de esas pólizas **NO** están presentes en el nuevo archivo cargado.
3. **Desactivación Automática**: Aquellas pólizas ausentes en el nuevo archivo se marcan automáticamente como `CANCELLED` o `INACTIVE`.
4. **Actualización**: Las pólizas que sí están presentes se actualizan con la nueva información del archivo (Novedades).

---

## 5. Política de Congelamiento
Una regla especial aplicada durante la validación asíncrona es el **Congelamiento de Póliza**.

- **Condición**: Si el valor de la prima (`premium_value` / `prima`) llega en **0** en el archivo para ciertos productos.
- **Acción**: La póliza NO se cancela ni se marca como error. En su lugar, el sistema la marca con el estado **`FROZEN` (Congelada)**.
- **Efecto**: La póliza sigue marcada como "Activa" bajo esta política especial, permitiendo que la cobertura se mantenga o se suspenda según la lógica interna, pero sin perder la persistencia en el stock.
