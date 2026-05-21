# Proceso: FTP, Prefijos y Validación

Este documento detalla la fase inicial y crítica del procesamiento de archivos.

## 1. Recuperación vía FTP
El sistema utiliza un cliente interno que se conecta periódicamente a servidores FTP/SFTP externos para buscar nuevos documentos de seguros.

---

## 2. Identificación por Prefijo
Para saber qué reglas aplicar, el sistema analiza el nombre del archivo buscando un **Prefijo Clave**:

| Prefijo | Producto Correspondiente | Anexo |
|---------|--------------------------|-------|
| `MAPFRE_VIDA_` | Voluntario Vida | Anexo 1 |
| `MAPFRE_CANCER_` | AP Cáncer | Anexo 2 |
| `MAPFRE_MENORES_` | AP Menores | Anexo 3 |
| `BOLIVAR_BANCO_` | Deudores Banco | Anexo 4 |
| `BOLIVAR_ESAL_` | Deudores ESAL | Anexo 5 |

---

## 3. Validación de Estructura (Síncrona)
**¡CRÍTICO!** Antes de cualquier inserción, el sistema valida que el archivo coincida exactamente con la parametrización guardada.

- **Verificación**: Número de columnas, nombres exactos de encabezados y orden.
- **Reporte de Error**: Si hay un cambio estructural, el sistema:
  1. Detiene el proceso inmediatamente.
  2. Registra el error en `PROCESSED_FILE`.
  3. Notifica al equipo operativo.
- **Éxito**: Si la estructura es correcta, se registra como **PENDIENTE** en la base de datos.

---

## 4. Procesamiento Asíncrono
Una vez aceptado el archivo, los registros se validan fila por fila de forma asíncrona:
1. Se ejecutan validaciones por columna (Edad, Plazo, Tasa).
2. Se guardan los resultados individualmente en la tabla `RECORD`.
3. Al finalizar, el estado del archivo cambia a `PROCESADO` o `ERROR` (si hubo fallas en registros).
