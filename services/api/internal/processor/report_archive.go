package processor

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/buskseguros-design/services/api/internal/store"
)

func reportsArchiveBaseDir() string {
	baseDir := strings.TrimSpace(os.Getenv("REPORTS_ARCHIVE_DIR"))
	if baseDir == "" {
		baseDir = "./data/reports-archive"
	}
	return baseDir
}

func buildReportArchivePath(fileID, fileName string) (string, error) {
	baseDir := reportsArchiveBaseDir()
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve reports archive dir: %w", err)
	}
	if err := os.MkdirAll(absBase, 0o755); err != nil {
		return "", fmt.Errorf("mkdir reports archive dir: %w", err)
	}
	safeName := strings.ReplaceAll(fileName, string(filepath.Separator), "_")
	return filepath.Join(absBase, fmt.Sprintf("%s_%s.reporte.xlsx", fileID, safeName)), nil
}

func saveValidationReportArchive(report store.FileValidationReport, fileID, fileName string) string {
	path, err := buildReportArchivePath(fileID, fileName)
	if err != nil {
		log.Printf("[processor] reporte_auditoria_skip preparar_ruta file_id=%s err=%v", fileID, err)
		return ""
	}
	data, err := store.ValidationReportClientXLSX(report)
	if err != nil {
		log.Printf("[processor] reporte_auditoria_skip generar_xlsx file_id=%s err=%v", fileID, err)
		return ""
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("[processor] reporte_auditoria_skip escribir file_id=%s path=%s err=%v", fileID, path, err)
		return ""
	}
	log.Printf("[processor] reporte_auditoria_guardado file_id=%s path=%s", fileID, path)
	return path
}
