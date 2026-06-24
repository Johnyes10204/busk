package store

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/buskseguros-design/services/api/internal/model"
	"github.com/buskseguros-design/services/api/internal/validationnotes"
	"github.com/go-sql-driver/mysql"
	"github.com/xuri/excelize/v2"
)

type Store struct {
	db *sql.DB
	sb sq.StatementBuilderType
}

type FileQualitySummary struct {
	FileID         string `json:"file_id"`
	FileName       string `json:"file_name"`
	ProductID      string `json:"product_id,omitempty"`
	FileStatus     string `json:"file_status"`
	TotalPolicies  int    `json:"total_policies"`
	ActiveCount    int    `json:"active_count"`
	FrozenCount    int    `json:"frozen_count"`
	ManualCount    int    `json:"manual_review_count"`
	CancelledCount int    `json:"cancelled_count"`
}

type FileDuplicateCredit struct {
	CreditNumber        string `json:"credit_number"`
	Count               int    `json:"count"`
	RowNumbers          []int  `json:"row_numbers"`
	DuplicateRowNumbers []int  `json:"duplicate_row_numbers"`
}

type FilePendingValidation struct {
	RowNumber      int      `json:"row_number"`
	DocumentNumber string   `json:"document_number,omitempty"`
	CreditNumber   string   `json:"credit_number,omitempty"`
	PolicyStatus   string   `json:"policy_status"`
	Notes          []string `json:"notes"`
}

type FileValidationReport struct {
	FileID                  string                  `json:"file_id"`
	FileName                string                  `json:"file_name"`
	ProductID               string                  `json:"product_id,omitempty"`
	FileStatus              string                  `json:"file_status"`
	ErrorReason             string                  `json:"error_reason,omitempty"`
	ProcessedAt             string                  `json:"processed_at,omitempty"`
	PolicyRowCount          int                     `json:"policy_row_count"`
	DuplicateCredits        []FileDuplicateCredit   `json:"duplicate_credits"`
	TotalDuplicateCredits   int                     `json:"total_duplicate_credits"`
	TotalDuplicateRows      int                     `json:"total_duplicate_rows"`
	PendingValidations           []FilePendingValidation `json:"pending_validations"`
	TotalPendingValidations      int                     `json:"total_pending_validations"`
	InformativeValidations       []FilePendingValidation `json:"informative_validations,omitempty"`
	TotalInformativeValidations  int                     `json:"total_informative_validations"`
	SourceColumns                []string                `json:"source_columns,omitempty"`
	ExportedRows                 []FileExportedRow       `json:"exported_rows,omitempty"`
	EmailSourceColumns           []string                `json:"email_source_columns,omitempty"`
	EmailExportedRows            []FileExportedRow       `json:"email_exported_rows,omitempty"`
}

func NewMySQLFromEnv() (*Store, error) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3306)/busk?parseTime=true&multiStatements=true"

	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}

	st := &Store{
		db: db,
		sb: sq.StatementBuilder.PlaceholderFormat(sq.Question),
	}
	if err := st.runMigrations(); err != nil {
		return nil, err
	}
	if err := st.backfillFormatsFromLegacyProducts(); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *Store) UpsertProduct(p model.Product) model.Product {
	if p.HeaderRow <= 0 {
		p.HeaderRow = 1
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	mappingsJSON, _ := json.Marshal(p.Mappings)
	rulesJSON, _ := json.Marshal(p.Rules)

	sqlStr, args, _ := s.sb.Insert("products").
		Columns("id", "code", "insurer", "file_prefix", "sheet_name", "header_row", "mappings_json", "rules_json", "created_at").
		Values(p.ID, p.Code, p.Insurer, p.FilePrefix, p.SheetName, p.HeaderRow, string(mappingsJSON), string(rulesJSON), p.CreatedAt).
		Suffix("ON DUPLICATE KEY UPDATE code=VALUES(code), insurer=VALUES(insurer), file_prefix=VALUES(file_prefix), sheet_name=VALUES(sheet_name), header_row=VALUES(header_row), mappings_json=VALUES(mappings_json), rules_json=VALUES(rules_json)").
		ToSql()
	_, _ = s.db.Exec(sqlStr, args...)
	return p
}

func (s *Store) ListProducts() []model.Product {
	sqlStr, args, _ := s.sb.Select("id", "code", "insurer", "file_prefix", "sheet_name", "header_row", "mappings_json", "rules_json", "created_at").
		From("products").
		OrderBy("created_at DESC").
		ToSql()
	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return []model.Product{}
	}
	defer rows.Close()

	out := make([]model.Product, 0)
	for rows.Next() {
		var p model.Product
		var mappingsJSON, rulesJSON string
		if err := rows.Scan(&p.ID, &p.Code, &p.Insurer, &p.FilePrefix, &p.SheetName, &p.HeaderRow, &mappingsJSON, &rulesJSON, &p.CreatedAt); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(mappingsJSON), &p.Mappings)
		_ = json.Unmarshal([]byte(rulesJSON), &p.Rules)
		out = append(out, p)
	}
	return out
}

func (s *Store) FindProductByPrefix(fileName string) (model.Product, bool) {
	sqlStr, args, _ := s.sb.Select("id", "code", "insurer", "file_prefix", "sheet_name", "header_row", "mappings_json", "rules_json", "created_at").
		From("products").
		// Match por contención (case-insensitive): si el nombre del archivo contiene el prefijo.
		Where(sq.Expr("UPPER(?) LIKE CONCAT('%', UPPER(file_prefix), '%')", strings.TrimSpace(fileName))).
		OrderBy("LENGTH(file_prefix) DESC").
		Limit(1).
		ToSql()
	row := s.db.QueryRow(sqlStr, args...)
	var p model.Product
	var mappingsJSON, rulesJSON string
	err := row.Scan(&p.ID, &p.Code, &p.Insurer, &p.FilePrefix, &p.SheetName, &p.HeaderRow, &mappingsJSON, &rulesJSON, &p.CreatedAt)
	if err != nil {
		return model.Product{}, false
	}
	_ = json.Unmarshal([]byte(mappingsJSON), &p.Mappings)
	_ = json.Unmarshal([]byte(rulesJSON), &p.Rules)
	return p, true
}

func (s *Store) UpsertProductFormat(f model.ProductFormat) model.ProductFormat {
	if f.HeaderRow <= 0 {
		f.HeaderRow = 1
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now().UTC()
	}
	mappingsJSON, _ := json.Marshal(f.Mappings)
	rulesJSON, _ := json.Marshal(f.Rules)
	active := 0
	if f.Active {
		active = 1
	}
	sqlStr, args, _ := s.sb.Insert("product_formats").
		Columns("id", "product_id", "format_name", "file_prefix", "sheet_name", "header_row", "priority", "active", "mappings_json", "rules_json", "created_at").
		Values(f.ID, f.ProductID, f.Name, f.FilePrefix, nullableString(f.SheetName), f.HeaderRow, f.Priority, active, string(mappingsJSON), string(rulesJSON), f.CreatedAt).
		Suffix("ON DUPLICATE KEY UPDATE product_id=VALUES(product_id), format_name=VALUES(format_name), file_prefix=VALUES(file_prefix), sheet_name=VALUES(sheet_name), header_row=VALUES(header_row), priority=VALUES(priority), active=VALUES(active), mappings_json=VALUES(mappings_json), rules_json=VALUES(rules_json)").
		ToSql()
	_, _ = s.db.Exec(sqlStr, args...)
	return f
}

func (s *Store) ListProductFormats(productID string) []model.ProductFormat {
	qb := s.sb.Select("id", "product_id", "format_name", "file_prefix", "sheet_name", "header_row", "priority", "active", "mappings_json", "rules_json", "created_at").
		From("product_formats")
	if strings.TrimSpace(productID) != "" {
		qb = qb.Where(sq.Eq{"product_id": strings.TrimSpace(productID)})
	}
	sqlStr, args, _ := qb.OrderBy("product_id ASC", "priority DESC", "created_at DESC").ToSql()
	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return []model.ProductFormat{}
	}
	defer rows.Close()

	out := make([]model.ProductFormat, 0)
	for rows.Next() {
		var f model.ProductFormat
		var mappingsJSON, rulesJSON string
		var active int
		var sheetName sql.NullString
		if err := rows.Scan(&f.ID, &f.ProductID, &f.Name, &f.FilePrefix, &sheetName, &f.HeaderRow, &f.Priority, &active, &mappingsJSON, &rulesJSON, &f.CreatedAt); err != nil {
			continue
		}
		if sheetName.Valid {
			f.SheetName = sheetName.String
		}
		f.Active = active == 1
		_ = json.Unmarshal([]byte(mappingsJSON), &f.Mappings)
		_ = json.Unmarshal([]byte(rulesJSON), &f.Rules)
		out = append(out, f)
	}
	return out
}

func (s *Store) SetProductFormatActive(formatID string, active bool) error {
	formatID = strings.TrimSpace(formatID)
	if formatID == "" {
		return nil
	}
	activeInt := 0
	if active {
		activeInt = 1
	}
	_, err := s.db.Exec(`UPDATE product_formats SET active = ? WHERE id = ?`, activeInt, formatID)
	return err
}

func (s *Store) FindProductFormatCandidates(fileName string) []model.Product {
	name := strings.TrimSpace(fileName)
	if name == "" {
		return []model.Product{}
	}

	sqlFormats, argsFormats, _ := s.sb.
		Select(
			"p.id", "p.code", "p.insurer", "f.file_prefix", "f.sheet_name", "f.header_row",
			"f.mappings_json", "f.rules_json", "p.created_at",
		).
		From("product_formats f").
		Join("products p ON p.id = f.product_id").
		Where(sq.Expr("f.active = 1")).
		Where(sq.Expr("UPPER(?) LIKE CONCAT('%', UPPER(f.file_prefix), '%')", name)).
		OrderBy("LENGTH(f.file_prefix) DESC", "f.priority DESC", "f.created_at DESC").
		ToSql()

	rows, err := s.db.Query(sqlFormats, argsFormats...)
	if err == nil {
		defer rows.Close()
		formats := make([]model.Product, 0)
		for rows.Next() {
			var p model.Product
			var mappingsJSON, rulesJSON string
			var sheetName sql.NullString
			if err := rows.Scan(&p.ID, &p.Code, &p.Insurer, &p.FilePrefix, &sheetName, &p.HeaderRow, &mappingsJSON, &rulesJSON, &p.CreatedAt); err != nil {
				continue
			}
			if sheetName.Valid {
				p.SheetName = sheetName.String
			}
			_ = json.Unmarshal([]byte(mappingsJSON), &p.Mappings)
			_ = json.Unmarshal([]byte(rulesJSON), &p.Rules)
			formats = append(formats, p)
		}
		if len(formats) > 0 {
			return formats
		}
	}

	return []model.Product{}
}

func (s *Store) AddFileRecord(r model.FileProcessRecord) {
	sqlStr, args, _ := s.sb.Insert("processed_files").
		Columns("id", "file_name", "product_id", "file_hash", "status", "error_reason", "email_error", "validation_report_json", "remote_path", "processed_path", "archive_path", "processed_at").
		Values(r.ID, r.FileName, nullableString(r.ProductID), nullableString(r.FileHash), string(r.Status), nullableString(r.ErrorReason), nullableString(r.EmailError), nullableString(r.ValidationReportJSON), r.RemotePath, nullableString(r.ProcessedPath), nullableString(r.ArchivePath), r.ProcessedAt).
		Suffix("ON DUPLICATE KEY UPDATE product_id=VALUES(product_id), file_hash=VALUES(file_hash), status=VALUES(status), error_reason=VALUES(error_reason), email_error=VALUES(email_error), validation_report_json=VALUES(validation_report_json), remote_path=VALUES(remote_path), processed_path=VALUES(processed_path), archive_path=VALUES(archive_path), processed_at=VALUES(processed_at)").
		ToSql()
	_, _ = s.db.Exec(sqlStr, args...)
}

// SetFileEmailError persiste el fallo de notificación por correo del archivo.
func (s *Store) SetFileEmailError(fileID, emailError string) error {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return fmt.Errorf("file_id es obligatorio")
	}
	_, err := s.db.Exec(`UPDATE processed_files SET email_error = ? WHERE id = ?`, nullableString(emailError), fileID)
	return err
}

// FileHashAlreadyProcessed indica si ya se trató un archivo con el mismo contenido (SHA-256
// de todos los bytes del fichero) en estado PROCESSED o SKIPPED.
// Nota: la deduplicación es global por contenido (no por producto).
func (s *Store) FileHashAlreadyProcessed(productID, fileHash string) bool {
	if strings.TrimSpace(fileHash) == "" {
		return false
	}
	qb := s.sb.Select("1").
		From("processed_files").
		Where(sq.Eq{"file_hash": fileHash}).
		Where(sq.Eq{"status": []string{
			string(model.FileStatusProcessed),
			string(model.FileStatusSkipped),
		}})
	sqlStr, args, _ := qb.Limit(1).ToSql()
	var one int
	err := s.db.QueryRow(sqlStr, args...).Scan(&one)
	return err == nil && one == 1
}

func (s *Store) InsertPolicies(policies []model.PolicyRecord) error {
	if len(policies) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	q := `INSERT INTO policies (
		file_id, product_id, file_name, ` + "`row_number`" + `, document_number, credit_number, policy_status, raw_data_json, validation_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	for _, p := range policies {
		if _, err = tx.Exec(
			q,
			p.FileID,
			p.ProductID,
			p.FileName,
			p.RowNumber,
			nullableString(p.DocumentNumber),
			nullableString(p.CreditNumber),
			p.PolicyStatus,
			p.RawDataJSON,
			nullableString(p.ValidationJSON),
			p.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert policy row=%d: %w", p.RowNumber, err)
		}
	}
	return tx.Commit()
}

// CancelMissingStockPolicies marca como CANCELLED pólizas históricas del producto que no
// aparezcan en el stock actual (match por credit_number). Excluye el file_id actual.
func (s *Store) CancelMissingStockPolicies(productID, currentFileID string, currentCredits []string) (int64, error) {
	if strings.TrimSpace(productID) == "" || strings.TrimSpace(currentFileID) == "" {
		return 0, nil
	}

	unique := make(map[string]struct{}, len(currentCredits))
	for _, c := range currentCredits {
		credit := strings.TrimSpace(c)
		if credit == "" {
			continue
		}
		unique[credit] = struct{}{}
	}
	credits := make([]string, 0, len(unique))
	for c := range unique {
		credits = append(credits, c)
	}

	// Si el stock llega vacío (sin créditos válidos), se cancelan todas las históricas del producto
	// excepto las del file actual.
	base := `UPDATE policies
		SET policy_status = 'CANCELLED'
		WHERE product_id = ?
		  AND file_id <> ?
		  AND policy_status <> 'CANCELLED'
		  AND credit_number IS NOT NULL
		  AND TRIM(credit_number) <> ''`
	args := []any{productID, currentFileID}

	if len(credits) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(credits)), ",")
		base += " AND credit_number NOT IN (" + placeholders + ")"
		for _, c := range credits {
			args = append(args, c)
		}
	}

	res, err := s.db.Exec(base, args...)
	if err != nil {
		return 0, err
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

func (s *Store) UpsertAllowedPremiums(productID string, premiums []float64) error {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`DELETE FROM product_allowed_premiums WHERE product_id = ?`, productID); err != nil {
		return err
	}
	seen := map[float64]struct{}{}
	for _, p := range premiums {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		if _, err = tx.Exec(
			`INSERT INTO product_allowed_premiums (product_id, premium_value, active, created_at) VALUES (?, ?, 1, ?)`,
			productID, p, time.Now().UTC(),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetAllowedPremiums(productID string) []float64 {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return []float64{}
	}
	rows, err := s.db.Query(
		`SELECT premium_value FROM product_allowed_premiums WHERE product_id = ? AND active = 1 ORDER BY premium_value`,
		productID,
	)
	if err != nil {
		return []float64{}
	}
	defer rows.Close()
	out := make([]float64, 0)
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err != nil {
			continue
		}
		out = append(out, v)
	}
	return out
}

func (s *Store) AddAllowedPremium(productID string, premium float64) error {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT INTO product_allowed_premiums (product_id, premium_value, active, created_at)
		 VALUES (?, ?, 1, ?)
		 ON DUPLICATE KEY UPDATE active = 1`,
		productID, premium, time.Now().UTC(),
	)
	return err
}

func (s *Store) DeleteAllowedPremium(productID string, premium float64) error {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM product_allowed_premiums WHERE product_id = ? AND premium_value = ?`, productID, premium)
	return err
}

func (s *Store) ClearAllowedPremiums(productID string) error {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM product_allowed_premiums WHERE product_id = ?`, productID)
	return err
}

func (s *Store) UpsertProductRuleParam(productID, paramKey, paramValue string) error {
	productID = strings.TrimSpace(productID)
	paramKey = strings.TrimSpace(strings.ToLower(paramKey))
	if productID == "" || paramKey == "" {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT INTO product_rule_params (product_id, param_key, param_value, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE param_value=VALUES(param_value), updated_at=VALUES(updated_at)`,
		productID, paramKey, paramValue, time.Now().UTC(),
	)
	return err
}

func (s *Store) GetProductRuleParam(productID, paramKey string) (string, bool) {
	productID = strings.TrimSpace(productID)
	paramKey = strings.TrimSpace(strings.ToLower(paramKey))
	if productID == "" || paramKey == "" {
		return "", false
	}
	var v string
	err := s.db.QueryRow(
		`SELECT param_value FROM product_rule_params WHERE product_id = ? AND param_key = ? LIMIT 1`,
		productID, paramKey,
	).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}

func (s *Store) UpsertGlobalRuleParam(paramKey, paramValue string) error {
	paramKey = strings.TrimSpace(strings.ToLower(paramKey))
	if paramKey == "" {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT INTO global_rule_params (param_key, param_value, updated_at)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE param_value=VALUES(param_value), updated_at=VALUES(updated_at)`,
		paramKey, paramValue, time.Now().UTC(),
	)
	return err
}

func (s *Store) GetGlobalRuleParam(paramKey string) (string, bool) {
	paramKey = strings.TrimSpace(strings.ToLower(paramKey))
	if paramKey == "" {
		return "", false
	}
	var v string
	err := s.db.QueryRow(`SELECT param_value FROM global_rule_params WHERE param_key = ? LIMIT 1`, paramKey).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}

func (s *Store) PolicyCreditExists(productID, creditNumber string) bool {
	if strings.TrimSpace(productID) == "" || strings.TrimSpace(creditNumber) == "" {
		return false
	}
	sqlStr, args, _ := s.sb.Select("1").
		From("policies").
		Where(sq.Eq{"product_id": productID, "credit_number": creditNumber}).
		Limit(1).ToSql()
	var one int
	err := s.db.QueryRow(sqlStr, args...).Scan(&one)
	return err == nil && one == 1
}

func (s *Store) ListFileRecords() []model.FileProcessRecord {
	sqlStr, args, _ := s.sb.Select("id", "file_name", "product_id", "file_hash", "status", "error_reason", "email_error", "remote_path", "processed_path", "archive_path", "processed_at").
		From("processed_files").
		OrderBy("processed_at DESC").
		ToSql()
	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return []model.FileProcessRecord{}
	}
	defer rows.Close()

	out := make([]model.FileProcessRecord, 0)
	for rows.Next() {
		var r model.FileProcessRecord
		var productID, fileHash, errorReason, emailError, processedPath, archivePath sql.NullString
		var status string
		if err := rows.Scan(&r.ID, &r.FileName, &productID, &fileHash, &status, &errorReason, &emailError, &r.RemotePath, &processedPath, &archivePath, &r.ProcessedAt); err != nil {
			continue
		}
		if productID.Valid {
			r.ProductID = productID.String
		}
		if fileHash.Valid {
			r.FileHash = fileHash.String
		}
		if errorReason.Valid {
			r.ErrorReason = errorReason.String
		}
		if emailError.Valid {
			r.EmailError = emailError.String
		}
		if processedPath.Valid {
			r.ProcessedPath = processedPath.String
		}
		if archivePath.Valid {
			r.ArchivePath = archivePath.String
		}
		r.Status = model.FileProcessStatus(status)
		out = append(out, r)
	}
	return out
}

func (s *Store) GetFileRecordByID(fileID string) (model.FileProcessRecord, bool) {
	sqlStr, args, _ := s.sb.Select("id", "file_name", "product_id", "file_hash", "status", "error_reason", "email_error", "remote_path", "processed_path", "archive_path", "processed_at").
		From("processed_files").
		Where(sq.Eq{"id": fileID}).
		Limit(1).
		ToSql()
	var r model.FileProcessRecord
	var productID, fileHash, errorReason, emailError, processedPath, archivePath sql.NullString
	var status string
	err := s.db.QueryRow(sqlStr, args...).Scan(&r.ID, &r.FileName, &productID, &fileHash, &status, &errorReason, &emailError, &r.RemotePath, &processedPath, &archivePath, &r.ProcessedAt)
	if err != nil {
		return model.FileProcessRecord{}, false
	}
	if productID.Valid {
		r.ProductID = productID.String
	}
	if fileHash.Valid {
		r.FileHash = fileHash.String
	}
	if errorReason.Valid {
		r.ErrorReason = errorReason.String
	}
	if emailError.Valid {
		r.EmailError = emailError.String
	}
	if processedPath.Valid {
		r.ProcessedPath = processedPath.String
	}
	if archivePath.Valid {
		r.ArchivePath = archivePath.String
	}
	r.Status = model.FileProcessStatus(status)
	return r, true
}

func (s *Store) ListPoliciesByProduct(productID, status string, limit int) []model.PolicyRecord {
	qb := s.sb.Select(
		"file_id", "product_id", "file_name", "`row_number`", "document_number", "credit_number", "policy_status", "raw_data_json", "validation_json", "created_at",
	).From("policies").Where(sq.Eq{"product_id": productID}).OrderBy("created_at DESC")

	if strings.TrimSpace(status) != "" {
		qb = qb.Where(sq.Eq{"policy_status": status})
	}
	if limit > 0 {
		qb = qb.Limit(uint64(limit))
	}

	sqlStr, args, _ := qb.ToSql()
	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return []model.PolicyRecord{}
	}
	defer rows.Close()

	out := make([]model.PolicyRecord, 0)
	for rows.Next() {
		var p model.PolicyRecord
		var documentNumber, creditNumber, validationJSON sql.NullString
		if err := rows.Scan(
			&p.FileID, &p.ProductID, &p.FileName, &p.RowNumber, &documentNumber, &creditNumber, &p.PolicyStatus, &p.RawDataJSON, &validationJSON, &p.CreatedAt,
		); err != nil {
			continue
		}
		if documentNumber.Valid {
			p.DocumentNumber = documentNumber.String
		}
		if creditNumber.Valid {
			p.CreditNumber = creditNumber.String
		}
		if validationJSON.Valid {
			p.ValidationJSON = validationJSON.String
		}
		out = append(out, p)
	}
	return out
}

func applyPolicySearchFilters(qb sq.SelectBuilder, productID, creditNumber, documentNumber string) sq.SelectBuilder {
	if strings.TrimSpace(productID) != "" {
		qb = qb.Where(sq.Eq{"product_id": productID})
	}
	if strings.TrimSpace(creditNumber) != "" {
		qb = qb.Where(sq.Eq{"credit_number": creditNumber})
	}
	if strings.TrimSpace(documentNumber) != "" {
		qb = qb.Where(sq.Eq{"document_number": documentNumber})
	}
	return qb
}

// SearchPoliciesPage busca pólizas por crédito y/o documento (productID vacío = todos los productos).
// page es 1-based; devuelve el total de filas que cumplen el filtro para poder paginar hasta el final.
func (s *Store) SearchPoliciesPage(productID, creditNumber, documentNumber string, page, pageSize int) ([]model.PolicyRecord, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	countQB := applyPolicySearchFilters(
		s.sb.Select("COUNT(*)").From("policies"),
		productID, creditNumber, documentNumber,
	)
	countSQL, countArgs, _ := countQB.ToSql()
	var total int
	if err := s.db.QueryRow(countSQL, countArgs...).Scan(&total); err != nil {
		return []model.PolicyRecord{}, 0
	}

	qb := applyPolicySearchFilters(
		s.sb.Select(
			"file_id", "product_id", "file_name", "`row_number`", "document_number", "credit_number", "policy_status", "raw_data_json", "validation_json", "created_at",
		).From("policies"),
		productID, creditNumber, documentNumber,
	)
	qb = qb.OrderBy("created_at DESC").Limit(uint64(pageSize)).Offset(uint64(offset))

	sqlStr, args, _ := qb.ToSql()
	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return []model.PolicyRecord{}, total
	}
	defer rows.Close()

	out := make([]model.PolicyRecord, 0)
	for rows.Next() {
		var p model.PolicyRecord
		var documentNum, creditNum, validationJSON sql.NullString
		if err := rows.Scan(
			&p.FileID, &p.ProductID, &p.FileName, &p.RowNumber, &documentNum, &creditNum, &p.PolicyStatus, &p.RawDataJSON, &validationJSON, &p.CreatedAt,
		); err != nil {
			continue
		}
		if documentNum.Valid {
			p.DocumentNumber = documentNum.String
		}
		if creditNum.Valid {
			p.CreditNumber = creditNum.String
		}
		if validationJSON.Valid {
			p.ValidationJSON = validationJSON.String
		}
		out = append(out, p)
	}
	return out, total
}

func (s *Store) GetFileQualitySummary(fileID string) (FileQualitySummary, error) {
	var out FileQualitySummary
	out.FileID = fileID

	qFile := `SELECT id, file_name, product_id, status FROM processed_files WHERE id = ? LIMIT 1`
	var productID sql.NullString
	if err := s.db.QueryRow(qFile, fileID).Scan(&out.FileID, &out.FileName, &productID, &out.FileStatus); err != nil {
		return out, err
	}
	if productID.Valid {
		out.ProductID = productID.String
	}

	qCounts := `SELECT 
		COUNT(*) as total,
		SUM(CASE WHEN policy_status='ACTIVE' THEN 1 ELSE 0 END) as active_count,
		SUM(CASE WHEN policy_status='FROZEN' THEN 1 ELSE 0 END) as frozen_count,
		SUM(CASE WHEN policy_status='MANUAL_REVIEW' THEN 1 ELSE 0 END) as manual_count,
		SUM(CASE WHEN policy_status='CANCELLED' THEN 1 ELSE 0 END) as cancelled_count
		FROM policies WHERE file_id = ?`
	var total, active, frozen, manual, cancelled sql.NullInt64
	if err := s.db.QueryRow(qCounts, fileID).Scan(&total, &active, &frozen, &manual, &cancelled); err != nil {
		return out, err
	}
	if total.Valid {
		out.TotalPolicies = int(total.Int64)
	}
	if active.Valid {
		out.ActiveCount = int(active.Int64)
	}
	if frozen.Valid {
		out.FrozenCount = int(frozen.Int64)
	}
	if manual.Valid {
		out.ManualCount = int(manual.Int64)
	}
	if cancelled.Valid {
		out.CancelledCount = int(cancelled.Int64)
	}
	return out, nil
}

// policyRowInput es una fila (desde policies o desde memoria) para armar FileValidationReport.
type policyRowInput struct {
	RowNumber      int
	DocumentNumber string
	CreditNumber   string
	PolicyStatus   string
	ValidationJSON string
	RawDataJSON    string
}

func completeFileValidationReport(
	fileID, fileName, productID, fileStatus, errorReason, processedAtRFC string,
	inputs []policyRowInput,
) FileValidationReport {
	out := FileValidationReport{
		FileID:      fileID,
		FileName:    fileName,
		ProductID:   productID,
		FileStatus:  fileStatus,
		ErrorReason: errorReason,
		ProcessedAt: processedAtRFC,
	}
	creditCounts := make(map[string]int)
	creditRows := make(map[string][]int)
	pending := make([]FilePendingValidation, 0)
	informative := make([]FilePendingValidation, 0)

	for _, in := range inputs {
		out.PolicyRowCount++
		doc := strings.TrimSpace(in.DocumentNumber)
		cred := strings.TrimSpace(in.CreditNumber)
		if cred != "" {
			creditCounts[cred]++
			creditRows[cred] = append(creditRows[cred], in.RowNumber)
		}
		notes := make([]string, 0)
		if strings.TrimSpace(in.ValidationJSON) != "" {
			_ = json.Unmarshal([]byte(in.ValidationJSON), &notes)
		}
		st := strings.TrimSpace(in.PolicyStatus)
		if strings.EqualFold(st, "CANCELLED") {
			continue
		}
		blocking, info := validationnotes.Split(notes)
		if strings.EqualFold(st, "FROZEN") && len(info) == 0 && len(blocking) == 0 {
			info = []string{validationnotes.Informativo("PRIMA CERO: PÓLIZA CONGELADA")}
		}
		if len(info) > 0 {
			informative = append(informative, FilePendingValidation{
				RowNumber:      in.RowNumber,
				DocumentNumber: doc,
				CreditNumber:   cred,
				PolicyStatus:   st,
				Notes:          info,
			})
		}
		if len(blocking) > 0 || strings.EqualFold(st, "MANUAL_REVIEW") {
			rowNotes := blocking
			if len(rowNotes) == 0 && strings.EqualFold(st, "MANUAL_REVIEW") {
				rowNotes = nil
			}
			pending = append(pending, FilePendingValidation{
				RowNumber:      in.RowNumber,
				DocumentNumber: doc,
				CreditNumber:   cred,
				PolicyStatus:   st,
				Notes:          rowNotes,
			})
		}
	}

	duplicates := make([]FileDuplicateCredit, 0)
	totalDuplicateRows := 0
	for credit, count := range creditCounts {
		if count > 1 {
			rowNums := creditRows[credit]
			dupRows := make([]int, 0)
			if len(rowNums) > 1 {
				dupRows = append(dupRows, rowNums[1:]...)
			}
			duplicates = append(duplicates, FileDuplicateCredit{
				CreditNumber:        credit,
				Count:               count,
				RowNumbers:          rowNums,
				DuplicateRowNumbers: dupRows,
			})
			totalDuplicateRows += count - 1
		}
	}
	sort.Slice(duplicates, func(i, j int) bool {
		if duplicates[i].Count == duplicates[j].Count {
			return duplicates[i].CreditNumber < duplicates[j].CreditNumber
		}
		return duplicates[i].Count > duplicates[j].Count
	})

	out.DuplicateCredits = duplicates
	out.TotalDuplicateCredits = len(duplicates)
	out.TotalDuplicateRows = totalDuplicateRows
	out.PendingValidations = pending
	out.TotalPendingValidations = len(pending)
	out.InformativeValidations = informative
	out.TotalInformativeValidations = len(informative)
	out.SourceColumns, out.ExportedRows = buildFileExportedRows(inputs)
	out.EmailSourceColumns, out.EmailExportedRows = buildFileExportedRowsEmail(inputs)
	return out
}

// validationReportClientRows devuelve [encabezado, ...filas] para CSV y Excel:
// una fila por fila del archivo con incidencias; todas las novedades en detalle_novedad (listado numerado).
func validationReportClientRows(r FileValidationReport) [][]string {
	header := []string{
		"tipo_registro",
		"nombre_archivo",
		"file_id",
		"producto_id",
		"fila_excel",
		"identificacion_afiliado",
		"numero_credito",
		"estado_poliza",
		"cantidad_novedades",
		"novedades_json",
		"observaciones",
		"detalle_novedad",
	}
	out := [][]string{header}
	fileName := strings.TrimSpace(r.FileName)
	fileID := strings.TrimSpace(r.FileID)
	productID := strings.TrimSpace(r.ProductID)

	dupSummaryByCredit := make(map[string]string)
	for _, dup := range r.DuplicateCredits {
		cred := strings.TrimSpace(dup.CreditNumber)
		if cred == "" {
			continue
		}
		dupSummaryByCredit[cred] = fmt.Sprintf(
			"Resumen: el crédito u operación «%s» aparece %d veces en el archivo. Filas: %s. Filas repetidas (después de la primera aparición): %s.",
			cred, dup.Count, formatIntListForCSV(dup.RowNumbers), formatIntListForCSV(dup.DuplicateRowNumbers),
		)
	}

	byRow := make(map[int]FilePendingValidation)
	for _, pv := range r.PendingValidations {
		ex, ok := byRow[pv.RowNumber]
		if !ok {
			byRow[pv.RowNumber] = pv
			continue
		}
		ex.Notes = append(append([]string{}, ex.Notes...), pv.Notes...)
		if strings.TrimSpace(ex.DocumentNumber) == "" && strings.TrimSpace(pv.DocumentNumber) != "" {
			ex.DocumentNumber = pv.DocumentNumber
		}
		if strings.TrimSpace(ex.CreditNumber) == "" && strings.TrimSpace(pv.CreditNumber) != "" {
			ex.CreditNumber = pv.CreditNumber
		}
		if strings.EqualFold(strings.TrimSpace(pv.PolicyStatus), "MANUAL_REVIEW") {
			ex.PolicyStatus = "MANUAL_REVIEW"
		}
		byRow[pv.RowNumber] = ex
	}
	rowNums := make([]int, 0, len(byRow))
	for rn := range byRow {
		rowNums = append(rowNums, rn)
	}
	sort.Ints(rowNums)

	nData := 0
	for _, rn := range rowNums {
		pv := byRow[rn]
		rawCount := len(pv.Notes)
		notes := trimNotesPreserveAll(pv.Notes)
		cred := strings.TrimSpace(pv.CreditNumber)
		if cred != "" {
			if sum, ok := dupSummaryByCredit[cred]; ok && !noteSliceContainsExact(notes, sum) {
				notes = append(notes, sum)
			}
		}
		detail := formatNovedadesColumn(notes)
		if strings.TrimSpace(detail) == "" {
			detail = defaultPendingRowDetailMessage(pv.PolicyStatus, rawCount > 0)
		}
		jsonNotes, _ := json.Marshal(notes)
		out = append(out, []string{
			"Incidencia en fila",
			fileName,
			fileID,
			productID,
			strconv.Itoa(pv.RowNumber),
			strings.TrimSpace(pv.DocumentNumber),
			strings.TrimSpace(pv.CreditNumber),
			etiquetaEstadoPolizaInforme(pv.PolicyStatus),
			strconv.Itoa(len(notes)),
			string(jsonNotes),
			observacionesForRow(notes, pv.PolicyStatus),
			detail,
		})
		nData++
	}

	if nData == 0 && len(r.DuplicateCredits) > 0 {
		for _, dup := range r.DuplicateCredits {
			cred := strings.TrimSpace(dup.CreditNumber)
			if cred == "" {
				continue
			}
			detail := fmt.Sprintf(
				"El crédito u operación «%s» aparece %d veces en el archivo y no hay otras incidencias registradas por fila. Filas: %s. Filas repetidas (después de la primera aparición): %s.",
				cred, dup.Count, formatIntListForCSV(dup.RowNumbers), formatIntListForCSV(dup.DuplicateRowNumbers),
			)
			notes := []string{detail}
			jsonNotes, _ := json.Marshal(notes)
			out = append(out, []string{
				"Crédito duplicado en archivo",
				fileName,
				fileID,
				productID,
				"",
				"",
				cred,
				"",
				"1",
				string(jsonNotes),
				observacionesForRow(notes, ""),
				detail,
			})
			nData++
		}
	}

	if nData == 0 {
		er := strings.TrimSpace(r.ErrorReason)
		if er != "" {
			notes := []string{er}
			jsonNotes, _ := json.Marshal(notes)
			out = append(out, []string{
				"Resumen del archivo",
				fileName,
				fileID,
				productID,
				"",
				"",
				"",
				etiquetaEstadoPolizaInforme(r.FileStatus),
				"1",
				string(jsonNotes),
				observacionesForRow(notes, r.FileStatus),
				er,
			})
		}
	}
	return out
}

// validationReportInformativeRows: misma estructura que incidencias, para la hoja «Informes» del Excel (no bloquean carga).
func validationReportInformativeRows(r FileValidationReport) [][]string {
	header := []string{
		"tipo_registro",
		"nombre_archivo",
		"file_id",
		"producto_id",
		"fila_excel",
		"identificacion_afiliado",
		"numero_credito",
		"estado_poliza",
		"cantidad_avisos",
		"avisos_json",
		"observaciones",
		"detalle_aviso",
	}
	out := [][]string{header}
	fileName := strings.TrimSpace(r.FileName)
	fileID := strings.TrimSpace(r.FileID)
	productID := strings.TrimSpace(r.ProductID)

	byRow := make(map[int]FilePendingValidation)
	for _, pv := range r.InformativeValidations {
		ex, ok := byRow[pv.RowNumber]
		if !ok {
			byRow[pv.RowNumber] = pv
			continue
		}
		ex.Notes = append(append([]string{}, ex.Notes...), pv.Notes...)
		byRow[pv.RowNumber] = ex
	}
	rowNums := make([]int, 0, len(byRow))
	for rn := range byRow {
		rowNums = append(rowNums, rn)
	}
	sort.Ints(rowNums)

	for _, rn := range rowNums {
		pv := byRow[rn]
		notes := trimNotesPreserveAll(pv.Notes)
		detail := formatNovedadesColumn(notes)
		jsonNotes, _ := json.Marshal(notes)
		out = append(out, []string{
			"Informe informativo",
			fileName,
			fileID,
			productID,
			strconv.Itoa(pv.RowNumber),
			strings.TrimSpace(pv.DocumentNumber),
			strings.TrimSpace(pv.CreditNumber),
			etiquetaEstadoPolizaInforme(pv.PolicyStatus),
			strconv.Itoa(len(notes)),
			string(jsonNotes),
			observacionesForRow(notes, pv.PolicyStatus),
			detail,
		})
	}
	return out
}

func trimNotesPreserveAll(notes []string) []string {
	out := make([]string, 0, len(notes))
	for _, n := range notes {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		out = append(out, n)
	}
	return out
}

func noteSliceContainsExact(notes []string, target string) bool {
	t := strings.TrimSpace(target)
	if t == "" {
		return false
	}
	for _, n := range notes {
		if strings.TrimSpace(n) == t {
			return true
		}
	}
	return false
}

func formatNovedadesColumn(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	var b strings.Builder
	for i, n := range notes {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(". ")
		b.WriteString(validationnotes.DisplayText(n))
	}
	return b.String()
}

func defaultPendingRowDetailMessage(policyStatus string, hadRawNotes bool) string {
	_ = hadRawNotes
	if strings.EqualFold(strings.TrimSpace(policyStatus), "FROZEN") {
		return "PRIMA CERO: PÓLIZA CONGELADA"
	}
	if strings.EqualFold(strings.TrimSpace(policyStatus), "MANUAL_REVIEW") {
		return "REVISIÓN MANUAL"
	}
	return "INCIDENCIA SIN DETALLE"
}

func etiquetaEstadoPolizaInforme(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ACTIVE":
		return "Activa"
	case "FROZEN":
		return "Congelada"
	case "MANUAL_REVIEW":
		return "Revisión manual"
	case "CANCELLED":
		return "Cancelada"
	default:
		return strings.TrimSpace(status)
	}
}

// ValidationReportClientCSV genera un CSV UTF-8 (con BOM para Excel) legible para el cliente:
// una fila por fila con incidencias; detalle_novedad con todas las novedades numeradas.
func ValidationReportClientCSV(r FileValidationReport) ([]byte, error) {
	rows := validationReportClientRows(r)
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// validationReportXLSX escribe hojas Incidencias e Informes; includeMirror añade «Datos archivo».
func validationReportXLSX(r FileValidationReport, includeMirror bool) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheetIncidencias = "Incidencias"
	const sheetInformes = "Informes"
	if err := f.SetSheetName("Sheet1", sheetIncidencias); err != nil {
		return nil, err
	}
	if _, err := f.NewSheet(sheetInformes); err != nil {
		return nil, err
	}
	if err := writeValidationReportSheet(f, sheetIncidencias, validationReportClientRows(r)); err != nil {
		return nil, err
	}
	if err := writeValidationReportSheet(f, sheetInformes, validationReportInformativeRows(r)); err != nil {
		return nil, err
	}
	if includeMirror {
		const sheetDatos = "Datos archivo"
		if _, err := f.NewSheet(sheetDatos); err != nil {
			return nil, err
		}
		if err := writeValidationReportMirrorSheet(f, sheetDatos, validationReportMirrorRows(r)); err != nil {
			return nil, err
		}
	}
	idxInc, _ := f.GetSheetIndex(sheetIncidencias)
	f.SetActiveSheet(idxInc)
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ValidationReportClientXLSX genera un libro Excel (.xlsx):
// hoja «Incidencias» = bloquean o revisión manual (mismo contenido que el CSV);
// hoja «Informes» = avisos informativos (p. ej. vencimiento anterior al mes de facturación) sin frenar la carga;
// hoja «Datos archivo» = espejo del origen con novedades.
func ValidationReportClientXLSX(r FileValidationReport) ([]byte, error) {
	return validationReportXLSX(r, true)
}

// ValidationReportEmailXLSX es el adjunto de correo: una única hoja que replica la estructura del archivo
// con las filas que tuvieron incidencias (bloqueantes, revisión manual o avisos informativos)
// y una última columna "novedades" con la observación.
func ValidationReportEmailXLSX(r FileValidationReport) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "Reporte"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}
	if err := writeValidationReportMirrorSheet(f, sheet, validationReportEmailMirrorRows(r)); err != nil {
		return nil, err
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ValidationReportXLSXForErrorEmail genera un adjunto de una hoja cuando no hay informe JSON (error temprano).
// Sin filas del archivo original, sólo registra el motivo del error en la columna novedades.
func ValidationReportXLSXForErrorEmail(fileName, fileID, productID, errorReason string) ([]byte, error) {
	summary := strings.TrimSpace(errorReason)
	if summary == "" {
		summary = "Error en procesamiento del archivo"
	}
	r := FileValidationReport{
		FileName:  strings.TrimSpace(fileName),
		FileID:    strings.TrimSpace(fileID),
		ProductID: strings.TrimSpace(productID),
		EmailExportedRows: []FileExportedRow{{
			RowNumber:    0,
			PolicyStatus: "ERROR",
			Novedades:    formatNovedadesColumn([]string{validationnotes.Incidencia(summary)}),
		}},
	}
	return ValidationReportEmailXLSX(r)
}

func writeValidationReportSheet(f *excelize.File, sheet string, rows [][]string) error {
	for ri, row := range rows {
		for ci, val := range row {
			cell, err := excelize.CoordinatesToCellName(ci+1, ri+1)
			if err != nil {
				return err
			}
			if err := f.SetCellValue(sheet, cell, val); err != nil {
				return err
			}
		}
	}
	_ = f.SetColWidth(sheet, "A", "A", 28)
	_ = f.SetColWidth(sheet, "B", "B", 36)
	_ = f.SetColWidth(sheet, "C", "D", 14)
	_ = f.SetColWidth(sheet, "E", "E", 12)
	_ = f.SetColWidth(sheet, "F", "G", 22)
	_ = f.SetColWidth(sheet, "H", "H", 18)
	_ = f.SetColWidth(sheet, "I", "I", 12)
	_ = f.SetColWidth(sheet, "J", "J", 52)
	_ = f.SetColWidth(sheet, "K", "K", 88)
	if len(rows) > 1 {
		if wrapID, err := f.NewStyle(&excelize.Style{
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
		}); err == nil {
			last := len(rows)
			lastStr := strconv.Itoa(last)
			_ = f.SetCellStyle(sheet, "J2", "J"+lastStr, wrapID)
			_ = f.SetCellStyle(sheet, "K2", "K"+lastStr, wrapID)
		}
	}
	return nil
}

func formatIntListForCSV(nums []int) string {
	if len(nums) == 0 {
		return "(ninguna)"
	}
	cp := append([]int(nil), nums...)
	sort.Ints(cp)
	parts := make([]string, len(cp))
	for i, n := range cp {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, "; ")
}

// BuildFileValidationReportFromPolicies construye el mismo informe que GetFileValidationReport
// sin leer la tabla policies (p. ej. cuando no se insertaron filas por incidencias).
func BuildFileValidationReportFromPolicies(
	fileID, fileName, productID, fileStatus, errorReason, processedAtRFC string,
	policies []model.PolicyRecord,
) FileValidationReport {
	inputs := make([]policyRowInput, 0, len(policies))
	for _, p := range policies {
		inputs = append(inputs, policyRowInput{
			RowNumber:      p.RowNumber,
			DocumentNumber: p.DocumentNumber,
			CreditNumber:   p.CreditNumber,
			PolicyStatus:   p.PolicyStatus,
			ValidationJSON: p.ValidationJSON,
			RawDataJSON:    p.RawDataJSON,
		})
	}
	return completeFileValidationReport(fileID, fileName, productID, fileStatus, errorReason, processedAtRFC, inputs)
}

func (s *Store) GetFileValidationReport(fileID string) (FileValidationReport, error) {
	var out FileValidationReport
	out.FileID = strings.TrimSpace(fileID)

	qFile := `SELECT id, file_name, product_id, status, error_reason, processed_at, validation_report_json FROM processed_files WHERE id = ? LIMIT 1`
	var productID, errorReason, validationReportJSON sql.NullString
	var processedAt sql.NullTime
	if err := s.db.QueryRow(qFile, out.FileID).Scan(&out.FileID, &out.FileName, &productID, &out.FileStatus, &errorReason, &processedAt, &validationReportJSON); err != nil {
		return out, err
	}
	pid := ""
	if productID.Valid {
		pid = productID.String
		out.ProductID = pid
	}
	er := ""
	if errorReason.Valid {
		er = strings.TrimSpace(errorReason.String)
		out.ErrorReason = er
	}
	procAtRFC := ""
	if processedAt.Valid {
		procAtRFC = processedAt.Time.UTC().Format(time.RFC3339Nano)
		out.ProcessedAt = procAtRFC
	}

	qPolicies := `SELECT ` + "`row_number`" + `, document_number, credit_number, policy_status, validation_json, raw_data_json
		FROM policies
		WHERE file_id = ?
		ORDER BY ` + "`row_number` ASC"
	rows, err := s.db.Query(qPolicies, out.FileID)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	inputs := make([]policyRowInput, 0)
	for rows.Next() {
		var in policyRowInput
		var documentNumber, creditNumber, validationJSON, rawDataJSON sql.NullString
		if err := rows.Scan(&in.RowNumber, &documentNumber, &creditNumber, &in.PolicyStatus, &validationJSON, &rawDataJSON); err != nil {
			continue
		}
		if documentNumber.Valid {
			in.DocumentNumber = documentNumber.String
		}
		if creditNumber.Valid {
			in.CreditNumber = creditNumber.String
		}
		if validationJSON.Valid {
			in.ValidationJSON = validationJSON.String
		}
		if rawDataJSON.Valid {
			in.RawDataJSON = rawDataJSON.String
		}
		inputs = append(inputs, in)
	}

	if len(inputs) == 0 && validationReportJSON.Valid && strings.TrimSpace(validationReportJSON.String) != "" {
		var stored FileValidationReport
		if err := json.Unmarshal([]byte(validationReportJSON.String), &stored); err != nil {
			return out, fmt.Errorf("validation_report_json inválido: %w", err)
		}
		if strings.TrimSpace(stored.FileID) == "" {
			stored.FileID = out.FileID
		}
		if strings.TrimSpace(stored.FileName) == "" {
			stored.FileName = out.FileName
		}
		if strings.TrimSpace(stored.ProductID) == "" {
			stored.ProductID = pid
		}
		if strings.TrimSpace(stored.FileStatus) == "" {
			stored.FileStatus = out.FileStatus
		}
		if strings.TrimSpace(stored.ErrorReason) == "" {
			stored.ErrorReason = er
		}
		if strings.TrimSpace(stored.ProcessedAt) == "" {
			stored.ProcessedAt = procAtRFC
		}
		return stored, nil
	}

	return completeFileValidationReport(out.FileID, out.FileName, pid, out.FileStatus, er, procAtRFC, inputs), nil
}

func (s *Store) runMigrations() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		applied_at DATETIME(6) NOT NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return err
	}

	migrations := map[int64]struct {
		name string
		sql  string
	}{
		2026042401: {
			name: "create_products",
			sql: `CREATE TABLE IF NOT EXISTS products (
			id VARCHAR(100) PRIMARY KEY,
			code VARCHAR(120) NOT NULL,
			insurer VARCHAR(60) NOT NULL,
			file_prefix VARCHAR(255) NOT NULL,
			sheet_name VARCHAR(255) NULL,
			header_row INT NOT NULL DEFAULT 1,
			mappings_json JSON NOT NULL,
			rules_json JSON NOT NULL,
			created_at DATETIME(6) NOT NULL,
			INDEX idx_products_prefix (file_prefix)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		2026042402: {
			name: "create_processed_files",
			sql: `CREATE TABLE IF NOT EXISTS processed_files (
			id VARCHAR(120) PRIMARY KEY,
			file_name VARCHAR(255) NOT NULL,
			product_id VARCHAR(100) NULL,
			file_hash VARCHAR(64) NULL,
			status VARCHAR(40) NOT NULL,
			error_reason TEXT NULL,
			remote_path VARCHAR(500) NOT NULL,
			processed_path VARCHAR(500) NULL,
			archive_path VARCHAR(800) NULL,
			processed_at DATETIME(6) NOT NULL,
			INDEX idx_processed_hash_status (file_hash, status),
			INDEX idx_processed_status_time (status, processed_at),
			INDEX idx_processed_product (product_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		2026042403: {
			name: "create_policies",
			sql: `CREATE TABLE IF NOT EXISTS policies (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			file_id VARCHAR(120) NOT NULL,
			product_id VARCHAR(100) NOT NULL,
			file_name VARCHAR(255) NOT NULL,
			` + "`row_number`" + ` INT NOT NULL,
			document_number VARCHAR(120) NULL,
			credit_number VARCHAR(120) NULL,
			policy_status VARCHAR(40) NOT NULL,
			raw_data_json JSON NOT NULL,
			created_at DATETIME(6) NOT NULL,
			INDEX idx_policies_file (file_id),
			INDEX idx_policies_product (product_id),
			INDEX idx_policies_document (document_number),
			INDEX idx_policies_credit (credit_number),
			INDEX idx_policies_status (policy_status)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		2026042404: {
			name: "add_validation_json_to_policies",
			sql:  `ALTER TABLE policies ADD COLUMN validation_json JSON NULL`,
		},
		2026042405: {
			name: "add_file_hash_to_processed_files",
			sql:  `ALTER TABLE processed_files ADD COLUMN file_hash VARCHAR(64) NULL`,
		},
		2026042406: {
			name: "create_product_allowed_premiums",
			sql: `CREATE TABLE IF NOT EXISTS product_allowed_premiums (
			product_id VARCHAR(100) NOT NULL,
			premium_value DECIMAL(14,4) NOT NULL,
			active TINYINT(1) NOT NULL DEFAULT 1,
			created_at DATETIME(6) NOT NULL,
			PRIMARY KEY (product_id, premium_value),
			INDEX idx_pap_product_active (product_id, active),
			INDEX idx_pap_value (premium_value)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		2026042407: {
			name: "add_archive_path_to_processed_files",
			sql:  `ALTER TABLE processed_files ADD COLUMN archive_path VARCHAR(800) NULL`,
		},
		2026042408: {
			name: "create_product_rule_params",
			sql: `CREATE TABLE IF NOT EXISTS product_rule_params (
			product_id VARCHAR(100) NOT NULL,
			param_key VARCHAR(120) NOT NULL,
			param_value TEXT NOT NULL,
			updated_at DATETIME(6) NOT NULL,
			PRIMARY KEY (product_id, param_key),
			INDEX idx_prp_product (product_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		2026042409: {
			name: "create_global_rule_params",
			sql: `CREATE TABLE IF NOT EXISTS global_rule_params (
			param_key VARCHAR(120) PRIMARY KEY,
			param_value TEXT NOT NULL,
			updated_at DATETIME(6) NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		2026042410: {
			name: "create_product_formats",
			sql: `CREATE TABLE IF NOT EXISTS product_formats (
			id VARCHAR(140) PRIMARY KEY,
			product_id VARCHAR(100) NOT NULL,
			format_name VARCHAR(120) NOT NULL,
			file_prefix VARCHAR(255) NOT NULL,
			sheet_name VARCHAR(255) NULL,
			header_row INT NOT NULL DEFAULT 1,
			priority INT NOT NULL DEFAULT 0,
			active TINYINT(1) NOT NULL DEFAULT 1,
			mappings_json JSON NOT NULL,
			rules_json JSON NOT NULL,
			created_at DATETIME(6) NOT NULL,
			INDEX idx_pf_product (product_id),
			INDEX idx_pf_prefix (file_prefix),
			INDEX idx_pf_active_priority (active, priority)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		2026042411: {
			name: "add_legacy_bootstrap_index_product_formats",
			sql:  `CREATE INDEX idx_pf_product_prefix ON product_formats (product_id, file_prefix)`,
		},
		2026042912: {
			name: "add_validation_report_json_to_processed_files",
			sql:  `ALTER TABLE processed_files ADD COLUMN validation_report_json JSON NULL`,
		},
		2026052113: {
			name: "add_email_error_to_processed_files",
			sql:  `ALTER TABLE processed_files ADD COLUMN email_error TEXT NULL`,
		},
	}

	applied := map[int64]bool{}
	rows, err := s.db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
	}

	versions := make([]int64, 0, len(migrations))
	for v := range migrations {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })

	for _, v := range versions {
		if applied[v] {
			continue
		}
		m := migrations[v]
		if _, err := s.db.Exec(m.sql); err != nil {
			// Compatibilidad: en MySQL antiguo no hay IF NOT EXISTS para ADD COLUMN.
			// Si el objeto ya existe (1060/1061), tratamos la migración como aplicada.
			if me, ok := err.(*mysql.MySQLError); !(ok && (me.Number == 1060 || me.Number == 1061)) {
				return err
			}
		}
		if _, err := s.db.Exec(`INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)`, v, m.name, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

// backfillFormatsFromLegacyProducts crea un formato por producto cuando existen
// configuraciones legacy en products y aún no hay registros en product_formats.
func (s *Store) backfillFormatsFromLegacyProducts() error {
	const sqlBackfill = `
	INSERT INTO product_formats (
		id, product_id, format_name, file_prefix, sheet_name, header_row, priority, active, mappings_json, rules_json, created_at
	)
	SELECT
		CONCAT(p.id, '_fmt_legacy'),
		p.id,
		'legacy-auto',
		p.file_prefix,
		NULLIF(p.sheet_name, ''),
		CASE WHEN p.header_row IS NULL OR p.header_row <= 0 THEN 1 ELSE p.header_row END,
		10,
		1,
		p.mappings_json,
		p.rules_json,
		NOW(6)
	FROM products p
	LEFT JOIN product_formats pf ON pf.product_id = p.id
	WHERE pf.id IS NULL
	  AND TRIM(COALESCE(p.file_prefix, '')) <> ''`
	_, err := s.db.Exec(sqlBackfill)
	return err
}

func nullableString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
