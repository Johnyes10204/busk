#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
API_DIR="$ROOT_DIR/services/api"
DOCS_DIR="$ROOT_DIR/docs"
FRONT_DIR="$ROOT_DIR/frontend-admin"
API_PORT="${API_PORT:-8080}"
DOCS_PORT="${DOCS_PORT:-3000}"
FRONT_PORT="${FRONT_PORT:-5173}"
START_FRONTEND="${START_FRONTEND:-1}"
FREE_PORTS="${FREE_PORTS:-1}"

free_port_if_needed() {
  local port="$1"
  local pids
  pids="$(lsof -ti tcp:"$port" || true)"
  if [[ -z "$pids" ]]; then
    return 0
  fi

  echo "[dev] Liberando puerto ${port} (PID(s): ${pids//$'\n'/, })"
  while IFS= read -r pid; do
    [[ -z "$pid" ]] && continue
    kill "$pid" 2>/dev/null || true
  done <<< "$pids"

  sleep 1

  pids="$(lsof -ti tcp:"$port" || true)"
  if [[ -n "$pids" ]]; then
    echo "[dev] Forzando cierre en puerto ${port} (PID(s): ${pids//$'\n'/, })"
    while IFS= read -r pid; do
      [[ -z "$pid" ]] && continue
      kill -9 "$pid" 2>/dev/null || true
    done <<< "$pids"
  fi
}

cleanup() {
  if [[ -n "${API_PID:-}" ]] && kill -0 "$API_PID" 2>/dev/null; then
    kill "$API_PID" 2>/dev/null || true
  fi
  if [[ -n "${DOCS_PID:-}" ]] && kill -0 "$DOCS_PID" 2>/dev/null; then
    kill "$DOCS_PID" 2>/dev/null || true
  fi
  if [[ -n "${FRONT_PID:-}" ]] && kill -0 "$FRONT_PID" 2>/dev/null; then
    kill "$FRONT_PID" 2>/dev/null || true
  fi
}

trap cleanup EXIT INT TERM

if [[ "$FREE_PORTS" == "1" ]]; then
  free_port_if_needed "$API_PORT"
  free_port_if_needed "$DOCS_PORT"
  if [[ "$START_FRONTEND" == "1" ]]; then
    free_port_if_needed "$FRONT_PORT"
  fi
fi

echo "[dev] Iniciando API en $API_DIR"
(
  cd "$API_DIR"
  go run main.go
) &
API_PID=$!

echo "[dev] Iniciando Docsify en http://localhost:${DOCS_PORT}"
(
  cd "$ROOT_DIR"
  npx docsify-cli serve "$DOCS_DIR" -p "$DOCS_PORT"
) &
DOCS_PID=$!

if [[ "$START_FRONTEND" == "1" ]]; then
  if [[ ! -d "$FRONT_DIR" ]]; then
    echo "[dev] frontend-admin no existe en $FRONT_DIR"
    echo "[dev] Crea el frontend o ejecuta con START_FRONTEND=0"
    exit 1
  fi

  if [[ ! -d "$FRONT_DIR/node_modules" ]]; then
    echo "[dev] Instalando dependencias del frontend..."
    (
      cd "$FRONT_DIR"
      npm install
    )
  fi

  echo "[dev] Iniciando Frontend Admin en http://localhost:${FRONT_PORT}"
  (
    cd "$FRONT_DIR"
    npm run dev -- --host --port "$FRONT_PORT"
  ) &
  FRONT_PID=$!
fi

echo "[dev] API PID=${API_PID} | DOCS PID=${DOCS_PID}${FRONT_PID:+ | FRONT PID=${FRONT_PID}}"
echo "[dev] Ctrl+C para detener los procesos"

# Espera portátil (Bash 3.2 en macOS no soporta wait -n).
PIDS=("$API_PID" "$DOCS_PID")
if [[ -n "${FRONT_PID:-}" ]]; then
  PIDS+=("$FRONT_PID")
fi

while true; do
  for pid in "${PIDS[@]}"; do
    if ! kill -0 "$pid" 2>/dev/null; then
      wait "$pid" || true
      exit 1
    fi
  done
  sleep 1
done
