package store

import (
	"database/sql"
	"encoding/json"
	"strings"

	sq "github.com/Masterminds/squirrel"
)

// MapfreStockMatch es una póliza MAPFRE vigente encontrada por crédito (OP BT / NUMEROPRESTAMO).
type MapfreStockMatch struct {
	FileID         string
	ProductID      string
	DocumentNumber string
	CreditNumber   string
	PolicyStatus   string
	GroupPolicy    string
	ActivationDate string
	RawDataJSON    string
}

// MapfreStockCrossProductIDs productos donde se busca stock para cruce de cancelaciones (diagrama C.1.e).
func MapfreStockCrossProductIDs() []string {
	return []string{
		"mapfre_stock_cartera",
		"mapfre_inclusion_vida_voluntario",
		"mapfre_inclusion_ap_menores",
		"mapfre_inclusion_ap_cancer",
	}
}

// FindLatestMapfreStockPolicy devuelve la póliza MAPFRE más reciente no cancelada para un crédito.
func (s *Store) FindLatestMapfreStockPolicy(creditNumber, _ string) (MapfreStockMatch, bool) {
	credit := strings.TrimSpace(creditNumber)
	if credit == "" {
		return MapfreStockMatch{}, false
	}
	productIDs := MapfreStockCrossProductIDs()
	qb := s.sb.Select(
		"file_id", "product_id", "document_number", "credit_number", "policy_status", "raw_data_json",
	).From("policies").
		Where(sq.Eq{"credit_number": credit}).
		Where(sq.Eq{"product_id": productIDs}).
		Where(sq.NotEq{"policy_status": "CANCELLED"}).
		OrderBy("CASE product_id WHEN 'mapfre_stock_cartera' THEN 0 ELSE 1 END", "created_at DESC").
		Limit(1)

	sqlStr, args, _ := qb.ToSql()
	var m MapfreStockMatch
	var documentNum sql.NullString
	var raw sql.NullString
	err := s.db.QueryRow(sqlStr, args...).Scan(
		&m.FileID, &m.ProductID, &documentNum, &m.CreditNumber, &m.PolicyStatus, &raw,
	)
	if err != nil {
		return MapfreStockMatch{}, false
	}
	if documentNum.Valid {
		m.DocumentNumber = documentNum.String
	}
	if raw.Valid {
		m.RawDataJSON = raw.String
		m.GroupPolicy = mapfreGroupPolicyFromRaw(raw.String)
		m.ActivationDate = mapfreActivationFromRaw(raw.String)
	}
	return m, true
}

// ApplyMapfreStockCancellation marca CANCELLED las pólizas MAPFRE del crédito (cruce anulación → stock).
func (s *Store) ApplyMapfreStockCancellation(creditNumber, documentNumber string) (int64, error) {
	credit := strings.TrimSpace(creditNumber)
	if credit == "" {
		return 0, nil
	}
	productIDs := MapfreStockCrossProductIDs()
	qb := s.sb.Update("policies").
		Set("policy_status", "CANCELLED").
		Where(sq.Eq{"credit_number": credit}).
		Where(sq.Eq{"product_id": productIDs}).
		Where(sq.NotEq{"policy_status": "CANCELLED"})

	doc := normalizeMapfreDocument(documentNumber)
	if doc != "" {
		qb = qb.Where(sq.Or{
			sq.Eq{"document_number": doc},
			sq.Eq{"document_number": strings.TrimSpace(documentNumber)},
		})
	}

	sqlStr, args, err := qb.ToSql()
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(sqlStr, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func mapfreGroupPolicyFromRaw(rawJSON string) string {
	raw := strings.TrimSpace(rawJSON)
	if raw == "" {
		return ""
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return ""
	}
	for _, key := range []string{
		"group_policy_number",
		"NUMER PÓLIZA GRUPO",
		"NUMER POLIZA GRUPO",
		"plan_code",
		"CODIGO SEGURO",
		"policy_number",
	} {
		if v := strings.TrimSpace(values[key]); v != "" {
			return v
		}
	}
	return ""
}

func mapfreActivationFromRaw(rawJSON string) string {
	raw := strings.TrimSpace(rawJSON)
	if raw == "" {
		return ""
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return ""
	}
	for _, key := range []string{"activation_date", "FECHA DE ACTIVACIÓN", "FECHAACTIVACION", "coverage_start_date"} {
		if v := strings.TrimSpace(values[key]); v != "" {
			return v
		}
	}
	return ""
}

func normalizeMapfreDocument(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	out := strings.TrimLeft(b.String(), "0")
	if out == "" && b.Len() > 0 {
		return "0"
	}
	return out
}
