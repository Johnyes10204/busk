# Arquitectura de Infraestructura

Representación visual de los componentes del sistema y el ciclo de vida del procesamiento de documentos.

---

## 1. Modelos Visuales de Referencia
Para comprender rápidamente la magnitud y funcionamiento del sistema, revisa los modelos conceptuales aprobados:

### Modelo de Entidades (ERD)
Para claridad conceptual, a continuación las entidades core y sus relaciones principales en la base de datos:

![ERD Diagram](assets/diagram_0.png)

```mermaid
erDiagram
    FILE_TRACKING ||--o{ RAW_RECORD : contains
    RAW_RECORD ||--o{ POLICY : creates_or_updates
    PRODUCT ||--o{ FILE_TRACKING : defines
    PRODUCT ||--o{ RULE : has
    PRODUCT ||--o{ POLICY : groups
    
    FILE_TRACKING {
        uuid id PK
        uuid product_id FK
        string original_filename
        string status "PENDING, PROCESSING, PROCESSED, ERROR"
        jsonb validation_summary
        timestamp received_at
    }
    
    POLICY {
        uuid id PK
        uuid product_id FK
        string document_number UK
        string credit_id
        string status "ACTIVE, CANCELLED, FROZEN"
        numeric premium_value
        jsonb validation_data
    }
    
    RAW_RECORD {
        uuid id PK
        uuid file_id FK
        boolean is_valid
        jsonb raw_data
        jsonb execution_log
    }
```

### Ciclo de Vida del Archivo (File Tracking)
Controla el estado operativo del procesamiento en bloque de un Anexo FTP.

![State Diagram File](assets/diagram_1.png)

```mermaid
stateDiagram-v2
    [*] --> PENDING: Archivo Detectado en FTP
    PENDING --> PROCESSING: Pasa Validación Estructural
    PENDING --> ERROR: Fallan Columnas/Estructura
    PROCESSING --> PROCESSED: Finalizan Workers de Reglas
    PROCESSING --> ERROR: Falla de Lote/Timeout
    PROCESSED --> [*]
    ERROR --> [*]
```

### Flujo de Estados de Póliza (Policy)
A nivel de fila única individual; el ciclo de vida de un asegurado/deudor a partir de las reglas asíncronas.

![State Diagram Policy](assets/diagram_2.png)

```mermaid
stateDiagram-v2
    [*] --> PROCESSING: Registro Extraído

    PROCESSING --> ACTIVE: Pasa Reglas + Prima > 0
    PROCESSING --> FROZEN: Pasa Reglas + Prima == 0
    PROCESSING --> REJECTED: Falla Reglas (Edad/Plan)

    ACTIVE --> CANCELLED: Ausente en nuevo archivo de Reconciliación
    FROZEN --> CANCELLED: Ausente en nuevo archivo de Reconciliación
    ACTIVE --> FROZEN: Prima cambia a 0 pesos en nuevo archivo
    FROZEN --> ACTIVE: Prima recupera su valor > 0 en nuevo archivo
    
    CANCELLED --> [*]
    REJECTED --> [*]
```

### Modelo de Componentes General (Estructura de Carpetas)
- `/api`: Endpoints REST Fiber/Gin.
- `/jobs`: Background Workers (FTP, Reconciliación).
- `/rules`: Motor de Validación Síncrono y Asíncrono.
- `/models`: Structs (Entidades) de la BD.

---

## 2. Arquitectura de Componentes (C4 Model)
El sistema está diseñado en Go (Golang) para alto rendimiento, interactuando con un servidor FTP externo y una base de datos PostgreSQL.

![Context Diagram](assets/diagram_3.png)

```mermaid
flowchart TD
    A([Aseguradora \n Sube pólizas])
    B[(Servidor FTP)]
    C{Busk Core Engine \n Procesa y Valida}
    D([Portal Web / Broker])
    
    A -- Sube XLSX/CSV --> B
    B -- Extrae archivos --> C
    C -- Expone stock validado --> D
```

### Componentes Internos (Go)

![Component Diagram](assets/diagram_4.png)

```mermaid
flowchart TD
    DB[(PostgreSQL \n Stock, Reglas y Tracking)]
    
    subgraph Core [Busk Core App - Go]
        direction TB
        W(FTP Watcher)
        P(File Parser)
        RE{Rule Engine}
        Auth(Auth Middleware \n JWT)
        API(RESTful API)
    end
    Broker([Broker Externo])
    
    W -- Envía RAW file --> P
    P -- Envía Batch --> RE
    RE -- Upsert/Reconcilia --> DB
    Broker -- Peticiones API --> Auth
    Auth -- Rutea req --> API
    API -- Consulta Stock --> DB
```

---

## 3. Flujo de Integración y Procesamiento (Secuencia)
El siguiente diagrama detalla la integración End-to-End desde que la aseguradora sube el archivo hasta que queda disponible en la API.

![Sequence Diagram](assets/diagram_5.png)

```mermaid
sequenceDiagram
    participant ASG as Aseguradora (Mapfre/Bolívar)
    participant FTP as FTP Server
    participant W as FTP Watcher (Go)
    participant RE as Rule Engine
    participant DB as PostgreSQL
    participant API as REST API

    ASG->>FTP: 1. Sube archivo Anexo (ej. MAPFRE_VIDA_01.xlsx)
    loop Monitoreo (Cron)
        W->>FTP: 2. Escanea nuevos archivos
        FTP-->>W: Retorna `MAPFRE_VIDA_01.xlsx`
    end
    
    W->>DB: 3. Inserta FILE_TRACKING (Status: PENDING)
    W->>RE: 4. Inicia Validación Síncrona (Estructura)
    RE->>DB: Consulta Mapeo de Columnas (product_id)
    
    alt Estructura Inválida (Faltan Cols)
        RE->>DB: 5a. Actualiza STATUS a ERROR
        RE-->>W: Rechazo Inmediato
    else Estructura Válida
        RE->>DB: 5b. Inserta RAW Records
        RE->>DB: Actualiza STATUS a PROCESSING
        RE-->>W: Inicia Workers Asíncronos
        
        loop Por cada Registro (Row)
            RE->>RE: 6. Aplica Regla (Edad, Plan, Tasa)
            
            alt Falla Regla (ej. Edad > 75)
                RE->>DB: Marca RECORD como VÁLIDO=False (Con Alerta)
            else Cumple Reglas
                RE->>DB: Marca RECORD como VÁLIDO=True
            end
        end
        
        RE->>DB: 7. Reconciliación de Stock (Cruza vs Activos)
        RE->>DB: Upsert persistente a tabla POLICY (Status: ACTIVE/FROZEN/CANCELLED)
        RE->>DB: 8. Actualiza STATUS final: PROCESSED
    end
    
    note right of API: El archivo ya está listo para consumo externo.
    
    ASG->>API: 9a. POST /api/v1/auth/login (Credentials)
    API-->>ASG: JWT Access Token
    
    ASG->>API: 9b. GET /api/v1/stock/{id}/policies (Header: Bearer Token)
    API->>API: 10. Auth Middleware Valida JWT
    API->>DB: 11. Cliente consulta datos
    DB-->>API: Retorna Data Unificada (JSON)
    API-->>ASG: Respuesta 200 OK (Snake_Case)
```

---

## 4. Jobs Asíncronos (Background Workers)
El procesamiento del sistema delega las tareas pesadas a colas asíncronas manejadas internamente por **Goroutines** (Go).

| Nombre del Job | Disparador | Tarea Principal |
|----------------|------------|-----------------|
| **`FTPWatcherJob`** | Cron (ej. cada 15 min) o `POST /process/scan` | Conecta al SFTP de Mapfre/Bolívar, identifica nuevos archivos no procesados y encola la validación síncrona. |
| **`FileProcessingJob`** | Éxito en Validación Estructural | Lee el archivo fila por fila, instancia el **Motor de Reglas**, re-calcula primas y marca cada `RECORD` como válido o inválido. |
| **`StockReconciliationJob`** | Fin exitoso del `FileProcessingJob` | Ejecuta el cruce (`DIFF`) masivo en PostgreSQL para detectar cuáles pólizas deben marcarse como `CANCELLED`, `FROZEN` o `ACTIVE`. |

---

## 5. Eventos de Dominio (Event Streaming)
Alineado con las directrices de Zalando, el motor emite **Eventos de Cambio de Datos (Data Change Events)** cuando se altera el stock. Estos eventos pueden ser consumidos por un bus de mensajería (ej. Kafka/RabbitMQ) para que otros microservicios actúen.

### Data Change Events:
- **`policy_created`**: Disparado cuando el motor detecta un `document_number` nuevo en el mes actual.
- **`policy_updated`**: Disparado cuando el deudor o asegurado tiene "novedades" (cambio en el valor cobrado o actualización de parentescos).
- **`policy_frozen`**: Emitido específicamente cuando una prima en 0 dispara la política de congelamiento.
- **`policy_cancelled`**: Emitido durante el `StockReconciliationJob` si el asegurado no aparece en el nuevo archivo mensual.

### Lifecycle Events (Flujo Operativo):
- **`file_validation_failed`**: Se emite si el Anexo viene corrupto, notificando al equipo de operaciones para que exija corrección a la aseguradora.
- **`file_processing_completed`**: Notifica que el lote completo terminó y el API ya expone el stock fresco.
