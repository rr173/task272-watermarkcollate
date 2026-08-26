package store

import (
	"database/sql"

	"task272-watermarkcollate/internal/model"
)

// SaveWatermark 插入水印观测；更新仅允许状态迁移（pending→valid/excluded 等）。
func (s *Store) SaveWatermark(w *model.WatermarkObservation) error {
	now := s.now()
	return s.WithTx(func(tx *sql.Tx) error {
		var exists int
		err := tx.QueryRow(`SELECT COUNT(1) FROM watermark_observations WHERE id = ?`, w.ID).Scan(&exists)
		if err != nil {
			return err
		}
		if exists == 0 {
			w.CreatedAt = now
			_, err = tx.Exec(`INSERT INTO watermark_observations(id,leaf_id,half_id,mold_pair_id,position,x_mm,y_mm,rotation_deg,confidence,status,notes,created_at)
				VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
				w.ID, w.LeafID, w.HalfID, w.MoldPairID, string(w.Position), w.XMM, w.YMM, w.RotationDeg,
				w.Confidence, string(w.Status), w.Notes, fmtTS(w.CreatedAt))
			if isUniqueViolation(err) {
				return model.NewDomainError(model.ErrDuplicateKey, "纸页 %s 已存在半片观测 %s", w.LeafID, w.HalfID)
			}
			return err
		}
		var curStatus string
		if err := tx.QueryRow(`SELECT status FROM watermark_observations WHERE id = ?`, w.ID).Scan(&curStatus); err != nil {
			return err
		}
		if !model.WatermarkStatus(curStatus).CanTransitionTo(w.Status) {
			return model.StateTransition("水印观测", curStatus, string(w.Status))
		}
		_, err = tx.Exec(`UPDATE watermark_observations SET status=?,notes=? WHERE id=?`,
			string(w.Status), w.Notes, w.ID)
		return err
	})
}

// GetWatermark 按 ID 读取水印观测。
func (s *Store) GetWatermark(id string) (*model.WatermarkObservation, error) {
	row := s.db.QueryRow(`SELECT id,leaf_id,half_id,mold_pair_id,position,x_mm,y_mm,rotation_deg,confidence,status,notes,created_at FROM watermark_observations WHERE id=?`, id)
	w := &model.WatermarkObservation{}
	var position, status string
	var createdAt string
	if err := row.Scan(&w.ID, &w.LeafID, &w.HalfID, &w.MoldPairID, &position, &w.XMM, &w.YMM, &w.RotationDeg,
		&w.Confidence, &status, &w.Notes, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.NotFound("水印观测", id)
		}
		return nil, err
	}
	w.Position = model.WatermarkPosition(position)
	w.Status = model.WatermarkStatus(status)
	w.CreatedAt = parseTS(createdAt)
	return w, nil
}

// ListWatermarksByLeaf 列出某纸页的水印观测。
func (s *Store) ListWatermarksByLeaf(leafID string) ([]*model.WatermarkObservation, error) {
	rows, err := s.db.Query(`SELECT id,leaf_id,half_id,mold_pair_id,position,x_mm,y_mm,rotation_deg,confidence,status,notes,created_at FROM watermark_observations WHERE leaf_id=? ORDER BY created_at`, leafID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.WatermarkObservation
	for rows.Next() {
		w := &model.WatermarkObservation{}
		var position, status string
		var createdAt string
		if err := rows.Scan(&w.ID, &w.LeafID, &w.HalfID, &w.MoldPairID, &position, &w.XMM, &w.YMM, &w.RotationDeg,
			&w.Confidence, &status, &w.Notes, &createdAt); err != nil {
			return nil, err
		}
		w.Position = model.WatermarkPosition(position)
		w.Status = model.WatermarkStatus(status)
		w.CreatedAt = parseTS(createdAt)
		out = append(out, w)
	}
	return out, rows.Err()
}
