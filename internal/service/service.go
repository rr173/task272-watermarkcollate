package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"task272-watermarkcollate/internal/adjudicate"
	"task272-watermarkcollate/internal/chainline"
	"task272-watermarkcollate/internal/model"
	"task272-watermarkcollate/internal/quire"
	"task272-watermarkcollate/internal/store"
	"task272-watermarkcollate/internal/watermark"
)

// Service 是跨模块业务编排层：校验领域不变量、组合算法包、统一落库。
type Service struct {
	store *store.Store
}

// New 构造 Service。
func New(st *store.Store) *Service { return &Service{store: st} }

// genID 生成短随机 ID（16 字节 hex）。
func genID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------------- 手稿批次 ----------------

// CreateManuscript 创建手稿批次（organizing 起步）。
func (s *Service) CreateManuscript(title, period, description string) (*model.Manuscript, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, model.InvalidInput("手稿标题不能为空")
	}
	m := &model.Manuscript{
		ID:          genID(),
		Title:       title,
		Period:      period,
		Description: description,
		Status:      model.ManuscriptOrganizing,
		Version:     1,
	}
	if err := s.store.SaveManuscript(m); err != nil {
		return nil, fmt.Errorf("保存手稿: %w", err)
	}
	return m, nil
}

// ListManuscripts 列出全部手稿。
func (s *Service) ListManuscripts() ([]*model.Manuscript, error) { return s.store.ListManuscripts() }

// GetManuscript 读取手稿。
func (s *Service) GetManuscript(id string) (*model.Manuscript, error) { return s.store.GetManuscript(id) }

// UpdateManuscriptStatus 流转手稿状态（带乐观锁与状态机校验）。
func (s *Service) UpdateManuscriptStatus(id string, next model.ManuscriptStatus, version int) (*model.Manuscript, error) {
	m, err := s.store.GetManuscript(id)
	if err != nil {
		return nil, err
	}
	if m.Version != version {
		return nil, model.VersionMismatch("手稿", id, version, m.Version)
	}
	if !m.Status.CanTransitionTo(next) {
		return nil, model.StateTransition("手稿", string(m.Status), string(next))
	}
	m.Status = next
	if err := s.store.SaveManuscript(m); err != nil {
		return nil, fmt.Errorf("更新手稿状态: %w", err)
	}
	return m, nil
}

// SealManuscript 封存手稿（终态，封存后不可再导入纸页/水印/关系）。
func (s *Service) SealManuscript(id string, version int) (*model.Manuscript, error) {
	m, err := s.store.GetManuscript(id)
	if err != nil {
		return nil, err
	}
	if m.Status.IsTerminal() {
		return m, nil
	}
	if !m.Status.CanTransitionTo(model.ManuscriptSealed) {
		return nil, model.StateTransition("手稿", string(m.Status), string(model.ManuscriptSealed))
	}
	return s.UpdateManuscriptStatus(id, model.ManuscriptSealed, version)
}

// ---------------- 纸页观测 ----------------

// AddLeaf 导入纸页观测；封存手稿拒绝导入；(manuscript_id, page_no) 唯一。
func (s *Service) AddLeaf(m *model.Manuscript, l *model.Leaf) (*model.Leaf, error) {
	if m.Status.IsTerminal() {
		return nil, model.NewDomainError(model.ErrSealed, "手稿已封存，禁止导入纸页")
	}
	if l.PageNo <= 0 || l.QuireNo <= 0 {
		return nil, model.InvalidInput("页码与折页号必须为正整数")
	}
	if !model.ValidPosition(l.Position) {
		return nil, model.InvalidInput("未知开面 %q", l.Position)
	}
	if !model.ValidEdge(l.BindingEdge) {
		return nil, model.InvalidInput("未知装订边 %q", l.BindingEdge)
	}
	if l.Confidence < 0 || l.Confidence > 1 {
		return nil, model.InvalidInput("置信度必须位于 [0,1]")
	}
	if l.WidthMM <= 0 || l.HeightMM <= 0 {
		return nil, model.InvalidInput("纸页宽高必须为正数（毫米）")
	}
	if l.ChainDeg != chainline.Unknown && (l.ChainDeg < 0 || l.ChainDeg > 359) {
		return nil, model.InvalidInput("链线方向必须为 0-359 度或 -1（未观测）")
	}
	l.ID = genID()
	l.ManuscriptID = m.ID
	l.Status = model.LeafPending
	l.Version = 1
	if err := s.store.SaveLeaf(l); err != nil {
		return nil, fmt.Errorf("保存纸页: %w", err)
	}
	return l, nil
}

// GetLeaf 读取纸页。
func (s *Service) GetLeaf(id string) (*model.Leaf, error) { return s.store.GetLeaf(id) }

// ListLeaves 列出手稿纸页。
func (s *Service) ListLeaves(manuscriptID string) ([]*model.Leaf, error) {
	return s.store.ListLeaves(manuscriptID)
}

// UpdateLeaf 更新纸页（标记有效/破损/排除、调整置信度等；带乐观锁与状态机校验）。
func (s *Service) UpdateLeaf(id string, status model.LeafStatus, confidence float64, notes string, version int) (*model.Leaf, error) {
	l, err := s.store.GetLeaf(id)
	if err != nil {
		return nil, err
	}
	if l.Version != version {
		return nil, model.VersionMismatch("纸页", id, version, l.Version)
	}
	if !l.Status.CanTransitionTo(status) {
		return nil, model.StateTransition("纸页", string(l.Status), string(status))
	}
	if confidence < 0 || confidence > 1 {
		return nil, model.InvalidInput("置信度必须位于 [0,1]")
	}
	l.Status = status
	l.Confidence = confidence
	l.Notes = notes
	if err := s.store.SaveLeaf(l); err != nil {
		return nil, fmt.Errorf("更新纸页: %w", err)
	}
	return l, nil
}

// ---------------- 水印观测 ----------------

// AddWatermark 登记水印半片观测；(leaf_id, half_id) 唯一。
func (s *Service) AddWatermark(l *model.Leaf, w *model.WatermarkObservation) (*model.WatermarkObservation, error) {
	if l.Status.IsExcluded() {
		return nil, model.NewDomainError(model.ErrUnprocessable, "纸页 %s 已排除，不能登记水印", l.ID)
	}
	if !model.ValidWatermarkPosition(w.Position) {
		return nil, model.InvalidInput("未知水印半片位置 %q", w.Position)
	}
	if w.Confidence < 0 || w.Confidence > 1 {
		return nil, model.InvalidInput("置信度必须位于 [0,1]")
	}
	if w.XMM < 0 || w.YMM < 0 || w.XMM > l.WidthMM || w.YMM > l.HeightMM {
		return nil, model.InvalidInput("水印坐标越界：须位于纸页范围内（x∈[0,%.0fmm], y∈[0,%.0fmm]）", l.WidthMM, l.HeightMM)
	}
	w.ID = genID()
	w.LeafID = l.ID
	w.Status = model.WatermarkPending
	if err := s.store.SaveWatermark(w); err != nil {
		return nil, fmt.Errorf("保存水印观测: %w", err)
	}
	return w, nil
}

// ActivateWatermark 将水印观测置为 valid（成为配对证据）。
func (s *Service) ActivateWatermark(id string) (*model.WatermarkObservation, error) {
	w, err := s.store.GetWatermark(id)
	if err != nil {
		return nil, err
	}
	if !w.Status.CanTransitionTo(model.WatermarkValid) {
		return nil, model.StateTransition("水印观测", string(w.Status), string(model.WatermarkValid))
	}
	w.Status = model.WatermarkValid
	if err := s.store.SaveWatermark(w); err != nil {
		return nil, fmt.Errorf("激活水印观测: %w", err)
	}
	return w, nil
}

// ListWatermarksByLeaf 列出纸页水印观测。
func (s *Service) ListWatermarksByLeaf(leafID string) ([]*model.WatermarkObservation, error) {
	return s.store.ListWatermarksByLeaf(leafID)
}

// GetWatermark 读取水印观测。
func (s *Service) GetWatermark(id string) (*model.WatermarkObservation, error) {
	return s.store.GetWatermark(id)
}

// ---------------- 水印配对 ----------------

// RequestPairing 请求两个水印观测配对；已存在配对时直接返回既有记录。
// ctx 取消后不得再写入新配对。
func (s *Service) RequestPairing(ctx context.Context, m *model.Manuscript, aID, bID string) (*model.WatermarkPairing, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.Status.IsTerminal() {
		return nil, model.NewDomainError(model.ErrSealed, "手稿已封存，禁止新建配对")
	}
	a, err := s.store.GetWatermark(aID)
	if err != nil {
		return nil, err
	}
	b, err := s.store.GetWatermark(bID)
	if err != nil {
		return nil, err
	}
	if a.LeafID == b.LeafID {
		return nil, model.InvalidInput("不能配对同一纸页的两个观测")
	}
	existing, err := s.store.FindPairingByWatermarks(aID, bID)
	if err == nil {
		return existing, nil
	}
	if !model.IsNotFound(err) {
		return nil, fmt.Errorf("查找既有配对: %w", err)
	}
	leafA, err := s.store.GetLeaf(a.LeafID)
	if err != nil {
		return nil, err
	}
	p, err := watermark.Pair(a, b, leafA.WidthMM)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.ID = genID()
	p.ManuscriptID = m.ID
	p.Version = 1
	if err := s.store.SavePairing(p); err != nil {
		return nil, fmt.Errorf("保存水印配对: %w", err)
	}
	return p, nil
}

// ListPairings 列出手稿配对。
func (s *Service) ListPairings(manuscriptID string) ([]*model.WatermarkPairing, error) {
	return s.store.ListPairings(manuscriptID)
}

// PairingByID 读取配对。
func (s *Service) PairingByID(id string) (*model.WatermarkPairing, error) {
	return s.store.GetPairing(id)
}

// ConfirmPairing 人工确认/否决配对结果。
// 确认后回写相邻纸页关系的水印分；已确认关系的裁决不被改写。
func (s *Service) ConfirmPairing(ctx context.Context, id string, confirm bool, version int) (*model.WatermarkPairing, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := s.store.GetPairing(id)
	if err != nil {
		return nil, err
	}
	if p.Version != version {
		return nil, model.VersionMismatch("水印配对", id, version, p.Version)
	}
	next := model.PairingRejected
	if confirm {
		next = model.PairingConfirmed
	}
	if !p.Status.CanTransitionTo(next) {
		return nil, model.StateTransition("水印配对", string(p.Status), string(next))
	}
	p.Status = next
	if err := s.store.SavePairing(p); err != nil {
		return nil, fmt.Errorf("保存配对确认: %w", err)
	}
	if next == model.PairingConfirmed {
		if err := s.refreshRelationWatermarkScores(ctx, p); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// refreshRelationWatermarkScores 把已确认配对的评分写回尚未终态确认的相邻关系。
func (s *Service) refreshRelationWatermarkScores(ctx context.Context, p *model.WatermarkPairing) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a, err := s.store.GetWatermark(p.WatermarkAID)
	if err != nil {
		return fmt.Errorf("读取配对水印 A: %w", err)
	}
	b, err := s.store.GetWatermark(p.WatermarkBID)
	if err != nil {
		return fmt.Errorf("读取配对水印 B: %w", err)
	}
	rels, err := s.store.ListRelations(p.ManuscriptID)
	if err != nil {
		return fmt.Errorf("列出关系以回写水印分: %w", err)
	}
	for _, r := range rels {
		if r.Verdict.IsConfirmed() {
			continue
		}
		match := (r.LeftLeafID == a.LeafID && r.RightLeafID == b.LeafID) ||
			(r.LeftLeafID == b.LeafID && r.RightLeafID == a.LeafID)
		if !match {
			continue
		}
		left, err := s.store.GetLeaf(r.LeftLeafID)
		if err != nil {
			return fmt.Errorf("读取关系左页: %w", err)
		}
		right, err := s.store.GetLeaf(r.RightLeafID)
		if err != nil {
			return fmt.Errorf("读取关系右页: %w", err)
		}
		s.computeRelationEvidence(r, left, right)
		if r.Verdict == model.VerdictCandidate {
			verdict, ev, err := adjudicate.Aggregate(adjudicate.Input{
				WatermarkScore:  r.WatermarkScore,
				ChainConsistent: r.ChainConsistent,
				FoldContinuous:  r.FoldContinuous,
				HasWatermark:    r.WatermarkScore > 0,
			})
			if err != nil {
				return err
			}
			r.Verdict = verdict
			r.Evidence = adjudicate.RenderEvidence(ev)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.store.SaveRelation(r); err != nil {
			return fmt.Errorf("回写关系水印分: %w", err)
		}
	}
	return nil
}

// ---------------- 相邻纸页关系 ----------------

// CreateRelation 建立相邻纸页关系并自动计算证据（链线一致性 + 水印配对分 + 折页连续性）。
func (s *Service) CreateRelation(ctx context.Context, m *model.Manuscript, leftLeafID, rightLeafID string, adjudicator string) (*model.LeafRelation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.Status.IsTerminal() {
		return nil, model.NewDomainError(model.ErrSealed, "手稿已封存，禁止新建关系")
	}
	if leftLeafID == rightLeafID {
		return nil, model.InvalidInput("左右纸页不能相同")
	}
	existing, err := s.store.FindRelationByPair(leftLeafID, rightLeafID)
	if err == nil {
		return existing, nil
	}
	if !model.IsNotFound(err) {
		return nil, fmt.Errorf("查找既有关系: %w", err)
	}
	left, err := s.store.GetLeaf(leftLeafID)
	if err != nil {
		return nil, err
	}
	right, err := s.store.GetLeaf(rightLeafID)
	if err != nil {
		return nil, err
	}
	if left.ManuscriptID != m.ID || right.ManuscriptID != m.ID {
		return nil, model.InvalidInput("纸页不属于该手稿")
	}
	if !left.Status.IsValidObservation() || !right.Status.IsValidObservation() {
		return nil, model.NewDomainError(model.ErrUnprocessable, "纸页 %s/%s 必须为有效观测才能建立关系", left.LeafKey(), right.LeafKey())
	}
	r := &model.LeafRelation{
		ID:           genID(),
		ManuscriptID: m.ID,
		LeftLeafID:   leftLeafID,
		RightLeafID:  rightLeafID,
		PageDelta:    right.PageNo - left.PageNo,
		Verdict:      model.VerdictCandidate,
		Version:      1,
	}
	s.computeRelationEvidence(r, left, right)
	verdict, ev, err := adjudicate.Aggregate(adjudicate.Input{
		WatermarkScore:  r.WatermarkScore,
		ChainConsistent: r.ChainConsistent,
		FoldContinuous:  r.FoldContinuous,
		HasWatermark:    r.WatermarkScore > 0,
	})
	if err != nil {
		return nil, err
	}
	r.Verdict = verdict
	r.Evidence = adjudicate.RenderEvidence(ev)
	r.Adjudicator = adjudicator
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.store.SaveRelation(r); err != nil {
		return nil, fmt.Errorf("保存纸张关系: %w", err)
	}
	return r, nil
}

// computeRelationEvidence 聚合三通道证据到关系实体（纯计算，不落库）。
func (s *Service) computeRelationEvidence(r *model.LeafRelation, left, right *model.Leaf) {
	// 1) 链线一致性
	cr := chainline.Check(left.ChainDeg, right.ChainDeg)
	r.ChainConsistent = cr.Consistent
	// 2) 水印配对分：取该对纸页间已确认/匹配配对的最高分；无配对时取 0（标记无证据）。
	score := 0.0
	if ls, err := s.store.ListWatermarksByLeaf(left.ID); err == nil {
		for _, wa := range ls {
			if !wa.Status.ValidObservation() {
				continue
			}
			if rs, err := s.store.ListWatermarksByLeaf(right.ID); err == nil {
				for _, wb := range rs {
					if !wb.Status.ValidObservation() {
						continue
					}
					if p, err := s.store.FindPairingByWatermarks(wa.ID, wb.ID); err == nil {
						if p.Status == model.PairingMatched || p.Status == model.PairingConfirmed {
							if p.Score > score {
								score = p.Score
							}
						}
					}
				}
			}
		}
	}
	r.WatermarkScore = score
	// 3) 折页连续性（仅相邻页对自身）
	if r.PageDelta == 1 {
		r.FoldContinuous = right.QuireNo == left.QuireNo
		if !r.FoldContinuous {
			r.GapReasons = []string{"quire_jump"}
		}
	} else {
		r.FoldContinuous = false
		r.GapReasons = []string{"page_gap"}
	}
}

// ListRelations 列出手稿关系。
func (s *Service) ListRelations(manuscriptID string) ([]*model.LeafRelation, error) {
	return s.store.ListRelations(manuscriptID)
}

// RelationByID 读取关系。
func (s *Service) RelationByID(id string) (*model.LeafRelation, error) {
	return s.store.GetRelation(id)
}

// AdjudicateRelation 研究者裁决关系（同折页/重装订/冲突）；带乐观锁。
func (s *Service) AdjudicateRelation(id string, verdict model.RelationVerdict, adjudicator string, version int) (*model.LeafRelation, error) {
	r, err := s.store.GetRelation(id)
	if err != nil {
		return nil, err
	}
	if r.Version != version {
		return nil, model.VersionMismatch("纸张关系", id, version, r.Version)
	}
	if !r.Verdict.CanTransitionTo(verdict) {
		return nil, model.StateTransition("纸张关系", string(r.Verdict), string(verdict))
	}
	r.Verdict = verdict
	r.Adjudicator = adjudicator
	if err := s.store.SaveRelation(r); err != nil {
		return nil, fmt.Errorf("裁决纸张关系: %w", err)
	}
	return r, nil
}

// ConfirmRelation 确认既有裁决（终态）。
func (s *Service) ConfirmRelation(id string, adjudicator string, version int) (*model.LeafRelation, error) {
	r, err := s.store.GetRelation(id)
	if err != nil {
		return nil, err
	}
	if r.Version != version {
		return nil, model.VersionMismatch("纸张关系", id, version, r.Version)
	}
	if !r.Verdict.CanTransitionTo(model.VerdictConfirmed) {
		return nil, model.StateTransition("纸张关系", string(r.Verdict), string(model.VerdictConfirmed))
	}
	r.Verdict = model.VerdictConfirmed
	r.Adjudicator = adjudicator
	if err := s.store.SaveRelation(r); err != nil {
		return nil, fmt.Errorf("确认纸张关系: %w", err)
	}
	// 冻结版本的关系快照不可改写：此处不回写任何已冻结版本。
	// 新裁决如需进入快照，应通过 SupersedeVersion 产生新版本并在其冻结时固化。
	return r, nil
}

// ---------------- 折页连续性校验 / 重装订候选 ----------------

// VerifyManuscript 对整份手稿执行折页连续性校验，返回断裂点（重装订候选）。
func (s *Service) VerifyManuscript(m *model.Manuscript) (*quire.Verification, error) {
	leaves, err := s.store.ListLeaves(m.ID)
	if err != nil {
		return nil, err
	}
	ver := quire.Verify(leaves)
	ver.ManuscriptID = m.ID
	return &ver, nil
}

// RebindCandidates 汇总手稿中所有重装订候选关系与折页断裂点。
func (s *Service) RebindCandidates(m *model.Manuscript) (map[string]any, error) {
	rels, err := s.store.ListRelations(m.ID)
	if err != nil {
		return nil, err
	}
	rebound := []*model.LeafRelation{}
	for _, r := range rels {
		if r.Verdict == model.VerdictRebound || r.Verdict == model.VerdictConflict {
			rebound = append(rebound, r)
		}
	}
	ver, err := s.VerifyManuscript(m)
	if err != nil {
		return nil, err
	}
	sort.Slice(rebound, func(i, j int) bool { return rebound[i].CreatedAt.Before(rebound[j].CreatedAt) })
	return map[string]any{
		"manuscript_id":    m.ID,
		"fold_gaps":        ver.Gaps,
		"fold_continuous":  ver.Continuous,
		"rebound_relations": rebound,
	}, nil
}

// ---------------- 校勘版本 ----------------

// CreateVersion 创建校勘版本草稿：快照当前全部关系与裁决。
func (s *Service) CreateVersion(m *model.Manuscript, summary string) (*model.CollationVersion, error) {
	if m.Status.IsTerminal() {
		return nil, model.NewDomainError(model.ErrSealed, "手稿已封存，禁止新建版本")
	}
	no, err := s.store.NextVersionNo(m.ID)
	if err != nil {
		return nil, err
	}
	content, err := s.snapshotRelations(m.ID)
	if err != nil {
		return nil, err
	}
	v := &model.CollationVersion{
		ID:           genID(),
		ManuscriptID: m.ID,
		VersionNo:    no,
		Status:       model.VersionDraft,
		Summary:      summary,
		ContentJSON:  content,
	}
	if err := s.store.SaveVersion(v); err != nil {
		return nil, fmt.Errorf("保存校勘版本: %w", err)
	}
	return v, nil
}

// snapshotRelations 将手稿全部关系序列化为 JSON 快照。
func (s *Service) snapshotRelations(manuscriptID string) (string, error) {
	rels, err := s.store.ListRelations(manuscriptID)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(rels)
	if err != nil {
		return "", fmt.Errorf("序列化关系快照: %w", err)
	}
	return string(b), nil
}

// ListVersions 列出手稿版本。
func (s *Service) ListVersions(manuscriptID string) ([]*model.CollationVersion, error) {
	return s.store.ListVersions(manuscriptID)
}

func (s *Service) GetVersion(id string) (*model.CollationVersion, error) {
	v, err := s.store.GetVersion(id)
	if err != nil {
		return nil, err
	}
	// 冻结版本的关系快照在冻结时即固化，读取时原样返回，绝不重新生成；
	// 否则冻结后再改相邻关系裁决，旧快照会被当前关系覆盖，违背不可变契约。
	return v, nil
}

// FreezeVersion 冻结版本：先固化当前关系快照，再进入 frozen；冻结后内容不可修改。
func (s *Service) FreezeVersion(ctx context.Context, id string) (*model.CollationVersion, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	v, err := s.store.GetVersion(id)
	if err != nil {
		return nil, err
	}
	if !v.Status.CanTransitionTo(model.VersionFrozen) {
		return nil, model.StateTransition("校勘版本", string(v.Status), string(model.VersionFrozen))
	}
	content, err := s.snapshotRelations(v.ManuscriptID)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	v.ContentJSON = content
	v.Status = model.VersionFrozen
	if err := s.store.SaveVersion(v); err != nil {
		return nil, fmt.Errorf("冻结校勘版本: %w", err)
	}
	return v, nil
}

// SupersedeVersion 用新版本替代已冻结版本（历史快照不可变，仅新增替代关系）。
func (s *Service) SupersedeVersion(id string, summary string) (*model.CollationVersion, error) {
	old, err := s.store.GetVersion(id)
	if err != nil {
		return nil, err
	}
	if !old.Status.CanTransitionTo(model.VersionSuperseded) {
		return nil, model.StateTransition("校勘版本", string(old.Status), string(model.VersionSuperseded))
	}
	old.Status = model.VersionSuperseded
	if err := s.store.SaveVersion(old); err != nil {
		return nil, fmt.Errorf("替代校勘版本: %w", err)
	}
	m, err := s.store.GetManuscript(old.ManuscriptID)
	if err != nil {
		return nil, err
	}
	return s.CreateVersion(m, summary)
}

// ---------------- 统计 ----------------

// Stats 返回各表行数与状态分布（供 /api/stats 使用）。
func (s *Service) Stats() (map[string]any, error) {
	counts, err := s.store.CountTables()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"tables": counts,
	}, nil
}
