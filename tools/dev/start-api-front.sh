#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
API_DIR="$ROOT_DIR/services/api"
FRONT_DIR="$ROOT_DIR/frontend-admin"
API_PORT="${API_PORT:-8080}"
FRONT_PORT="${FRONT_PORT:-5173}"
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
  if [[ -n "${FRONT_PID:-}" ]] && kill -0 "$FRONT_PID" 2>/dev/null; then
    kill "$FRONT_PID" 2>/dev/null || true
  fi
}

trap cleanup EXIT INT TERM

if [[ "$FREE_PORTS" == "1" ]]; then
  free_port_if_needed "$API_PORT"
  free_port_if_needed "$FRONT_PORT"
fi

if [[ ! -d "$FRONT_DIR" ]]; then
  echo "[dev] frontend-admin no existe en $FRONT_DIR"
  exit 1
fi

echo "[dev] Iniciando API en $API_DIR (puerto ${API_PORT})"
(
  cd "$API_DIR"
  go run main.go
) &
API_PID=$!

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

echo "[dev] API PID=${API_PID} | FRONT PID=${FRONT_PID}"
echo "[dev] Ctrl+C para detener los procesos"

# Espera portátil (Bash 3.2 en macOS no soporta wait -n).
PIDS=("$API_PID" "$FRONT_PID")
while true; do
  for pid in "${PIDS[@]}"; do
    if ! kill -0 "$pid" 2>/dev/null; then
      wait "$pid" || true
      exit 1
    fi
  done
  sleep 1
done
