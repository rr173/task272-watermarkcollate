package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"task272-watermarkcollate/internal/model"
)

// SavePairing 插入水印配对（自动判定结果）；更新仅允许状态迁移。
func (s *Store) SavePairing(p *model.WatermarkPairing) error {
	now := s.now()
	return s.WithTx(func(tx *sql.Tx) error {
		var exists int
		err := tx.QueryRow(`SELECT COUNT(1) FROM watermark_pairings WHERE id = ?`, p.ID).Scan(&exists)
		if err != nil {
			return err
		}
		if exists == 0 {
			p.CreatedAt = now
			_, err = tx.Exec(`INSERT INTO watermark_pairings(id,manuscript_id,watermark_a_id,watermark_b_id,mold_pair_id,score,status,evidence,version,created_at,confirmed_at)
				VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
				p.ID, p.ManuscriptID, p.WatermarkAID, p.WatermarkBID, p.MoldPairID, p.Score, string(p.Status),
				p.Evidence, p.Version, fmtTS(p.CreatedAt), nullableTS(p.ConfirmedAt))
			if isUniqueViolation(err) {
				return model.NewDomainError(model.ErrDuplicateKey, "配对 (%s, %s) 已存在", p.WatermarkAID, p.WatermarkBID)
			}
			return err
		}
		var curVersion int
		var curStatus string
		if err := tx.QueryRow(`SELECT version,status FROM watermark_pairings WHERE id = ?`, p.ID).Scan(&curVersion, &curStatus); err != nil {
			return err
		}
		if curVersion != p.Version {
			return model.VersionMismatch("水印配对", p.ID, p.Version, curVersion)
		}
		if !model.PairingStatus(curStatus).CanTransitionTo(p.Status) {
			return model.StateTransition("水印配对", curStatus, string(p.Status))
		}
		p.Version = curVersion + 1
		if p.Status == model.PairingConfirmed || p.Status == model.PairingRejected {
			t := now
			p.ConfirmedAt = &t
		}
		_, err = tx.Exec(`UPDATE watermark_pairings SET score=?,status=?,evidence=?,version=?,confirmed_at=? WHERE id=?`,
			p.Score, string(p.Status), p.Evidence, p.Version, nullableTS(p.ConfirmedAt), p.ID)
		return err
	})
}

// GetPairing 按 ID 读取配对。
func (s *Store) GetPairing(id string) (*model.WatermarkPairing, error) {
	row := s.db.QueryRow(`SELECT id,manuscript_id,watermark_a_id,watermark_b_id,mold_pair_id,score,status,evidence,version,created_at,confirmed_at FROM watermark_pairings WHERE id=?`, id)
	return scanPairing(row)
}

// ListPairings 列出某手稿的配对（按创建时间倒序）。
func (s *Store) ListPairings(manuscriptID string) ([]*model.WatermarkPairing, error) {
	rows, err := s.db.Query(`SELECT id,manuscript_id,watermark_a_id,watermark_b_id,mold_pair_id,score,status,evidence,version,created_at,confirmed_at FROM watermark_pairings WHERE manuscript_id=? ORDER BY created_at DESC`, manuscriptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.WatermarkPairing
	for rows.Next() {
		p, err := scanPairingRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// FindPairingByWatermarks 按两个水印 ID 查找既有配对（避免重复创建）。
func (s *Store) FindPairingByWatermarks(aID, bID string) (*model.WatermarkPairing, error) {
	row := s.db.QueryRow(`SELECT id,manuscript_id,watermark_a_id,watermark_b_id,mold_pair_id,score,status,evidence,version,created_at,confirmed_at
		FROM watermark_pairings WHERE (watermark_a_id=? AND watermark_b_id=?) OR (watermark_a_id=? AND watermark_b_id=?)`,
		aID, bID, bID, aID)
	return scanPairing(row)
}

func scanPairing(row *sql.Row) (*model.WatermarkPairing, error) {
	p := &model.WatermarkPairing{}
	var status string
	var confirmedAt *string
	var createdAt string
	if err := row.Scan(&p.ID, &p.ManuscriptID, &p.WatermarkAID, &p.WatermarkBID, &p.MoldPairID, &p.Score,
		&status, &p.Evidence, &p.Version, &createdAt, &confirmedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.NotFound("水印配对", "")
		}
		return nil, err
	}
	p.Status = model.PairingStatus(status)
	p.CreatedAt = parseTS(createdAt)
	p.ConfirmedAt = parseNullableTS(confirmedAt)
	return p, nil
}

func scanPairingRows(row leafScanner) (*model.WatermarkPairing, error) {
	p := &model.WatermarkPairing{}
	var status string
	var confirmedAt *string
	var createdAt string
	if err := row.Scan(&p.ID, &p.ManuscriptID, &p.WatermarkAID, &p.WatermarkBID, &p.MoldPairID, &p.Score,
		&status, &p.Evidence, &p.Version, &createdAt, &confirmedAt); err != nil {
		return nil, err
	}
	p.Status = model.PairingStatus(status)
	p.CreatedAt = parseTS(createdAt)
	p.ConfirmedAt = parseNullableTS(confirmedAt)
	return p, nil
}

func nullableTS(t *time.Time) any {
	if t == nil {
		return nil
	}
	return fmtTS(*t)
}

func parseNullableTS(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t := parseTS(*s)
	return &t
}

// SaveRelation 插入相邻纸页关系（自动证据计算后落库）。
func (s *Store) SaveRelation(r *model.LeafRelation) error {
	now := s.now()
	return s.WithTx(func(tx *sql.Tx) error {
		var exists int
		err := tx.QueryRow(`SELECT COUNT(1) FROM leaf_relations WHERE id = ?`, r.ID).Scan(&exists)
		if err != nil {
			return err
		}
		gapJSON, _ := json.Marshal(r.GapReasons)
		if exists == 0 {
			r.CreatedAt = now
			_, err = tx.Exec(`INSERT INTO leaf_relations(id,manuscript_id,left_leaf_id,right_leaf_id,page_delta,chain_consistent,watermark_score,fold_continuous,gap_reasons,verdict,evidence,adjudicator,version,created_at,adjudicated_at)
				VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				r.ID, r.ManuscriptID, r.LeftLeafID, r.RightLeafID, r.PageDelta, boolInt(r.ChainConsistent),
				r.WatermarkScore, boolInt(r.FoldContinuous), string(gapJSON), string(r.Verdict), r.Evidence,
				r.Adjudicator, r.Version, fmtTS(r.CreatedAt), nullableTS(r.AdjudicatedAt))
			if isUniqueViolation(err) {
				return model.NewDomainError(model.ErrDuplicateKey, "关系 (%s, %s) 已存在", r.LeftLeafID, r.RightLeafID)
			}
			return err
		}
		var curVersion int
		var curVerdict string
		if err := tx.QueryRow(`SELECT version,verdict FROM leaf_relations WHERE id = ?`, r.ID).Scan(&curVersion, &curVerdict); err != nil {
			return err
		}
		if curVersion != r.Version {
			return model.VersionMismatch("纸张关系", r.ID, r.Version, curVersion)
		}
		if !model.RelationVerdict(curVerdict).CanTransitionTo(r.Verdict) {
			return model.StateTransition("纸张关系", curVerdict, string(r.Verdict))
		}
		r.Version = curVersion + 1
		if r.Verdict == model.VerdictConfirmed {
			t := now
			r.AdjudicatedAt = &t
		}
		_, err = tx.Exec(`UPDATE leaf_relations SET chain_consistent=?,watermark_score=?,fold_continuous=?,gap_reasons=?,verdict=?,evidence=?,adjudicator=?,version=?,adjudicated_at=? WHERE id=?`,
			boolInt(r.ChainConsistent), r.WatermarkScore, boolInt(r.FoldContinuous), string(gapJSON), string(r.Verdict),
			r.Evidence, r.Adjudicator, r.Version, nullableTS(r.AdjudicatedAt), r.ID)
		return err
	})
}

// GetRelation 按 ID 读取关系。
func (s *Store) GetRelation(id string) (*model.LeafRelation, error) {
	row := s.db.QueryRow(`SELECT id,manuscript_id,left_leaf_id,right_leaf_id,page_delta,chain_consistent,watermark_score,fold_continuous,gap_reasons,verdict,evidence,adjudicator,version,created_at,adjudicated_at FROM leaf_relations WHERE id=?`, id)
	return scanRelation(row)
}

// ListRelations 列出某手稿的关系（按创建时间倒序）。
func (s *Store) ListRelations(manuscriptID string) ([]*model.LeafRelation, error) {
	rows, err := s.db.Query(`SELECT id,manuscript_id,left_leaf_id,right_leaf_id,page_delta,chain_consistent,watermark_score,fold_continuous,gap_reasons,verdict,evidence,adjudicator,version,created_at,adjudicated_at FROM leaf_relations WHERE manuscript_id=? ORDER BY created_at DESC`, manuscriptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.LeafRelation
	for rows.Next() {
		r, err := scanRelationRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FindRelationByPair 按左右纸页查找关系。
func (s *Store) FindRelationByPair(leftID, rightID string) (*model.LeafRelation, error) {
	row := s.db.QueryRow(`SELECT id,manuscript_id,left_leaf_id,right_leaf_id,page_delta,chain_consistent,watermark_score,fold_continuous,gap_reasons,verdict,evidence,adjudicator,version,created_at,adjudicated_at FROM leaf_relations WHERE left_leaf_id=? AND right_leaf_id=?`, leftID, rightID)
	return scanRelation(row)
}

func scanRelation(row *sql.Row) (*model.LeafRelation, error) {
	r := &model.LeafRelation{}
	var chain, fold int
	var verdict string
	var gapJSON string
	var adj *string
	var createdAt string
	if err := row.Scan(&r.ID, &r.ManuscriptID, &r.LeftLeafID, &r.RightLeafID, &r.PageDelta, &chain, &r.WatermarkScore,
		&fold, &gapJSON, &verdict, &r.Evidence, &r.Adjudicator, &r.Version, &createdAt, &adj); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.NotFound("纸张关系", "")
		}
		return nil, err
	}
	r.ChainConsistent = chain == 1
	r.FoldContinuous = fold == 1
	r.Verdict = model.RelationVerdict(verdict)
	r.GapReasons = decodeGapReasons(gapJSON)
	r.CreatedAt = parseTS(createdAt)
	r.AdjudicatedAt = parseNullableTS(adj)
	return r, nil
}

func decodeGapReasons(gapJSON string) []string {
	parsed := []string{}
	if gapJSON != "" {
		_ = json.Unmarshal([]byte(gapJSON), &parsed)
	}
	out := make([]string, len(parsed))
	copy(out, parsed)
	return out
}

func scanRelationRows(row leafScanner) (*model.LeafRelation, error) {
	r := &model.LeafRelation{}
	var chain, fold int
	var verdict string
	var gapJSON string
	var adj *string
	var createdAt string
	if err := row.Scan(&r.ID, &r.ManuscriptID, &r.LeftLeafID, &r.RightLeafID, &r.PageDelta, &chain, &r.WatermarkScore,
		&fold, &gapJSON, &verdict, &r.Evidence, &r.Adjudicator, &r.Version, &createdAt, &adj); err != nil {
		return nil, err
	}
	r.ChainConsistent = chain == 1
	r.FoldContinuous = fold == 1
	r.Verdict = model.RelationVerdict(verdict)
	r.GapReasons = decodeGapReasons(gapJSON)
	r.CreatedAt = parseTS(createdAt)
	r.AdjudicatedAt = parseNullableTS(adj)
	return r, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

var _ = strings.TrimSpace
