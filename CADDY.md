# Caddy - Reverse Proxy HTTPS para Busk Seguros

Caddy maneja automáticamente certificados SSL/TLS y actúa como reverse proxy entre internet y la API.

---

## 🚀 Configuración Rápida

### 1. Editar Caddyfile según tu caso

**Caso 1: Con dominio (Let's Encrypt automático)**

```
api.busk.com {
	reverse_proxy http://api:8080 {
		header_up X-Forwarded-For {http.request.remote.host}
		header_up X-Forwarded-Proto {http.request.proto}
		header_up X-Forwarded-Host {http.request.host}
	}
	log {
		output stdout
		format json
	}
}
```

Caddy **obtiene certificado automáticamente** de Let's Encrypt. Los puertos 80 y 443 se exponen automáticamente.

**Caso 2: Con IP + puerto específico**

```
:8081 {
	reverse_proxy http://api:8080 {
		header_up X-Forwarded-For {http.request.request.remote.host}
		header_up X-Forwarded-Proto {http.request.proto}
		header_up X-Forwarded-Host {http.request.host}
	}
	log {
		output stdout
		format json
	}
}
```

Acceso: `https://62.146.228.79:8081/api/health`

**Caso 3: Con certificado auto-firmado (para testing sin dominio)**

```
:443 {
	tls internal
	reverse_proxy http://api:8080 {
		header_up X-Forwarded-For {http.request.remote.host}
		header_up X-Forwarded-Proto {http.request.proto}
		header_up X-Forwarded-Host {http.request.host}
	}
	log {
		output stdout
		format json
	}
}
```

Nota: El navegador mostrará alerta de certificado no confiable (es normal con auto-signed).

---

## 📝 Caddyfile - Explicación Línea por Línea

```caddyfile
:8081 {                              # Escucha en puerto 8081
	reverse_proxy http://api:8080 {  # Redirecciona a API interna
		header_up X-Forwarded-For {http.request.remote.host}
		header_up X-Forwarded-Proto {http.request.proto}
		header_up X-Forwarded-Host {http.request.host}
	}
	log {
		output stdout               # Logs a stdout (visible en docker logs)
		format json
	}
}
```

---

## 🔧 Pasos para Deployar

### 1. Editar Caddyfile en servidor

```bash
nano Caddyfile
```

Reemplaza `:8081` con tu dominio/IP:puerto:

```
# OPCIÓN A: Con dominio
api.busk.com {
	reverse_proxy http://api:8080 {
		header_up X-Forwarded-For {http.request.remote.host}
		header_up X-Forwarded-Proto {http.request.proto}
		header_up X-Forwarded-Host {http.request.host}
	}
}

# OPCIÓN B: Con IP
:8081 {
	reverse_proxy http://api:8080 {
		header_up X-Forwarded-For {http.request.remote.host}
		header_up X-Forwarded-Proto {http.request.proto}
		header_up X-Forwarded-Host {http.request.host}
	}
}
```

### 2. Iniciar con docker-compose

```bash
docker-compose -f docker-compose.servidor.yml up -d
```

Esto inicia **dos contenedores**:
- `busk-api` en puerto 8080 (interno)
- `busk-caddy` en puerto 8081/443 (expuesto a internet)

### 3. Verificar

```bash
# ¿Caddy está corriendo?
docker ps | grep caddy

# ¿Ver logs de Caddy?
docker logs busk-caddy

# ¿Responde HTTPS?
curl -k https://62.146.228.79:8081/api/v1/health
# (-k ignora error de certificado auto-firmado)
```

---

## 🌐 Casos de Uso

### Con Dominio (Recomendado)

```caddyfile
api.busk.com {
	reverse_proxy http://api:8080 {
		header_up X-Forwarded-For {http.request.remote.host}
		header_up X-Forwarded-Proto {http.request.proto}
		header_up X-Forwarded-Host {http.request.host}
	}
	log {
		output stdout
		format json
	}
}

# Redirigir HTTP → HTTPS automático (Caddy lo hace por defecto)
```

✅ Certificado Let's Encrypt automático  
✅ HTTPS en puerto estándar 443  
✅ Requiere que el dominio apunte al servidor

### Sin Dominio (Solo IP)

```caddyfile
:8081 {
	reverse_proxy http://api:8080 {
		header_up X-Forwarded-For {http.request.remote.host}
		header_up X-Forwarded-Proto {http.request.proto}
		header_up X-Forwarded-Host {http.request.host}
	}
	log {
		output stdout
		format json
	}
}
```

Acceso: `https://62.146.228.79:8081/api/v1/health`

⚠️ Certificado auto-signed (navegador mostrará alerta)

### Con Múltiples Dominios

```caddyfile
api.busk.com, api.staging.busk.com {
	reverse_proxy http://api:8080 {
		header_up X-Forwarded-For {http.request.remote.host}
		header_up X-Forwarded-Proto {http.request.proto}
		header_up X-Forwarded-Host {http.request.host}
	}
}
```

---

## 🔐 HTTPS Automático

### Con Dominio

Caddy **obtiene certificado de Let's Encrypt automáticamente**:
- Verifica que dominio apunte a servidor
- Descarga certificado
- Renueva automáticamente cada 90 días
- Redirige HTTP → HTTPS

### Con IP + Puerto

Usa certificado auto-firmado (no es trusted, pero funciona):
```caddyfile
:8081 {
	reverse_proxy http://api:8080
}
```

Para ignorar error en cliente:
```bash
curl -k https://62.146.228.79:8081/api/health
```

---

## 📊 Headers Preservados

Caddy envía estos headers a la API para preservar información:

```
X-Forwarded-For: <IP cliente original>
X-Forwarded-Proto: https
X-Forwarded-Host: 62.146.228.79:8081
```

La API puede usar estos headers en logs para saber la IP real del cliente.

---

## 🛠️ Troubleshooting

### "connection refused"

```bash
docker logs busk-caddy
```

Si dice "error connecting to api:8080", la API no está corriendo:

```bash
docker logs busk-api
docker ps
```

### "certificate error"

Si usas IP sin dominio, es normal. Ignora con `-k`:

```bash
curl -k https://62.146.228.79:8081/api/v1/health
```

### "port already in use"

```bash
# Cambiar puerto en Caddyfile
:9081 {
	reverse_proxy http://api:8080
}
```

### Ver logs en vivo

```bash
docker logs -f busk-caddy
```

---

## 📦 Volúmenes de Caddy

El docker-compose crea volúmenes para persistencia:

```
caddy-data   → Almacena certificados SSL
caddy-config → Almacena configuración
```

Los certificados se mantienen incluso si reinicia el contenedor.

---

## 🔄 Reiniciar Después de Cambiar Caddyfile

```bash
# Editar Caddyfile
nano Caddyfile

# Reiniciar Caddy (la API no se afecta)
docker restart busk-caddy

# Verificar logs
docker logs busk-caddy
```

---

## ⚡ Ejemplo Completo

### Archivo: Caddyfile

```
62.146.228.79:8081 {
	reverse_proxy http://api:8080 {
		header_up X-Forwarded-For {http.request.remote.host}
		header_up X-Forwarded-Proto {http.request.proto}
		header_up X-Forwarded-Host {http.request.host}
	}
	log {
		output stdout
		format json
	}
}
```

### Comando para iniciar

```bash
docker-compose -f docker-compose.servidor.yml up -d
```

### Acceder

```bash
curl -k https://62.146.228.79:8081/api/v1/health
```

### Ver logs

```bash
docker logs busk-caddy
```

---

**¡Listo!** API expuesta en HTTPS sin instalar nada en el servidor. 🚀
