package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type FileConfig struct {
	MySQLDSN                     string   `json:"mysql_dsn"`
	ProcessorWorkers             int      `json:"processor_workers"`
	ProcessorReadFullOnRowErrors *bool    `json:"processor_read_full_file_on_row_errors"`
	FilesArchiveDir              string   `json:"files_archive_dir"`
	SFTPHost                     string   `json:"sftp_host"`
	SFTPPort                     string   `json:"sftp_port"`
	SFTPUser                     string   `json:"sftp_user"`
	SFTPPassword                 string   `json:"sftp_password"`
	SFTPRemoteDir                string   `json:"sftp_remote_dir"`
	SendgridAPIKey               string   `json:"sendgrid_api_key"`
	SendgridFromEmail            string   `json:"sendgrid_from_email"`
	SendgridErrorToEmails        []string `json:"sendgrid_error_to_emails"`
}

func LoadAndApply() {
	path, ok := findConfigPath()
	if !ok {
		log.Printf("[config] sin config.json; usando variables de entorno")
		return
	}
	cfg, err := readConfig(path)
	if err != nil {
		log.Printf("[config] no se pudo leer %s: %v", path, err)
		return
	}
	apply(cfg)
	log.Printf("[config] configuración cargada desde %s", path)
}

func findConfigPath() (string, bool) {
	candidates := []string{
		strings.TrimSpace(os.Getenv("API_CONFIG_FILE")),
		"config.json",
		filepath.Join("services", "api", "config.json"),
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return c, true
		}
	}
	return "", false
}

func readConfig(path string) (FileConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return FileConfig{}, err
	}
	var cfg FileConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return FileConfig{}, fmt.Errorf("json inválido: %w", err)
	}
	return cfg, nil
}

func apply(cfg FileConfig) {
	setIfValue("MYSQL_DSN", cfg.MySQLDSN)
	setIfValue("FILES_ARCHIVE_DIR", cfg.FilesArchiveDir)
	setIfValue("SFTP_HOST", cfg.SFTPHost)
	setIfValue("SFTP_PORT", cfg.SFTPPort)
	setIfValue("SFTP_USER", cfg.SFTPUser)
	setIfValue("SFTP_PASSWORD", cfg.SFTPPassword)
	setIfValue("SFTP_REMOTE_DIR", cfg.SFTPRemoteDir)
	setIfValue("SENDGRID_API_KEY", cfg.SendgridAPIKey)
	setIfValue("SENDGRID_FROM_EMAIL", cfg.SendgridFromEmail)
	if len(cfg.SendgridErrorToEmails) > 0 {
		setIfValue("SENDGRID_ERROR_TO_EMAILS", strings.Join(cfg.SendgridErrorToEmails, ","))
	}
	if cfg.ProcessorWorkers > 0 {
		_ = os.Setenv("PROCESSOR_WORKERS", fmt.Sprintf("%d", cfg.ProcessorWorkers))
	}
	if cfg.ProcessorReadFullOnRowErrors != nil {
		if *cfg.ProcessorReadFullOnRowErrors {
			_ = os.Setenv("PROCESSOR_READ_FULL_FILE_ON_ROW_ERRORS", "true")
		} else {
			_ = os.Setenv("PROCESSOR_READ_FULL_FILE_ON_ROW_ERRORS", "false")
		}
	}
}

func setIfValue(key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	_ = os.Setenv(key, value)
}
