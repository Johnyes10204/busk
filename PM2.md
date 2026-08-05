# PM2 - Ejecutar Busk Seguros sin Docker

Ejecuta la API directamente en el servidor usando PM2 (Process Manager).

---

## 🚀 3 Pasos para Iniciar

### 1. Compilar binario

```bash
cd /home/lambda_buskseguros/busk
bash build.sh
```

Esto genera `./services/api/busk-api` (binario ejecutable).

### 2. Instalar PM2 (si no lo tienes)

```bash
npm install -g pm2
```

Verificar:
```bash
pm2 --version
```

### 3. Iniciar con PM2

```bash
pm2 start ecosystem.config.js

# Ver estado
pm2 status

# Ver logs
pm2 logs busk-api

# Ver logs en tiempo real
pm2 logs busk-api -f
```

**¡Listo!** API corriendo en `http://62.146.228.79:8080/api/v1/health`

---

## ⚙️ Configuración

### Editar variables de entorno

```bash
nano ecosystem.config.js
```

Actualiza:
- `MYSQL_DSN` — credenciales MySQL
- `SFTP_HOST`, `SFTP_USER`, `SFTP_PASSWORD` — credenciales SFTP
- `SENDGRID_API_KEY` — para notificaciones (opcional)
- `PROCESSOR_WORKERS` — workers paralelos (default: 2)

Luego reinicia:
```bash
pm2 restart busk-api
```

---

## 📋 Comandos Comunes

### Ver estado
```bash
pm2 status
pm2 list
```

### Ver logs
```bash
pm2 logs busk-api
pm2 logs busk-api -f          # En tiempo real
pm2 logs busk-api --lines 100 # Últimas 100 líneas
```

### Reiniciar
```bash
pm2 restart busk-api
pm2 restart all
```

### Detener
```bash
pm2 stop busk-api
pm2 stop all
```

### Iniciar de nuevo
```bash
pm2 start ecosystem.config.js
pm2 start busk-api
```

### Eliminar de PM2
```bash
pm2 delete busk-api
```

---

## 🔄 Auto-start en Reboot

Para que la API se inicie automáticamente cuando reinicia el servidor:

```bash
# Generar script systemd
pm2 startup systemd -u $USER --hp /home/$USER

# Guardar configuración actual
pm2 save

# Verificar
sudo systemctl status pm2-$USER
```

Ahora PM2 iniciará automáticamente al reiniciar. ✅

---

## 🐛 Troubleshooting

### "PM2 not found"
```bash
npm install -g pm2
```

### "Permission denied"
```bash
chmod +x services/api/busk-api
```

### "Port 8080 already in use"
```bash
# Ver qué está usando el puerto
sudo lsof -i :8080

# Matar proceso
sudo kill -9 <PID>

# O cambiar puerto en ecosystem.config.js
# (Requiere cambiar en código Go también)
```

### "Cannot connect to MySQL"
```bash
# Verificar credenciales en ecosystem.config.js
cat ecosystem.config.js | grep MYSQL_DSN

# Verificar MySQL está corriendo
sudo systemctl status mysql
mysql -u root -p
```

### Ver error completo
```bash
pm2 logs busk-api --err
```

---

## 📊 Monitoreo

### Ver uso de recursos en vivo
```bash
pm2 monit
```

### Logs con colores
```bash
pm2 logs busk-api --format
```

### Guardar logs a archivo
```bash
pm2 logs busk-api > logs/api.log
```

---

## 🆚 PM2 vs Docker

| Aspecto | PM2 | Docker |
|---------|-----|--------|
| Complejidad | Simple | Medio |
| Overhead | Bajo | Medio |
| Portabilidad | Depende del OS | Portable |
| MySQL local | ✅ Fácil | ⚠️ Necesita config |
| Logs | `pm2 logs` | `docker logs` |
| Auto-restart | ✅ Sí | ✅ Sí |
| Auto-start reboot | ✅ `pm2 startup` | ⚠️ Requiere systemd |

**PM2 es ideal para:**
- Servidor Linux con MySQL local
- Desarrollo/staging
- Simplificar el stack

---

## 🔗 Acceso desde Internet

La API está disponible en:

```
http://62.146.228.79:8080/api/v1/health
```

Si quieres HTTPS con Caddy:
```bash
# Iniciar Caddy en otro puerto
docker run -d \
  --name busk-caddy \
  -p 80:80 \
  -v $(pwd)/Caddyfile:/etc/caddy/Caddyfile:ro \
  caddy:2-alpine
```

---

## 📚 Próximos Pasos

1. ✅ Compilar: `bash build.sh`
2. ✅ Editar: `nano ecosystem.config.js`
3. ✅ Iniciar: `pm2 start ecosystem.config.js`
4. ✅ Verificar: `pm2 logs busk-api`

¡Listo! 🚀
