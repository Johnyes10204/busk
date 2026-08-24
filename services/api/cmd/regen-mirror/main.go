// Regenerador puntual del XLSX espejo de novedades para un file_id ya procesado.
// Uso: go run ./cmd/regen-mirror -file-id <file_id> -out /tmp/espejo.xlsx
// Lee la BD (MYSQL_DSN) y produce el XLSX con el layout actual del código
// (columnas del archivo original + observaciones + novedades).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/buskseguros-design/services/api/internal/store"
	"github.com/joho/godotenv"
)

func main() {
	fileID := flag.String("file-id", "", "file_id existente en processed_files")
	outPath := flag.String("out", "", "ruta destino .xlsx")
	flag.Parse()

	if *fileID == "" || *outPath == "" {
		log.Fatal("uso: -file-id <id> -out <ruta.xlsx>")
	}

	_ = godotenv.Load(".env")
	_ = godotenv.Load("services/api/.env")

	st, err := store.NewMySQLFromEnv()
	if err != nil {
		log.Fatalf("conexión MySQL: %v", err)
	}
	report, err := st.GetFileValidationReport(*fileID)
	if err != nil {
		log.Fatalf("GetFileValidationReport: %v", err)
	}

	data, err := store.ValidationReportClientXLSX(report)
	if err != nil {
		log.Fatalf("ValidationReportClientXLSX: %v", err)
	}
	if err := os.WriteFile(*outPath, data, 0o644); err != nil {
		log.Fatalf("escribir salida: %v", err)
	}
	fmt.Printf("OK file_id=%s exported_rows=%d source_cols=%d bytes=%d -> %s\n",
		*fileID, len(report.ExportedRows), len(report.SourceColumns), len(data), *outPath)
}
