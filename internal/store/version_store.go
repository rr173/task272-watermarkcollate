package store

import (
	"database/sql"
	"fmt"

	"task272-watermarkcollate/internal/model"
)

// SaveVersion 插入或按乐观版本更新校勘版本。
func (s *Store) SaveVersion(v *model.CollationVersion) error {
	now := s.now()
	return s.WithTx(func(tx *sql.Tx) error {
		var exists int
		err := tx.QueryRow(`SELECT COUNT(1) FROM collation_versions WHERE id = ?`, v.ID).Scan(&exists)
		if err != nil {
			return err
		}
		if exists == 0 {
			v.CreatedAt = now
			_, err = tx.Exec(`INSERT INTO collation_versions(id,manuscript_id,version_no,status,summary,content_json,created_at,frozen_at,superseded_at)
				VALUES(?,?,?,?,?,?,?,?,?)`,
				v.ID, v.ManuscriptID, v.VersionNo, string(v.Status), v.Summary, v.ContentJSON,
				fmtTS(v.CreatedAt), nullableTS(v.FrozenAt), nullableTS(v.SupersededAt))
			if isUniqueViolation(err) {
				return model.NewDomainError(model.ErrDuplicateKey, "手稿 %s 的版本号 %d 已存在", v.ManuscriptID, v.VersionNo)
			}
			return err
		}
		var curStatus string
		var storedContent string
		if err := tx.QueryRow(`SELECT status,content_json FROM collation_versions WHERE id = ?`, v.ID).Scan(&curStatus, &storedContent); err != nil {
			return err
		}
		if !model.VersionStatus(curStatus).CanTransitionTo(v.Status) {
			return model.StateTransition("校勘版本", curStatus, string(v.Status))
		}
		// 不可变契约：已冻结（含被 superseded 的历史冻结）版本的关系快照不得改写。
		// 既有冻结版本仅允许流转状态（如冻结→替代），写入新 content_json 视为篡改快照。
		switch model.VersionStatus(curStatus) {
		case model.VersionFrozen, model.VersionSuperseded:
			if v.ContentJSON != storedContent {
				return model.NewDomainError(model.ErrFrozen, "校勘版本 %s 已冻结，快照不可改写", v.ID)
			}
		}
		switch v.Status {
		case model.VersionFrozen:
			if v.FrozenAt == nil {
				t := now
				v.FrozenAt = &t
			}
		case model.VersionSuperseded:
			if v.SupersededAt == nil {
				t := now
				v.SupersededAt = &t
			}
		}
		_, err = tx.Exec(`UPDATE collation_versions SET status=?,summary=?,content_json=?,frozen_at=?,superseded_at=? WHERE id=?`,
			string(v.Status), v.Summary, v.ContentJSON, nullableTS(v.FrozenAt), nullableTS(v.SupersededAt), v.ID)
		return err
	})
}

// GetVersion 按 ID 读取版本。
func (s *Store) GetVersion(id string) (*model.CollationVersion, error) {
	row := s.db.QueryRow(`SELECT id,manuscript_id,version_no,status,summary,content_json,created_at,frozen_at,superseded_at FROM collation_versions WHERE id=?`, id)
	return scanVersion(row)
}

// ListVersions 列出某手稿的版本（按版本号倒序）。
func (s *Store) ListVersions(manuscriptID string) ([]*model.CollationVersion, error) {
	rows, err := s.db.Query(`SELECT id,manuscript_id,version_no,status,summary,content_json,created_at,frozen_at,superseded_at FROM collation_versions WHERE manuscript_id=? ORDER BY version_no DESC`, manuscriptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.CollationVersion
	for rows.Next() {
		v, err := scanVersionRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// NextVersionNo 返回某手稿下一个版本号。
func (s *Store) NextVersionNo(manuscriptID string) (int, error) {
	var maxNo sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(version_no) FROM collation_versions WHERE manuscript_id=?`, manuscriptID).Scan(&maxNo); err != nil {
		return 0, err
	}
	if !maxNo.Valid {
		return 1, nil
	}
	return int(maxNo.Int64) + 1, nil
}

func scanVersion(row *sql.Row) (*model.CollationVersion, error) {
	v := &model.CollationVersion{}
	var status string
	var frozen, superseded *string
	var createdAt string
	if err := row.Scan(&v.ID, &v.ManuscriptID, &v.VersionNo, &status, &v.Summary, &v.ContentJSON,
		&createdAt, &frozen, &superseded); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.NotFound("校勘版本", "")
		}
		return nil, err
	}
	v.Status = model.VersionStatus(status)
	v.CreatedAt = parseTS(createdAt)
	v.FrozenAt = parseNullableTS(frozen)
	v.SupersededAt = parseNullableTS(superseded)
	return v, nil
}

func scanVersionRows(row leafScanner) (*model.CollationVersion, error) {
	v := &model.CollationVersion{}
	var status string
	var frozen, superseded *string
	var createdAt string
	if err := row.Scan(&v.ID, &v.ManuscriptID, &v.VersionNo, &status, &v.Summary, &v.ContentJSON,
		&createdAt, &frozen, &superseded); err != nil {
		return nil, err
	}
	v.Status = model.VersionStatus(status)
	v.CreatedAt = parseTS(createdAt)
	v.FrozenAt = parseNullableTS(frozen)
	v.SupersededAt = parseNullableTS(superseded)
	return v, nil
}

// CountTables 统计各表行数（供自检/统计 API 使用）。
func (s *Store) CountTables() (map[string]int64, error) {
	names := []string{"manuscripts", "leaves", "watermark_observations", "watermark_pairings", "leaf_relations", "collation_versions"}
	out := make(map[string]int64, len(names))
	for _, n := range names {
		var c int64
		if err := s.db.QueryRow(fmt.Sprintf(`SELECT COUNT(1) FROM %s`, n)).Scan(&c); err != nil {
			return nil, err
		}
		out[n] = c
	}
	return out, nil
}
