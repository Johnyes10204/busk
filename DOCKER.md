# Ejecutar Busk Seguros en Docker

## 🚀 Quick Start (3 pasos)

### 1. Clonar repo y entrar a carpeta
```bash
cd /path/to/busk
```

### 2. Crear `.env` con variables
```bash
cat > .env << EOF
MYSQL_ROOT_PASSWORD=root
MYSQL_PASSWORD=busk123
MYSQL_DATABASE=busk
PROCESSOR_WORKERS=2
SFTP_HOST=sftp.example.com
SFTP_USER=user
SFTP_PASSWORD=pass
SFTP_REMOTE_DIR=/incoming
SENDGRID_API_KEY=SG.xxxxx
SENDGRID_FROM_EMAIL=noreply@busk.com
SENDGRID_ERROR_TO_EMAILS=ops@busk.com
EOF
```

### 3. Iniciar todo
```bash
docker-compose up -d
```

**¡Listo!** API disponible en `http://localhost:8080`

---

## 📋 Comandos Comunes

### Ver logs en tiempo real
```bash
docker-compose logs -f api
```

### Ver solo logs de MySQL
```bash
docker-compose logs -f mysql
```

### Ejecutar comando en API
```bash
docker-compose exec api ./busk-api
```

### Reiniciar servicios
```bash
docker-compose restart
```

### Detener sin borrar datos
```bash
docker-compose stop
```

### Detener y borrar TODO (incluyendo BD)
```bash
docker-compose down -v
```

### Ver estado de contenedores
```bash
docker-compose ps
```

### Acceder a MySQL desde terminal
```bash
docker-compose exec mysql mysql -uroot -proot busk
```

---

## 🔧 Configuración

### Variables de entorno (en `.env`)

| Variable | Defecto | Descripción |
|----------|---------|-------------|
| `MYSQL_ROOT_PASSWORD` | `root` | Password del usuario root MySQL |
| `MYSQL_PASSWORD` | `busk123` | Password del usuario `busk` |
| `MYSQL_DATABASE` | `busk` | Nombre de la BD |
| `MYSQL_PORT` | `3306` | Puerto MySQL (interno) |
| `API_PORT` | `8080` | Puerto API (externo) |
| `PROCESSOR_WORKERS` | `2` | Workers paralelos para procesar |
| `FILES_ARCHIVE_DIR` | `/data/files-archive` | Carpeta archivos procesados |
| `REPORTS_ARCHIVE_DIR` | `/data/reports-archive` | Carpeta reportes |
| `SFTP_HOST` | (vacío) | Host SFTP de aseguradoras |
| `SFTP_PORT` | `22` | Puerto SFTP |
| `SFTP_USER` | (vacío) | Usuario SFTP |
| `SFTP_PASSWORD` | (vacío) | Password SFTP |
| `SFTP_REMOTE_DIR` | `/incoming` | Directorio remoto SFTP |
| `SENDGRID_API_KEY` | (vacío) | API key SendGrid (opcional) |
| `SENDGRID_FROM_EMAIL` | (vacío) | Email remitente (opcional) |
| `SENDGRID_ERROR_TO_EMAILS` | (vacío) | Emails para errores (opcional) |

---

## 🏗️ En Servidor (producción)

### Opción 1: Docker Host Directo

#### 1. Copiar a servidor
```bash
scp -r /path/to/busk user@servidor.com:/opt/busk
ssh user@servidor.com
cd /opt/busk
```

#### 2. Crear `.env` con credenciales seguras
```bash
# Editar con valores reales
nano .env
```

#### 3. Iniciar
```bash
docker-compose up -d
```

#### 4. Verificar
```bash
docker-compose ps
docker-compose logs -f api
curl http://localhost:8080/api/v1/health
```

---

### Opción 2: Con systemd (auto-start en reinicio)

#### 1. Crear servicio systemd
```bash
sudo nano /etc/systemd/system/busk.service
```

#### Contenido:
```ini
[Unit]
Description=Busk Seguros - Docker Compose
After=docker.service network-online.target
Wants=network-online.target

[Service]
Type=oneshot
WorkingDirectory=/opt/busk
ExecStart=/usr/bin/docker-compose up -d
ExecStop=/usr/bin/docker-compose down
RemainAfterExit=yes
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

#### 2. Habilitar y iniciar
```bash
sudo systemctl daemon-reload
sudo systemctl enable busk.service
sudo systemctl start busk.service
sudo systemctl status busk.service
```

#### 3. Ver logs del servicio
```bash
sudo systemctl logs -u busk -f
```

---

### Opción 3: Docker Compose con Nginx Reverso

#### `docker-compose.yml` (versión con Nginx):
```yaml
version: "3.9"

services:
  mysql:
    # ... igual que arriba
  
  api:
    # ... igual que arriba
    expose:
      - 8080
    networks:
      - busk-network
  
  nginx:
    image: nginx:alpine
    container_name: busk-nginx
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./certs:/etc/nginx/certs:ro
    depends_on:
      - api
    networks:
      - busk-network
```

#### `nginx.conf` (proxy reverso):
```nginx
events { worker_connections 1024; }
http {
  upstream api {
    server api:8080;
  }
  server {
    listen 80;
    server_name busk.example.com;
    location / {
      proxy_pass http://api;
      proxy_set_header Host $host;
      proxy_set_header X-Real-IP $remote_addr;
    }
  }
}
```

---

## 📊 Monitoreo

### Health check
```bash
curl http://localhost:8080/api/v1/health
# {"status":"ok","time":"2026-08-04T12:00:00Z"}
```

### Ver contenedores
```bash
docker ps | grep busk
```

### Estadísticas recursos
```bash
docker stats busk-api busk-mysql
```

### Logs persistentes
Los logs se guardan en `/var/lib/docker/containers/...` (automático)

---

## 🐛 Troubleshooting

### Error: "MySQL connection refused"
```bash
# Esperar a que MySQL esté listo
docker-compose logs mysql
# Reintentar API
docker-compose restart api
```

### Error: "Port 3306 already in use"
```bash
# Liberar puerto o cambiar en .env
MYSQL_PORT=3307
docker-compose down
docker-compose up -d
```

### Error: "SFTP connection timeout"
```bash
# Verificar credenciales SFTP en .env
docker-compose exec api wget -qO- http://localhost:8080/api/v1/health
# Revisar logs
docker-compose logs api | grep SFTP
```

### Limpiar todo (start fresh)
```bash
docker-compose down -v
docker rmi busk-api:latest
docker-compose up -d
```

---

## 🔐 Seguridad (Producción)

### 1. Cambiar passwords por defecto
```bash
MYSQL_ROOT_PASSWORD=<strong-password>
MYSQL_PASSWORD=<strong-password>
```

### 2. Usar secretos en lugar de .env (Docker Swarm/K8s)
```bash
echo "secret-password" | docker secret create mysql_password -
```

### 3. Usar HTTPS con Let's Encrypt
```bash
# Con Nginx + Certbot
docker run --rm -it -v /opt/busk/certs:/etc/letsencrypt \
  certbot/certbot certonly -d busk.example.com
```

### 4. Límites de recursos
```yaml
api:
  deploy:
    resources:
      limits:
        cpus: '1'
        memory: 1G
      reservations:
        cpus: '0.5'
        memory: 512M
```

---

## 📈 Escalado

### Múltiples workers API (con Nginx)
```yaml
api:
  build: .
  deploy:
    replicas: 3  # 3 instancias
  environment:
    PROCESSOR_WORKERS: 2
```

Con Nginx load balancing automático.

---

## 📞 Soporte

- Logs: `docker-compose logs -f api`
- Status: `docker-compose ps`
- Health: `curl http://localhost:8080/api/v1/health`

---

**Versión:** 1.0  
**Última actualización:** 2026-08-04
