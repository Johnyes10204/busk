# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository layout

Multi-component monorepo for **Busk Seguros** — an insurance policy ingestion pipeline that pulls XLSX/XLS/CSV files off an SFTP, validates them, and materializes policies into MySQL.

- `services/api/` — Go 1.23 HTTP API + processing pipeline. `main.go` (~1.6k lines) wires HTTP routes + seed data + product/format catalog constants; business logic lives in `internal/`.
- `tools/sftpconnect/` — standalone Go CLI to test SFTP connectivity and download files.
- `frontend-admin/` — React 19 + Vite + TypeScript admin console (single-page app in `src/App.tsx`, ~1.4k lines).
- `docs/` — Docsify site with runbooks and functional specs (Spanish). Postman collection at `docs/postman/`.
- `busk-docs/` — separate Docsify site with the technical manual, architecture diagrams, and product catalog.
- `tools/dev/start-api-with-docs.sh` — dev entrypoint that starts API + docs + frontend together.

The code comments, logs, docs, and validation messages are in Spanish — keep that language for anything user-facing.

## Common commands

### All-in-one dev
```bash
bash tools/dev/start-api-with-docs.sh
```
Frees the ports first, then starts API (`:8080`), Docsify docs (`:3000`), and Frontend Admin (`:5173`). Override with `API_PORT`, `DOCS_PORT`, `FRONT_PORT`, `START_FRONTEND=0`, `FREE_PORTS=0`.

### API (Go)
```bash
cd services/api
go run main.go                        # start API on :8080
go build ./...                        # compile
go test ./...                         # run all tests (~113 tests, no DB required — pure unit tests)
go test ./internal/processor -run TestValidarPlanMapfre_PrimaNoCoincidePlan  # single test
go test -run TestSeedFilePrefixes_CoverageDownloads ./...                    # top-level test
```
Tests are pure functions and do not touch MySQL/SFTP.

### Frontend
```bash
cd frontend-admin
npm install
npm run dev       # Vite dev server, proxies /api → http://localhost:8080
npm run build     # tsc -b && vite build
npm run lint      # eslint
```

### SFTP scratch tool
```bash
cd tools/sftpconnect
SFTP_PASSWORD='...' go run .
```

## Configuration

The API reads `services/api/config.json` (see `config.example.json`) OR environment variables. The config file just sets env vars via `internal/config/config.go`, so both paths converge. `.env` files at repo root or `services/api/.env` are also loaded.

Env vars that matter:
- `MYSQL_DSN` — required (`root@tcp(127.0.0.1:3306)/busk?parseTime=true&multiStatements=true`)
- `SFTP_HOST` / `SFTP_PORT` / `SFTP_USER` / `SFTP_PASSWORD` / `SFTP_REMOTE_DIR`
- `PROCESSOR_WORKERS` (default 2), `PROCESSOR_READ_FULL_FILE_ON_ROW_ERRORS`
- `FILES_ARCHIVE_DIR` (default `./data/files-archive`), `REPORTS_ARCHIVE_DIR`
- `SENDGRID_API_KEY` / `SENDGRID_FROM_EMAIL` / `SENDGRID_ERROR_TO_EMAILS` — if unset, notifications are silently disabled.

The MySQL schema is created and migrated automatically at API startup by `store.runMigrations()` — there is no separate migration tool. Add a new migration by inserting an entry into the `migrations` map in `services/api/internal/store/store.go` with a sortable integer key (currently `YYYYMMDDNN`).

## Architecture

### End-to-end file flow
1. `POST /api/v1/process/scan` → `processor.ScanAndEnqueue()` lists the SFTP root, filters spreadsheets, and enqueues them onto a channel consumed by a worker pool (`PROCESSOR_WORKERS`). Scan sorts STOCK files first (`filePriority`), then INCLUSION, then rest.
2. Each worker calls `processOne` → identifies the product/format by filename via `store.FindProductFormatCandidates` (case-insensitive substring match on `file_prefix`, ordered by prefix length → priority → created_at). Multiple `product_formats` per `product` are supported.
3. The XLSX/XLS is parsed (`github.com/xuri/excelize/v2` or `github.com/extrame/xls`), each row is mapped to canonical fields per the format's `mappings_json`, and validation rules from `rules_json` + product-specific logic (BOLIVAR debt math, MAPFRE plan/premium matching, date parsing) run. See `bolivar_rules.go`, `mapfre_plan.go`, `mapfre_cancel.go`.
4. **File-level gate:** if any row has blocking issues (`policyRowHasBlockingIssues`), NO policies are persisted — the whole file goes to ERROR with only a validation report. See `policiesRowSetHasBlockingIssues` in `processor.go`.
5. On success, policies are inserted; for STOCK files, policies from prior loads missing from the current stock get auto-cancelled (`CancelMissingStockPolicies`); for MAPFRE "Anulacion masiva" files, matching stock rows get flagged cancelled (`applyMapfreCancellationsToStock`).
6. The remote file is moved to `PROCESSED/` or `ERROR/` on the SFTP via `moveRemoteFile`. A local archive copy is saved under `FILES_ARCHIVE_DIR`. A validation report JSON+XLSX is saved under `REPORTS_ARCHIVE_DIR`. SendGrid sends success/error emails.
7. Progress is tracked in-memory (`Service.progress`) and exposed via `GET /api/v1/process/progress`.

### Data model
- `products` (legacy top-level format) + `product_formats` (multiple formats per product, with `priority` and `active`). New code should treat formats as the source of truth; `products` is kept for backfill/compat.
- `processed_files` — one row per file ingestion attempt (status: `PENDING`/`QUEUED`/`PROCESSING`/`PROCESSED`/`SKIPPED`/`ERROR`). `file_hash` (SHA-256) deduplicates identical files.
- `policies` — extracted rows; `policy_status` is `ACTIVE` / `FROZEN` / `MANUAL_REVIEW` / `CANCELLED`.
- `product_allowed_premiums`, `product_rule_params`, `global_rule_params` — tunables consumed by validation rules.

### Product/format seed
`main.go` contains hardcoded prefix constants and the seed function `seed(st)` invoked by `POST /api/v1/bootstrap/sample-products`. The current insurance-product coverage:
- **MAPFRE** — Vida Voluntario (Anexo 1), AP Menores (Anexo 3), AP Cáncer (Anexo 2), Stock, Anulación Masiva.
- **BOLÍVAR** — Deudores Banco (Anexo 4, Micro/Pyme), Deudores ESAL + Stock (Anexo 5, Micro/Pyme).

File matching is by substring on filename. The April 2026 batch uses numeric contract prefixes (`5024424900103` etc.) that map to specific MAPFRE products; the legacy layout uses `INCLUSION-*-MAPFRE.xlsx`. See `TestSeedFilePrefixes_CoverageDownloads` for the authoritative match table.

### HTTP surface
Routes are defined inline in `main.go`. All under `/api/v1/`:
- `health`, `bootstrap/sample-products`
- `products`, `product-formats` (+ `/active`, `/match-test`), `products/allowed-premiums`
- `process/scan`, `process/progress`
- `files`, `files/retry`, `files/summary`, `files/validation-report`, `files/validation-csv`, `files/validation-xlsx`, `files/download`
- `policies`, `policies/search`

## Domain rules to preserve

- When plan↔prima validation fails **because of the prima**, the tag must be `REVISAR PRIMA (PLAN)`. Never emit `REVISAR PLAN` for that branch (a plain "revisar plan" tag drops the reason and is treated as a regression).
- Row-level blocking issues prevent the entire file from being persisted (see file-level gate above) — don't silently downgrade this to per-row skips.
- Filename → product matching is intentionally case-insensitive substring: `UPPER(name) LIKE '%prefix%'`. Prefixes are chosen to disambiguate across insurers (see `TestSeedFilePrefixes_NoCrossInsurer`); when adding a new prefix, add a cross-insurer collision test.
