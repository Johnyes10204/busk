# Busk Seguros - Documentacion

Este sitio concentra runbooks y specs tecnicos del flujo de procesamiento.

## Accesos rapidos

- [Runbook API Paso a Paso](specs/RUNBOOK_API_PASO_A_PASO.md)
- [API Funcional](specs/API_FUNCIONAL.md)
- [Entidad API Unificada](specs/ENTIDAD_API_UNIFICADA_DESDE_SFTP.md)
- [Flujo Recepcion y Validaciones](specs/FLUJO_RECEPCION_VALIDACIONES.md)

## Estructura

- `specs/`: especificaciones funcionales y tecnicas.
- `analysis/`: documentos de analisis y diseno tecnico.
- `diagrams/`: diagramas Mermaid (arquitectura, dominio y datos).
- `inputs/`: anexos fuente PDF/XLSX de referencia.

## Nota operativa

La coleccion Postman completa esta en:

- `docs/postman/Busk-Seguros-API.postman_collection.json`

Para iniciar API + sitio Docsify + Frontend Admin en un solo comando:

- `bash tools/dev/start-api-with-docs.sh`

Esto levanta:

- API en `http://localhost:8080`
- Documentacion en `http://localhost:3000`
- Frontend Admin en `http://localhost:5173`

Variables opcionales:

- `API_PORT` para declarar el puerto esperado de la API (default `8080`, usado para liberar puerto)
- `DOCS_PORT` para cambiar puerto de docs
- `FRONT_PORT` para cambiar puerto del frontend
- `START_FRONTEND=0` para no levantar el frontend
- `FREE_PORTS=0` para desactivar liberación automática de puertos antes de arrancar
