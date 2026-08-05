# Deploy en Servidor

Guía rápida para deployar Busk Seguros en servidor con MySQL local.

---

## 🚀 3 Pasos para Deployar

### 1. Copiar proyecto al servidor

```bash
# Desde tu máquina local
scp -r /ruta/a/busk usuario@servidor.com:/opt/busk

# O clonar directo en servidor
ssh usuario@servidor.com
cd /opt
git clone https://github.com/Johnyes10204/busk.git
cd busk
```

### 2. Crear `.env` con tus datos

```bash
nano .env
```

**Pegar esto, reemplazando valores:**

```env
# MySQL ya está instalado en el servidor
# Solo define credenciales (usuario debe existir en MySQL)
MYSQL_DSN=root:TU_CONTRASEÑA_AQUItcp(localhost:3306)/busk?parseTime=true&multiStatements=true

# Workers para procesar archivos en paralelo
PROCESSOR_WORKERS=2

# Archivos y reportes
FILES_ARCHIVE_DIR=/data/files-archive
REPORTS_ARCHIVE_DIR=/data/reports-archive

# SFTP de aseguradoras (IMPORTANTE)
SFTP_HOST=sftp.example.com
SFTP_PORT=22
SFTP_USER=tu_usuario
SFTP_PASSWORD=tu_password
SFTP_REMOTE_DIR=/incoming

# Email (opcional - si quieres notificaciones)
SENDGRID_API_KEY=SG.xxxxx
SENDGRID_FROM_EMAIL=noreply@busk.com
SENDGRID_ERROR_TO_EMAILS=ops@busk.com
```

**Guardar:** Ctrl+X → Y → Enter

### 3. Iniciar API en Docker

```bash
# Usar docker-compose.servidor.yml (conecta a MySQL local)
docker-compose -f docker-compose.servidor.yml up -d

# Ver logs
docker-compose -f docker-compose.servidor.yml logs -f api

# Ver status
docker-compose -f docker-compose.servidor.yml ps
```

**¡Listo!** API disponible en `http://servidor.com:8080`

---

## 📋 Verificación

### Health check
```bash
curl http://localhost:8080/api/v1/health
# {"status":"ok","time":"2026-08-04T12:00:00Z"}
```

### Ver logs
```bash
docker-compose -f docker-compose.servidor.yml logs api | head -50
```

### Archivos escaneados
```bash
# Los archivos SFTP se descargan aquí
ls -la /opt/busk/services/api/data/files-archive/
```

---

## 🔧 Pre-requisitos en Servidor

Verificar que MySQL esté listo:

```bash
# Ver si MySQL está corriendo
sudo systemctl status mysql

# Conectar a MySQL
mysql -u root -p

# Dentro de MySQL, crear usuario:
CREATE USER 'busk'@'localhost' IDENTIFIED BY 'busk123';
CREATE DATABASE busk;
GRANT ALL PRIVILEGES ON busk.* TO 'busk'@'localhost';
FLUSH PRIVILEGES;
EXIT;
```

---

## 🎯 Diferencias: docker-compose.yml vs docker-compose.servidor.yml

| Archivo | Uso | MySQL |
|---------|-----|-------|
| `docker-compose.yml` | Desarrollo/Test completo | En Docker (incluido) |
| `docker-compose.servidor.yml` | Servidor con MySQL local | En servidor (local) |

**Elegir según contexto:**

```bash
# En desarrollo (con todo en Docker)
docker-compose up -d

# En servidor (MySQL ya existe)
docker-compose -f docker-compose.servidor.yml up -d
```

---

## 🔄 Alias rápido (opcional)

Para no escribir `docker-compose -f docker-compose.servidor.yml` cada vez:

```bash
# Agregar al .bashrc o .zshrc
alias dc-srv='docker-compose -f docker-compose.servidor.yml'

# Luego usar:
dc-srv up -d
dc-srv logs -f api
dc-srv ps
```

---

## 🚀 Auto-start en reinicio (systemd)

Para que API se inicie automáticamente cuando reinicia el servidor:

```bash
sudo nano /etc/systemd/system/busk-api.service
```

**Pegar:**
```ini
[Unit]
Description=Busk Seguros API - Docker
After=docker.service mysql.service
Wants=mysql.service network-online.target
StartLimitIntervalSec=200
StartLimitBurst=5

[Service]
Type=oneshot
WorkingDirectory=/opt/busk
ExecStart=/usr/bin/docker-compose -f docker-compose.servidor.yml up -d
ExecStop=/usr/bin/docker-compose -f docker-compose.servidor.yml down
RemainAfterExit=yes
Restart=on-failure
RestartSec=30
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

**Habilitar:**
```bash
sudo systemctl daemon-reload
sudo systemctl enable busk-api.service
sudo systemctl start busk-api.service
sudo systemctl status busk-api.service

# Ver logs
sudo journalctl -u busk-api -f
```

---

## 📊 Monitoreo

### Ver estado en tiempo real
```bash
docker-compose -f docker-compose.servidor.yml ps
docker-compose -f docker-compose.servidor.yml logs -f api
```

### Ver uso de recursos
```bash
docker stats busk-api
```

### Ver archivos procesados
```bash
ls -lh /opt/busk/services/api/data/files-archive/
ls -lh /opt/busk/services/api/data/reports-archive/
```

---

## 🐛 Troubleshooting

### Error: "Cannot connect to MySQL"
```bash
# Verificar que MySQL esté corriendo
sudo systemctl status mysql

# Verificar credenciales
mysql -u busk -pbusk123 busk

# Revisar logs API
docker-compose -f docker-compose.servidor.yml logs api | grep -i mysql
```

### Error: "Port 8080 already in use"
```bash
# Cambiar puerto en .env
API_PORT=9090

# Reiniciar
docker-compose -f docker-compose.servidor.yml restart
```

### Error: "SFTP connection timeout"
```bash
# Verificar credenciales en .env
cat .env | grep SFTP

# Ver logs
docker-compose -f docker-compose.servidor.yml logs api | grep -i sftp
```

### Limpiar y reiniciar
```bash
docker-compose -f docker-compose.servidor.yml down
docker rmi busk-api:latest
docker-compose -f docker-compose.servidor.yml up -d
```

---

## 📞 Checklist de Deploy

- [ ] Proyecto copiado a `/opt/busk`
- [ ] `.env` creado con credenciales reales
- [ ] MySQL corriendo en servidor
- [ ] Usuario `busk` existe en MySQL
- [ ] Base de datos `busk` existe
- [ ] `docker-compose.servidor.yml` presente
- [ ] `docker-compose -f docker-compose.servidor.yml up -d` ejecutado
- [ ] `curl http://localhost:8080/api/v1/health` devuelve OK
- [ ] Logs muestran "AUTO-SCAN scheduler iniciado"
- [ ] Archivos SFTP aparecen en `/data/files-archive/`

---

## 🎯 Flujo Operacional

```
1. API inicia en Docker
   ↓
2. Se conecta a MySQL local
   ↓
3. Se crea tabla de migraciones (automático)
   ↓
4. Auto-scan inicia (cada 1 hora)
   ↓
5. Descarga archivos de SFTP
   ↓
6. Valida según reglas de negocio
   ↓
7. Guarda en BD
   ↓
8. Genera reportes
   ↓
9. Envía email (si configurado)
```

---

**Versión:** 1.0  
**Última actualización:** 2026-08-04  
**Contacto:** ops@busk.com
