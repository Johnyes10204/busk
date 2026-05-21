# Firmas de API (Hub Técnico)

Esta sección documenta los endpoints REST disponibles para controlar y monitorear el procesamiento automatizado desde el FTP.

## 1. Control de Procesamiento

### `POST /api/v1/process/scan`
Dispara manualmente el escaneo del servidor FTP para buscar nuevos archivos.

**Descripción:** El sistema busca archivos que coincidan con los prefijos configurados (`MAPFRE_`, `BOLIVAR_`).

**Respuestas:**
- `200 OK`: Escaneo iniciado exitosamente.
- `503 Service Unavailable`: Servidor FTP no disponible.

---

### `GET /api/v1/files`
Lista todos los archivos detectados en el FTP con su estado actual.

**Filtros opcionales:** `status` (PENDIENTE, ERROR, PROCESADO, FROZEN), `productId`, `date`.

---

### `GET /api/v1/files/:id/diff`
Obtiene el reporte de diferencias (Inclusions, Cancellations y Novedades) una vez el proceso asíncrono termina.

**Esquema de Retorno:**
```json
{
  "filename": "MAPFRE_VIDA_202403.xlsx",
  "status": "PROCESSED",
  "inclusions": [ ... ],
  "cancellations": [ ... ],
  "novedades": [ ... ]
}
```

## 2. Gestión de Stock e Inventario

### `GET /api/v1/stock/:productId/policies`
Consulta el listado de pólizas activas en el stock para un producto específico.

**Parámetros de Consulta (Query):**
- `document_number`: Filtrar por número de documento (Cédula/NIT).
- `credit_id`: Filtrar por ID de crédito (Bolívar).
- `full_name`: Filtrar por nombre del asegurado (Búsqueda parcial).
- `status`: Filtrar por estado (`ACTIVE`, `FROZEN`, `CANCELLED`).
- `page` / `limit`: Control de paginación.

---

### `GET /api/v1/stock/:productId/policies/:id`
Obtiene el detalle completo de una póliza o crédito. El esquema es **unificado** para todos los productos, permitiendo que el cliente procese la información de manera genérica.

#### Esquema Unificado (Base):
| Nodo | Descripción |
|------|-------------|
| `customer_data` | Información del titular/asegurado/deudor. |
| `financial_data` | Valores monetarios, tasas y primas. |
| `reference_data` | Datos específicos del origen (Póliza Mapfre o Crédito Bolívar). |
| `validation_data` | Resultados de reglas y alertas. |

#### Ejemplo: Producto MAPFRE (Vida Voluntaria)
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "product_id": "MAPFRE_VIDA",
  "status": "ACTIVE",
  "customer_data": {
    "id_number": "12345678",
    "id_type": "CC",
    "full_name": "JUAN PEREZ",
    "gender": "M",
    "birth_date": "1985-05-15"
  },
  "financial_data": {
    "base_value": 0,
    "premium_value": 8600.00,
    "tax_value": 0,
    "total_value": 8600.00,
    "currency": "COP"
  },
  "reference_data": {
    "reference_number": "108-000456",
    "start_date": "2024-01-01",
    "end_date": "2024-12-31",
    "additional_system_info": {
      "branch": "BOGOTA",
      "plan": "PLAN A"
    }
  },
  "validation_data": {
    "is_valid": true,
    "alerts": [],
    "calculated_age": 38
  }
}
```

#### Ejemplo: Producto BOLIVAR (Deudores)
```json
{
  "id": "660e8400-e29b-41d4-a716-446655441111",
  "product_id": "BOLIVAR_BANCO",
  "status": "ACTIVE",
  "customer_data": {
    "id_number": "900123456",
    "id_type": "NIT",
    "full_name": "EMPRESA ABC SAS",
    "gender": "N/A",
    "birth_date": null
  },
  "financial_data": {
    "base_value": 25000000.00,
    "premium_value": 20825.00,
    "tax_value": 0,
    "total_value": 20825.00,
    "currency": "COP",
    "applied_rate": 0.000833
  },
  "reference_data": {
    "reference_number": "99887766",
    "start_date": "2023-11-20",
    "end_date": null,
    "additional_system_info": {
      "term_months": 60,
      "operation_type": "BT"
    }
  },
  "validation_data": {
    "is_valid": true,
    "alerts": ["MANUAL_REVIEW_REQUIRED"],
    "requires_manual_action": true
  }
}
```

---

### `POST /api/v1/stock/reload`
Reemplaza la base de stock actual con los datos consolidados del último archivo procesado exitosamente.

---

### `GET /api/v1/rules/:productId`
Consulta la parametrización de estructura y reglas aplicada para un tipo de archivo específico.

---

## 3. Manejo de Errores (RFC 7807)
Todos los errores devueltos por la API seguirán la especificación **Problem JSON (RFC 7807)** requerida por Zalando Guidelines.

```json
{
  "type": "https://docs.buskseguros.com/errors/resource-not-found",
  "title": "Stock Not Found",
  "status": 404,
  "detail": "No active policies found for product MAPFRE_VIDA_XYZ.",
  "instance": "/api/v1/stock/MAPFRE_VIDA_XYZ/policies"
}
```

---

## 4. Especificación OpenAPI 3.0 (Swagger)
A continuación, el contrato formal de integración para el consumo de políticas validadas y control de archivos.

```yaml
openapi: 3.0.3
info:
  title: Busk Seguros Docs API
  version: 1.0.0
  description: "API for accessing validated insurance policies and controlling FTP file ingestion."
servers:
  - url: https://api.buskseguros.com/v1
security:
  - bearerAuth: []
paths:
  /auth/login:
    post:
      summary: Authenticate user and get JWT
      operationId: loginUser
      security: [] # No auth required to login
      tags:
        - Authentication
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [email, password]
              properties:
                email:
                  type: string
                  format: email
                password:
                  type: string
                  format: password
      responses:
        '200':
          description: Successful authentication
          content:
            application/json:
              schema:
                type: object
                properties:
                  access_token:
                    type: string
                  expires_in:
                    type: integer
        '401':
          description: Invalid credentials
          content:
            application/problem+json:
              schema:
                $ref: '#/components/schemas/Problem'
  
  /process/scan:
    post:
      summary: Trigger manual FTP scan
      operationId: triggerScan
      tags:
        - Control
      responses:
        '200':
          description: Scan initiated successfully
        '503':
          description: FTP service unavailable
          content:
            application/problem+json:
              schema:
                $ref: '#/components/schemas/Problem'
  
  /files:
    get:
      summary: List all detected files
      operationId: listFiles
      tags:
        - Monitoring
      parameters:
        - in: query
          name: status
          schema:
            type: string
            enum: [PENDING, PROCESSING, PROCESSED, ERROR]
        - in: query
          name: product_id
          schema:
            type: string
      responses:
        '200':
          description: A list of file tracking records
          
  /files/{id}/diff:
    get:
      summary: Get processing diff for a specific file
      operationId: getFileDiff
      tags:
        - Monitoring
      parameters:
        - in: path
          name: id
          required: true
          schema:
            type: string
            format: uuid
      responses:
        '200':
          description: Diff report containing inclusions, cancellations, and novedades
          
  /stock/reload:
    post:
      summary: Reload persistent stock from the latest processed file
      operationId: reloadStock
      tags:
        - Stock Management
      responses:
        '200':
          description: Stock reloaded successfully
          
  /rules/{product_id}:
    get:
      summary: Get rule configuration for a product
      operationId: getProductRules
      tags:
        - Configuration
      parameters:
        - in: path
          name: product_id
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Rule configuration
          
  /stock/{product_id}/policies:
    get:
      summary: Retrieve active policies for a product
      operationId: getStockPolicies
      tags:
        - Stock
      parameters:
        - in: path
          name: product_id
          required: true
          schema:
            type: string
        - in: query
          name: document_number
          schema:
            type: string
        - in: query
          name: credit_id
          schema:
            type: string
        - in: query
          name: status
          schema:
            type: string
            enum: [ACTIVE, FROZEN, CANCELLED]
      responses:
        '200':
          description: A list of policies
          content:
            application/json:
              schema:
                type: object
                properties:
                  items:
                    type: array
                    items:
                      $ref: '#/components/schemas/Policy'
        '400':
          description: Bad Request
          content:
            application/problem+json:
              schema:
                $ref: '#/components/schemas/Problem'
        '404':
          description: Not Found
          content:
            application/problem+json:
              schema:
                $ref: '#/components/schemas/Problem'

components:
  schemas:
    Policy:
      type: object
      properties:
        id:
          type: string
          format: uuid
        product_id:
          type: string
        status:
          type: string
        customer_data:
          type: object
        financial_data:
          type: object
        reference_data:
          type: object
        validation_data:
          type: object
    Problem:
      type: object
      properties:
        type:
          type: string
          format: uri
        title:
          type: string
        status:
          type: integer
        detail:
          type: string
        instance:
          type: string
          format: uri
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT
      description: "Enter your Bearer token in the format **Bearer &lt;token&gt;**"
```
