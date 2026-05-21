# Busk Seguros API - Especificacion Funcional

Documento funcional resumido para negocio, operaciones y tecnologia.

## 1. Que hace la API

- Recibe y procesa archivos de polizas/creditos desde SFTP/FTP.
- Valida encabezados y reglas por producto.
- Persiste resultados y expone stock por API REST.
- Mantiene trazabilidad de archivos y registros.

## 2. Flujo funcional

1. Escaneo de archivos remotos.
2. Identificacion de producto.
3. Validacion estructural de columnas.
4. Validacion de reglas por fila.
5. Reconciliacion y persistencia de stock.
6. Estado final del lote: PROCESSED o ERROR.

## 3. Endpoints principales

- `POST /api/v1/auth/login`: obtiene JWT.
- `POST /api/v1/process/scan`: ejecuta escaneo y procesamiento.
- `GET /api/v1/files`: consulta lotes y resumen SFTP.
- `GET /api/v1/files/:id/diff`: resultado por archivo.
- `GET /api/v1/stock/:productId/policies`: stock paginado por producto.
- `GET /api/v1/stock/:productId/policies/:id`: detalle de poliza.
- `GET /api/v1/rules/:productId`: reglas configuradas.
- `POST /api/v1/rules/:productId`: crear regla.
- `GET /api/v1/products`: listar productos.
- `POST /api/v1/products`: crear/actualizar producto.

## 4. Seguridad

- JWT Bearer para rutas protegidas.
- Errores en formato RFC7807.

## 5. En tiempo de ejecucion

Esta especificacion tambien queda publicada por el API en:

- `GET /api/v1/specs` (JSON)
- `GET /api/v1/specs/markdown` (Markdown)
- `GET /api/v1/specs/web` (vista web amigable)
- `GET /api/v1/openapi.json` (OpenAPI 3.0)
- `GET /api/v1/swagger` (Swagger UI ejecutable)
