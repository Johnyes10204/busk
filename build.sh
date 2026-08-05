#!/bin/bash
# Script para compilar la API de Busk Seguros

set -e

echo "🔨 Compilando Busk API..."

cd services/api

# Compilar binario
go build -o busk-api main.go

echo "✅ Compilación exitosa: ./services/api/busk-api"
echo ""
echo "Próximos pasos:"
echo "  1. npm install -g pm2"
echo "  2. pm2 start ecosystem.config.js"
echo "  3. pm2 logs busk-api"
