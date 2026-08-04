# PROPUESTA COMERCIAL DE SERVICIOS
## Infraestructura, Despliegue y Administración — Busk Seguros

| | |
|--|--|
| **Fecha** | 27 de julio de 2026 |
| **Proyecto** | Sistema de Ingesta de Pólizas — Busk Seguros |
| **Validez** | 30 días calendario |
| **Elaborado por** | [Tu nombre / empresa] |
| **Contacto** | [Email] — [Teléfono] |

---

## 1. OBJETO DEL SERVICIO

Implementación, despliegue y administración mensual del sistema **Busk Seguros**: plataforma de procesamiento automatizado de pólizas de seguros que descarga archivos XLSX/XLS desde un servidor SFTP de aseguradoras (MAPFRE, Bolívar), los valida, los materializa en base de datos MySQL y envía notificaciones automáticas de resultado por correo electrónico.

---

## 2. STACK TECNOLÓGICO DEL SISTEMA

| Componente | Tecnología | Función |
|------------|-----------|---------|
| API Backend | Go 1.23 | Motor principal: descarga SFTP, valida pólizas, escribe en BD |
| Frontend Admin | React 19 + Vite + TypeScript | Panel de control: archivos, pólizas y reportes |
| Base de datos | MySQL 8.x | Pólizas, archivos procesados y configuración de productos |
| Servidor web | Nginx | Proxy reverso HTTPS → API + entrega del frontend |
| Certificado SSL | Let's Encrypt / ACM | HTTPS con renovación automática |
| Seguridad | Fail2ban + Firewall | Bloqueo de accesos no autorizados |
| Monitoreo | UptimeRobot / CloudWatch | Disponibilidad 24/7 con alertas |
| Auto-arranque | systemd | Reinicio automático del API ante fallos |
| Notificaciones | SendGrid | Emails éxito/error por archivo *(cubierto por el cliente)* |

---

## 3. OPCIÓN A — AMAZON WEB SERVICES (AWS) ★ Principal

### Arquitectura

```
Internet
    │
    ▼
Route 53 (DNS) + ACM (SSL gratuito)
    │
    ▼
Application Load Balancer (HTTPS :443)
    │
    ▼
VPC Privada
├── EC2 t3.large  →  Go API + Frontend React
│       │
│       └── NAT Gateway → SFTP Aseguradoras (externo)
│
├── RDS MySQL 8.x (subred privada, siempre disponible)
│
└── S3  →  Archivos XLSX + Reportes de validación
```

### Detalle de servicios

| Servicio | Especificación | USD/mes | COP/mes* |
|----------|---------------|---------|---------|
| **EC2 t3.large** | 2 vCPU, 8 GB RAM — API Go + Frontend | $60.74 | $255,108 |
| **EBS gp3 30 GB** | Disco raíz EC2 | $2.40 | $10,080 |
| **Elastic IP** | IP fija (no cambia al reiniciar) | $0.00 | $0 |
| **RDS MySQL 8.x db.t3.medium** | 2 vCPU, 4 GB RAM — Single-AZ | $52.00 | $218,400 |
| **RDS Storage gp3 50 GB** | + backups automáticos 7 días | $5.75 | $24,150 |
| **Application Load Balancer** | HTTPS, SSL termination | $16.43 | $69,006 |
| **ALB LCU** | Tráfico moderado | $1.50 | $6,300 |
| **NAT Gateway** | Salida segura al SFTP de aseguradoras | $32.85 | $137,970 |
| **NAT Data Processing** | ~5 GB archivos SFTP/mes | $0.23 | $966 |
| **Route 53** | DNS del dominio | $1.00 | $4,200 |
| **ACM SSL/TLS** | Certificado HTTPS | $0.00 | $0 |
| **S3 Standard 50 GB** | `files-archive/` + `reports-archive/` | $1.15 | $4,830 |
| **S3 Requests** | PUT/GET archivos XLSX y reportes | $0.50 | $2,100 |
| **CloudWatch Logs** | Logs del API Go | $2.00 | $8,400 |
| **CloudWatch Alarms** | Auto-recovery EC2 + alerta RDS | $0.30 | $1,260 |
| **Data Transfer Out** | ~20 GB salida a internet | $1.80 | $7,560 |
| **SendGrid** | Notificaciones email | $0.00 | $0 *(cubierto por cliente)* |
| **Subtotal infraestructura AWS** | | **$178.65** | **$750,330** |
| **Administración, gestión y mantenimiento (35%)** | Despliegue inicial, monitoreo 24/7, soporte ante incidentes, actualizaciones de seguridad, gestión de BD, renovación SSL y revisiones mensuales de rendimiento | **$62.53** | **$262,616** |
| **TOTAL MENSUAL AWS** | | **$241.18 USD** | **$1,012,946 COP** |

*\*Tasa de referencia: 1 USD = 4,200 COP*

### Proyección AWS

| Período | USD | COP |
|---------|-----|-----|
| Mensual | $241.18 | $1,012,946 |
| Trimestral | $723.54 | $3,038,838 |
| Semestral | $1,447.08 | $6,077,736 |
| **Anual** | **$2,894.16** | **$12,155,472** |

### Ahorro con Reserved Instances (compromiso 1 año)

| Ajuste | USD/mes | COP/mes |
|--------|---------|---------|
| EC2 t3.large Reserved (~36% dto.) | -$21.87 | -$91,854 |
| RDS db.t3.medium Reserved (~36% dto.) | -$18.72 | -$78,624 |
| **Ahorro mensual** | **-$40.59** | **-$170,478** |
| **Total mensual con Reserved + gestión** | **~$200 USD** | **~$842,468 COP** |

### Por qué AWS

| Ventaja | Detalle |
|---------|---------|
| Alta disponibilidad | RDS siempre activo e independiente del API, Auto Recovery EC2 |
| Escalabilidad automática | Crece con el volumen sin migrar de proveedor |
| Seguridad enterprise | VPC privada, IAM, Security Groups, cifrado en tránsito y reposo |
| BD gestionada | RDS maneja backups, parches y failover automáticamente |
| S3 para archivos | Almacenamiento ilimitado de XLSX y reportes con versionado |
| Cumplimiento | Certificaciones SOC 2, ISO 27001, PCI DSS |
| Soporte AWS | Plans de soporte técnico disponibles 24/7 |

---

## 4. OPCIÓN B — HOSTINGER VPS KVM 8

### Especificaciones del servidor

| Especificación | Detalle |
|----------------|---------|
| **Procesador** | 8 vCPU AMD EPYC |
| **Memoria RAM** | 32 GB |
| **Almacenamiento** | 400 GB NVMe SSD |
| **Ancho de banda** | 32 TB/mes |
| **Velocidad de red** | 1 Gbps |
| **Precio promocional** | COP 77,900/mes |
| **Precio de renovación** | COP 168,900/mes (plan 2 años) |
| **Disponibilidad** | 24/7 — 730 horas/mes |
| **Incluido** | Firewall, backups semanales, soporte 24/7, dominio gratis 1er año |

### Cotización Hostinger KVM 8

| Componente | COP/mes |
|------------|---------|
| VPS Hostinger KVM 8 | $168,900 |
| Backups diarios automáticos | $16,800 |
| MySQL / Nginx / HestiaCP / SSL / Monitoreo | $0 |
| SendGrid | $0 *(cubierto por cliente)* |
| **TOTAL MENSUAL** | **$250,695 COP** |

### Proyección Hostinger KVM 8

| Período | COP |
|---------|-----|
| Mensual | $250,695 |
| Trimestral | $752,085 |
| Semestral | $1,504,170 |
| **Anual** | **$3,008,340** |

---

## 5. COMPARATIVO

| Criterio | AWS On-Demand | AWS Reserved 1 año | Hostinger KVM 8 |
|----------|:-------------:|:-----------------:|:---------------:|
| vCPU | 2 | 2 | 8 |
| RAM | 8 GB | 8 GB | 32 GB |
| Almacenamiento | 80 GB EBS + S3 ∞ | 80 GB EBS + S3 ∞ | 400 GB NVMe |
| Base de datos | RDS dedicado | RDS dedicado | MySQL en VPS |
| SSL | ACM gratuito | ACM gratuito | Let's Encrypt gratuito |
| Backups | 7 días automáticos | 7 días automáticos | Diarios |
| Escalabilidad | Automática | Automática | Manual |
| Alta disponibilidad | ✓ (Multi-AZ opcional) | ✓ (Multi-AZ opcional) | — |
| Panel gestión | Consola AWS | Consola AWS | HestiaCP |
| Subtotal infraestructura | $750,330 COP | $579,852 COP | $185,700 COP |
| Gestión 35% | $262,616 COP | $202,948 COP | $64,995 COP |
| **Total/mes** | **$1,012,946 COP** | **~$842,468 COP** | **$250,695 COP** |
| **Total/año** | **$12,155,472 COP** | **~$10,109,616 COP** | **$3,008,340 COP** |

---

## 6. SERVICIOS DE ADMINISTRACIÓN Y GESTIÓN (incluidos en el 35%)

| Servicio | Descripción |
|----------|-------------|
| Despliegue inicial | Configuración completa del servidor, dominio, SSL y todos los servicios |
| Monitoreo 24/7 | Supervisión continua con alertas ante caídas o errores |
| Gestión de seguridad | Actualizaciones del SO, revisión de accesos y parches |
| Administración de BD | Backups, optimización de tablas y revisión de integridad |
| Gestión de certificados | Renovación automática SSL sin interrupciones |
| Soporte ante incidentes | Atención y resolución ante fallos del sistema o servidor |
| Revisión mensual | Análisis de logs, rendimiento y recomendaciones de mejora |
| Actualizaciones | Despliegue de nuevas versiones cuando se requiera |

---

## 7. CONDICIONES COMERCIALES

| Condición | Detalle |
|-----------|---------|
| Forma de pago | Mensual anticipado |
| Contrato mínimo | 3 meses |
| Tiempo de implementación | 3 a 5 días hábiles |
| Medio de pago | Transferencia bancaria / PSE |
| Precios | No incluyen IVA |
| Vigencia propuesta | 30 días calendario desde la fecha de emisión |

---

## 8. PRÓXIMOS PASOS

1. Selección de la opción de infraestructura (AWS o Hostinger KVM 8)
2. Aprobación de la propuesta comercial
3. Firma de contrato de servicios
4. Pago del primer mes anticipado
5. Inicio de configuración e implementación (3–5 días hábiles)
6. Entrega con ambiente en producción funcionando al 100%

---

*[Tu nombre / empresa] — [Email] — [Teléfono]*
*Fecha: 27 de julio de 2026*

---

<!-- PROMPT PARA GENERAR PDF
Pega todo el contenido de este archivo en el chat y usa el siguiente prompt:

"Convierte este documento Markdown en un PDF profesional con las siguientes características:
- Diseño corporativo con encabezado en azul oscuro (#1a2e4a) y texto blanco
- Logo en la esquina superior derecha si se proporciona, si no, solo el título
- Tablas con filas alternas en gris claro para mejor lectura
- Bloques de código con fondo gris y tipografía monospace
- Tipografía limpia: títulos en bold, cuerpo en 11pt
- Márgenes adecuados para impresión en papel A4
- Numeración de páginas en el pie de página
- Línea separadora al final de cada sección
- El bloque de arquitectura ASCII debe respetarse tal como está
- Resalta en negrita los totales de cada opción de cotización"
-->
