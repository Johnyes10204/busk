# Docker Compose Unificado (API + Observabilidad)

Este proyecto ya puede ejecutarse con un solo `docker compose` levantando:

- API Busk (`api`)
- Prometheus (`prometheus`)
- Grafana (`grafana`)

## MySQL en la maquina host

Por defecto el `docker-compose.yml` conecta el API a MySQL instalado en tu Mac usando:

- `DB_HOST=host.docker.internal`

Asegurate de que MySQL en el host:

- escuche en `0.0.0.0:3306` (no solo `127.0.0.1`, si quieres compatibilidad maxima)
- tenga creada la base `busk_seguros` (o cambia `DB_NAME`)
- tenga credenciales validas (por defecto el compose usa `root` sin password; ajustalo)

## 1) Levantar todo

Desde la raiz del repo:

```bash
docker compose up -d --build
```

## 2) URLs

- API: [http://localhost:3000](http://localhost:3000)
- Swagger UI: [http://localhost:3000/api/v1/swagger](http://localhost:3000/api/v1/swagger)
- Metrics Prometheus: [http://localhost:3000/metrics](http://localhost:3000/metrics)
- Prometheus UI: [http://localhost:9090](http://localhost:9090)
- Grafana UI: [http://localhost:3001](http://localhost:3001) (`admin` / `admin`)

## 3) Comandos utiles

```bash
# Ver estado
docker compose ps

# Ver logs API
docker compose logs -f api

# Bajar stack
docker compose down

# Bajar stack y borrar datos persistidos de Grafana
docker compose down -v
```

## 4) Variables relevantes

El compose define variables por defecto para entorno local:

- DB host: `host.docker.internal`
- DB user/pass: `root` / vacio (ajustalo)
- DB name: `busk_seguros`
- FTP desactivado por defecto (`FTP_HOST=""`) para evitar fallos locales

Si quieres activar FTP real, cambia variables en el servicio `api` del `docker-compose.yml`.
