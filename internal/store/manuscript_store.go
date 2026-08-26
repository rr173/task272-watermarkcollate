package store

import (
	"database/sql"
	"fmt"
	"time"

	"task272-watermarkcollate/internal/model"
)

// SaveManuscript 插入或按乐观版本更新手稿。version 不匹配返回 VERSION_MISMATCH。
func (s *Store) SaveManuscript(m *model.Manuscript) error {
	now := s.now()
	return s.WithTx(func(tx *sql.Tx) error {
		var exists int
		err := tx.QueryRow(`SELECT COUNT(1) FROM manuscripts WHERE id = ?`, m.ID).Scan(&exists)
		if err != nil {
			return err
		}
		if exists == 0 {
			m.CreatedAt = now
			m.UpdatedAt = now
			_, err = tx.Exec(`INSERT INTO manuscripts(id,title,period,description,status,version,created_at,updated_at)
				VALUES(?,?,?,?,?,?,?,?)`,
				m.ID, m.Title, m.Period, m.Description, string(m.Status), m.Version, fmtTS(m.CreatedAt), fmtTS(m.UpdatedAt))
			return err
		}
		// 乐观锁：仅当版本一致才覆盖状态与元数据。
		var curVersion int
		if err := tx.QueryRow(`SELECT version FROM manuscripts WHERE id = ?`, m.ID).Scan(&curVersion); err != nil {
			return err
		}
		if curVersion != m.Version {
			return model.VersionMismatch("手稿", m.ID, m.Version, curVersion)
		}
		m.Version = curVersion + 1
		m.UpdatedAt = now
		_, err = tx.Exec(`UPDATE manuscripts SET title=?,period=?,description=?,status=?,version=?,updated_at=? WHERE id=?`,
			m.Title, m.Period, m.Description, string(m.Status), m.Version, fmtTS(m.UpdatedAt), m.ID)
		return err
	})
}

// GetManuscript 按 ID 读取手稿。
func (s *Store) GetManuscript(id string) (*model.Manuscript, error) {
	row := s.db.QueryRow(`SELECT id,title,period,description,status,version,created_at,updated_at FROM manuscripts WHERE id=?`, id)
	m := &model.Manuscript{}
	var status string
	var createdAt, updatedAt string
	if err := row.Scan(&m.ID, &m.Title, &m.Period, &m.Description, &status, &m.Version, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.NotFound("手稿", id)
		}
		return nil, err
	}
	m.Status = model.ManuscriptStatus(status)
	m.CreatedAt, m.UpdatedAt = parseTS(createdAt), parseTS(updatedAt)
	return m, nil
}

// ListManuscripts 列出全部手稿（按创建时间倒序）。
func (s *Store) ListManuscripts() ([]*model.Manuscript, error) {
	rows, err := s.db.Query(`SELECT id,title,period,description,status,version,created_at,updated_at FROM manuscripts ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Manuscript
	for rows.Next() {
		m := &model.Manuscript{}
		var status string
		var createdAt, updatedAt string
		if err := rows.Scan(&m.ID, &m.Title, &m.Period, &m.Description, &status, &m.Version, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		m.Status = model.ManuscriptStatus(status)
		m.CreatedAt, m.UpdatedAt = parseTS(createdAt), parseTS(updatedAt)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) now() time.Time { return Now() }

func fmtTS(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTS(v any) time.Time {
	switch x := v.(type) {
	case time.Time:
		return x
	case string:
		t, _ := time.Parse(time.RFC3339Nano, x)
		return t
	case []byte:
		t, _ := time.Parse(time.RFC3339Nano, string(x))
		return t
	case nil:
		return time.Time{}
	default:
		return time.Time{}
	}
}

var _ = fmt.Sprintf
