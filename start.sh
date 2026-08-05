#!/bin/bash
# Script para iniciar Busk Seguros API en Docker
# Uso: bash start.sh [servidor|local|help]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_help() {
    cat <<EOF
Busk Seguros - Docker Startup Script

Uso: bash start.sh [MODE]

Modos:
  servidor    Inicia API + MySQL del servidor (RECOMENDADO para producción)
  local       Inicia API + MySQL en Docker (RECOMENDADO para desarrollo)
  stop        Detiene contenedores
  logs        Ver logs en tiempo real
  status      Ver estado de contenedores
  help        Mostrar esta ayuda

Ejemplos:
  bash start.sh servidor   # Deploy en servidor con MySQL local
  bash start.sh local      # Desarrollo completo en Docker
  bash start.sh logs       # Ver logs de API
  bash start.sh stop       # Detener todo

Pre-requisitos:
  - Docker instalado
  - docker-compose instalado
  - .env configurado (copiar de .env.example)

EOF
}

check_env() {
    if [ ! -f ".env" ]; then
        echo -e "${YELLOW}⚠️  Archivo .env no encontrado${NC}"
        echo "Creando .env desde .env.example..."
        cp .env.example .env
        echo -e "${YELLOW}⚠️  Edita .env con tus valores reales${NC}"
        echo "nano .env"
        exit 1
    fi
}

check_docker() {
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}❌ Docker no está instalado${NC}"
        exit 1
    fi

    if ! command -v docker-compose &> /dev/null; then
        echo -e "${RED}❌ docker-compose no está instalado${NC}"
        exit 1
    fi
}

start_servidor() {
    echo -e "${GREEN}🚀 Iniciando Busk Seguros en SERVIDOR (MySQL local)${NC}"
    check_env
    check_docker

    echo "Limpiando contenedores previos..."
    docker-compose -f docker-compose.servidor.yml down 2>/dev/null || true

    echo "Iniciando API..."
    docker-compose -f docker-compose.servidor.yml up -d

    echo "Esperando a que API esté lista..."
    sleep 5

    echo -e "${GREEN}✅ Busk API iniciada!${NC}"
    echo ""
    echo "Verificar status:"
    echo "  docker-compose -f docker-compose.servidor.yml ps"
    echo ""
    echo "Ver logs:"
    echo "  docker-compose -f docker-compose.servidor.yml logs -f api"
    echo ""
    echo "Health check:"
    echo "  curl http://localhost:8080/api/v1/health"
    echo ""

    # Intentar health check
    if curl -s http://localhost:8080/api/v1/health > /dev/null 2>&1; then
        echo -e "${GREEN}✅ API está respondiendo correctamente${NC}"
    else
        echo -e "${YELLOW}⚠️  API aún no responde. Espera unos segundos y reintenta.${NC}"
    fi
}

start_local() {
    echo -e "${GREEN}🚀 Iniciando Busk Seguros en LOCAL (API + MySQL en Docker)${NC}"
    check_env
    check_docker

    echo "Limpiando contenedores previos..."
    docker-compose down 2>/dev/null || true

    echo "Iniciando API y MySQL..."
    docker-compose up -d

    echo "Esperando a que MySQL esté listo..."
    sleep 10

    echo -e "${GREEN}✅ Busk API iniciada!${NC}"
    echo ""
    echo "Verificar status:"
    echo "  docker-compose ps"
    echo ""
    echo "Ver logs:"
    echo "  docker-compose logs -f api"
    echo ""
    echo "Health check:"
    echo "  curl http://localhost:8080/api/v1/health"
    echo ""

    # Intentar health check
    if curl -s http://localhost:8080/api/v1/health > /dev/null 2>&1; then
        echo -e "${GREEN}✅ API está respondiendo correctamente${NC}"
    else
        echo -e "${YELLOW}⚠️  API aún no responde. Esperando a MySQL...${NC}"
        echo "Reintenta en 10 segundos: curl http://localhost:8080/api/v1/health"
    fi
}

stop_all() {
    echo -e "${YELLOW}🛑 Deteniendo contenedores...${NC}"

    docker-compose -f docker-compose.servidor.yml down 2>/dev/null || true
    docker-compose down 2>/dev/null || true

    echo -e "${GREEN}✅ Contenedores detenidos${NC}"
    echo "Nota: Datos persisten en volúmenes de Docker"
}

show_logs() {
    echo -e "${YELLOW}📋 Mostrando logs de API...${NC}"
    echo "Presiona Ctrl+C para salir"
    echo ""

    if [ -f "docker-compose.servidor.yml" ]; then
        docker-compose -f docker-compose.servidor.yml logs -f api
    else
        docker-compose logs -f api
    fi
}

show_status() {
    echo -e "${YELLOW}📊 Status de contenedores${NC}"
    echo ""

    if [ -f "docker-compose.servidor.yml" ]; then
        docker-compose -f docker-compose.servidor.yml ps
    else
        docker-compose ps
    fi

    echo ""
    echo "Health check:"
    if curl -s http://localhost:8080/api/v1/health 2>/dev/null | grep -q "ok"; then
        echo -e "${GREEN}✅ API está respondiendo${NC}"
    else
        echo -e "${RED}❌ API no responde${NC}"
    fi
}

# Main
MODE="${1:-help}"

case "$MODE" in
    servidor)
        start_servidor
        ;;
    local)
        start_local
        ;;
    stop)
        stop_all
        ;;
    logs)
        show_logs
        ;;
    status)
        show_status
        ;;
    help|"")
        print_help
        ;;
    *)
        echo -e "${RED}❌ Modo desconocido: $MODE${NC}"
        print_help
        exit 1
        ;;
esac
