# Frontend Admin - Busk Seguros API

Este frontend permite administrar el flujo operativo del API de Busk Seguros desde una interfaz web.

## Qué incluye

- Salud del API (`GET /api/v1/health`)
- Seed inicial (`POST /api/v1/bootstrap/sample-products`)
- Gestión básica de productos y primas permitidas
- Scan de SFTP y monitoreo de progreso
- Consulta de archivos y resumen por `file_id`
- Búsqueda de pólizas

## Paso a paso

1. Levanta el API de Go en `http://localhost:8080`.
2. En otra terminal, entra al frontend:
   - `cd frontend-admin`
   - `npm install`
   - `npm run dev`
3. Abre la URL que muestra Vite (normalmente `http://localhost:5173`).
4. Ejecuta el flujo en pantalla:
   - **Paso 1-2:** salud + seed
   - **Paso 3-5:** productos + primas
   - **Paso 6-7:** scan + progreso + archivos + summary/descarga
   - **Paso 8:** búsqueda de pólizas

## Configuración de API

- Por defecto el frontend usa `/api/v1`.
- El `vite.config.ts` trae proxy para redirigir `/api` a `http://localhost:8080`.
- Si cambias host/puerto de API, ajusta la URL base en la UI o en el proxy.
