# Quick Start - Busk Seguros

Guía rápida para iniciar la API en 2 minutos.

---

## 🚀 Inicio Rápido (Opción 1: Script)

### En servidor con MySQL local:
```bash
bash start.sh servidor
```

### En desarrollo (todo en Docker):
```bash
bash start.sh local
```

### Ver logs:
```bash
bash start.sh logs
```

### Ver estado:
```bash
bash start.sh status
```

### Detener:
```bash
bash start.sh stop
```

---

## 📋 Inicio Manual (Opción 2: Docker Compose)

### 1. Preparar configuración
```bash
# Copiar template
cp .env.example .env

# Editar con tus valores
nano .env

# Agregar credenciales SFTP, SendGrid, etc.
```

### 2. Iniciar

**Servidor (MySQL local):**
```bash
docker-compose -f docker-compose.servidor.yml up -d
```

**Desarrollo (MySQL en Docker):**
```bash
docker-compose up -d
```

### 3. Verificar
```bash
# Ver estado
docker ps

# Health check
curl http://localhost:8080/api/v1/health

# Ver logs
docker-compose logs -f api
```

---

## ⚡ Variables Importantes

**Crear `.env` desde `.env.example`:**
```bash
cp .env.example .env
```

**Variables críticas:**
- `MYSQL_DSN` — Conexión a MySQL
- `SFTP_HOST`, `SFTP_USER`, `SFTP_PASSWORD` — Credenciales SFTP
- `SENDGRID_API_KEY` — Para notificaciones (opcional)

---

## ✅ Verificación

```bash
# ¿API responde?
curl http://localhost:8080/api/v1/health

# ¿Logs sin errores?
docker-compose logs api

# ¿Contenedores corriendo?
docker ps
```

---

## 🆘 Troubleshooting

### "Port 3306 already in use"
```bash
# Usar versión servidor
docker-compose -f docker-compose.servidor.yml up -d
```

### "Cannot connect to MySQL"
```bash
# Verificar .env
cat .env | grep MYSQL_DSN

# Verificar MySQL en servidor
sudo systemctl status mysql
mysql -u busk -pbusk123 busk
```

### "API no responde"
```bash
# Ver logs
docker-compose logs api | tail -20

# Reiniciar
docker-compose restart api
```

---

## 📚 Más Información

- **Docker:** Ver `DOCKER.md`
- **Deploy:** Ver `DEPLOY.md`
- **Config:** Ver `.env.example`

---

**¡Listo! 🚀**