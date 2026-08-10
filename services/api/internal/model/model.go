package model

import "time"

type Product struct {
	ID         string       `json:"id"`
	Code       string       `json:"code"`
	Insurer    string       `json:"insurer"`
	FilePrefix string       `json:"file_prefix"`
	SheetName  string       `json:"sheet_name"`
	HeaderRow  int          `json:"header_row"`
	Mappings   []FieldMap   `json:"mappings"`
	Rules      []RuleConfig `json:"rules"`
	CreatedAt  time.Time    `json:"created_at"`
}

type ProductFormat struct {
	ID         string       `json:"id"`
	ProductID  string       `json:"product_id"`
	Name       string       `json:"name"`
	FilePrefix string       `json:"file_prefix"`
	SheetName  string       `json:"sheet_name"`
	HeaderRow  int          `json:"header_row"`
	Priority   int          `json:"priority"`
	Active     bool         `json:"active"`
	Mappings   []FieldMap   `json:"mappings"`
	Rules      []RuleConfig `json:"rules"`
	CreatedAt  time.Time    `json:"created_at"`
}

type FieldMap struct {
	CanonicalField string   `json:"canonical_field"`
	SourceHeader   string   `json:"source_header"`
	Aliases        []string `json:"aliases,omitempty"`
	Required       bool     `json:"required"`
}

type RuleConfig struct {
	Type   string             `json:"type"`
	Field  string             `json:"field"`
	Params map[string]float64 `json:"params"`
}

type FileProcessStatus string

const (
	FileStatusPending    FileProcessStatus = "PENDING"
	FileStatusQueued     FileProcessStatus = "QUEUED"
	FileStatusProcessing FileProcessStatus = "PROCESSING"
	FileStatusProcessed  FileProcessStatus = "PROCESSED"
	FileStatusSkipped    FileProcessStatus = "SKIPPED"
	FileStatusError      FileProcessStatus = "ERROR"
)

type FileProcessRecord struct {
	ID                   string            `json:"id"`
	FileName             string            `json:"file_name"`
	ProductID            string            `json:"product_id,omitempty"`
	FileHash             string            `json:"file_hash,omitempty"`
	Status               FileProcessStatus `json:"status"`
	ErrorReason          string            `json:"error_reason,omitempty"`
	EmailError           string            `json:"email_error,omitempty"`
	ValidationReportJSON string            `json:"validation_report_json,omitempty"`
	RemotePath           string            `json:"remote_path"`
	ProcessedPath        string            `json:"processed_path,omitempty"`
	ArchivePath          string            `json:"archive_path,omitempty"`
	ReportArchivePath    string            `json:"report_archive_path,omitempty"`
	ProcessedAt          time.Time         `json:"processed_at"`
	// SuppressPersist es transitorio (no se serializa ni persiste). Cuando true, el worker no
	// upsertea el registro terminal ni envía notificación: se usa para archivos cuyo hash ya
	// fue manejado en una corrida previa (dedup), donde queremos borrar la fila QUEUED y no
	// dejar rastro nuevo.
	SuppressPersist bool `json:"-"`
}

type PolicyRecord struct {
	FileID         string    `json:"file_id"`
	ProductID      string    `json:"product_id"`
	FileName       string    `json:"file_name"`
	RowNumber      int       `json:"row_number"`
	DocumentNumber string    `json:"document_number,omitempty"`
	CreditNumber   string    `json:"credit_number,omitempty"`
	PolicyStatus   string    `json:"policy_status"` // ACTIVE | FROZEN | MANUAL_REVIEW | CANCELLED
	RawDataJSON    string    `json:"raw_data_json"`
	ValidationJSON string    `json:"validation_json,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
