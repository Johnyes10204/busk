#!/bin/bash
# Script para iniciar API (PM2) + Caddy (Docker)

set -e

cd "$(dirname "$0")"

echo "🚀 Iniciando Busk Seguros (PM2 + Caddy)..."
echo ""

# 1. Iniciar PM2
echo "1️⃣  Iniciando API con PM2 en puerto 8080..."
pm2 start ecosystem.config.js --no-autorestart

sleep 2

# 2. Iniciar Caddy
echo "2️⃣  Iniciando Caddy en puerto 80..."
docker run -d \
  --name busk-caddy \
  --restart unless-stopped \
  -p 80:80 \
  -v "$(pwd)/Caddyfile:/etc/caddy/Caddyfile:ro" \
  caddy:2-alpine

echo ""
echo "✅ Busk Seguros iniciada!"
echo ""
echo "Estado:"
pm2 status
echo ""
echo "Caddy:"
docker ps | grep busk-caddy
echo ""
echo "Acceder:"
echo "  http://62.146.228.79/api/v1/health"
echo ""
echo "Logs PM2:"
echo "  pm2 logs busk-api -f"
echo ""
echo "Logs Caddy:"
echo "  docker logs busk-caddy -f"
