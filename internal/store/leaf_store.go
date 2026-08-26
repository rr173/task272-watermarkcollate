package store

import (
	"database/sql"
	"strings"

	"task272-watermarkcollate/internal/model"
)

// SaveLeaf 插入或按乐观版本更新纸页观测。
// (manuscript_id, page_no) 重复返回 DUPLICATE_KEY；状态迁移非法返回 STATE_TRANSITION。
func (s *Store) SaveLeaf(l *model.Leaf) error {
	now := s.now()
	return s.WithTx(func(tx *sql.Tx) error {
		var exists int
		err := tx.QueryRow(`SELECT COUNT(1) FROM leaves WHERE id = ?`, l.ID).Scan(&exists)
		if err != nil {
			return err
		}
		if exists == 0 {
			l.CreatedAt = now
			l.UpdatedAt = now
			_, err = tx.Exec(`INSERT INTO leaves(id,manuscript_id,page_no,quire_no,position,status,binding_edge,chain_deg,width_mm,height_mm,confidence,notes,version,created_at,updated_at)
				VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				l.ID, l.ManuscriptID, l.PageNo, l.QuireNo, string(l.Position), string(l.Status),
				string(l.BindingEdge), l.ChainDeg, l.WidthMM, l.HeightMM, l.Confidence, l.Notes, l.Version,
				fmtTS(l.CreatedAt), fmtTS(l.UpdatedAt))
			if isUniqueViolation(err) {
				return model.NewDomainError(model.ErrDuplicateKey, "纸页 (manuscript=%s, page=%d) 已存在", l.ManuscriptID, l.PageNo)
			}
			return err
		}
		var curVersion int
		var curStatus string
		if err := tx.QueryRow(`SELECT version,status FROM leaves WHERE id = ?`, l.ID).Scan(&curVersion, &curStatus); err != nil {
			return err
		}
		if curVersion != l.Version {
			return model.VersionMismatch("纸页", l.ID, l.Version, curVersion)
		}
		if !model.LeafStatus(curStatus).CanTransitionTo(l.Status) {
			return model.StateTransition("纸页", curStatus, string(l.Status))
		}
		l.Version = curVersion + 1
		l.UpdatedAt = now
		_, err = tx.Exec(`UPDATE leaves SET quire_no=?,position=?,status=?,binding_edge=?,chain_deg=?,width_mm=?,height_mm=?,confidence=?,notes=?,version=?,updated_at=? WHERE id=?`,
			l.QuireNo, string(l.Position), string(l.Status), string(l.BindingEdge), l.ChainDeg, l.WidthMM,
			l.HeightMM, l.Confidence, l.Notes, l.Version, fmtTS(l.UpdatedAt), l.ID)
		return err
	})
}

// GetLeaf 按 ID 读取纸页。
func (s *Store) GetLeaf(id string) (*model.Leaf, error) {
	row := s.db.QueryRow(`SELECT id,manuscript_id,page_no,quire_no,position,status,binding_edge,chain_deg,width_mm,height_mm,confidence,notes,version,created_at,updated_at FROM leaves WHERE id=?`, id)
	return scanLeaf(row)
}

// ListLeaves 列出某手稿的全部纸页（按折页号、页号排序）。
func (s *Store) ListLeaves(manuscriptID string) ([]*model.Leaf, error) {
	if cached, ok := s.leafCache[manuscriptID]; ok {
		return cached, nil
	}
	rows, err := s.db.Query(`SELECT id,manuscript_id,page_no,quire_no,position,status,binding_edge,chain_deg,width_mm,height_mm,confidence,notes,version,created_at,updated_at FROM leaves WHERE manuscript_id=? ORDER BY quire_no,page_no`, manuscriptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Leaf
	for rows.Next() {
		l, err := scanLeafRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.leafCache[manuscriptID] = out
	return out, nil
}

func scanLeaf(row *sql.Row) (*model.Leaf, error) {
	l := &model.Leaf{}
	var position, status, edge string
	var createdAt, updatedAt string
	if err := row.Scan(&l.ID, &l.ManuscriptID, &l.PageNo, &l.QuireNo, &position, &status, &edge,
		&l.ChainDeg, &l.WidthMM, &l.HeightMM, &l.Confidence, &l.Notes, &l.Version, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.NotFound("纸页", "")
		}
		return nil, err
	}
	l.Position = model.LeafPosition(position)
	l.Status = model.LeafStatus(status)
	l.BindingEdge = model.BindingEdge(edge)
	l.CreatedAt, l.UpdatedAt = parseTS(createdAt), parseTS(updatedAt)
	return l, nil
}

type leafScanner interface {
	Scan(dest ...any) error
}

func scanLeafRows(row leafScanner) (*model.Leaf, error) {
	l := &model.Leaf{}
	var position, status, edge string
	var createdAt, updatedAt string
	if err := row.Scan(&l.ID, &l.ManuscriptID, &l.PageNo, &l.QuireNo, &position, &status, &edge,
		&l.ChainDeg, &l.WidthMM, &l.HeightMM, &l.Confidence, &l.Notes, &l.Version, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	l.Position = model.LeafPosition(position)
	l.Status = model.LeafStatus(status)
	l.BindingEdge = model.BindingEdge(edge)
	l.CreatedAt, l.UpdatedAt = parseTS(createdAt), parseTS(updatedAt)
	return l, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "unique")
}
