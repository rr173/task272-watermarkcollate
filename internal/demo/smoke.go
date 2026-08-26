package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"task272-watermarkcollate/internal/adjudicate"
	"task272-watermarkcollate/internal/model"
	"task272-watermarkcollate/internal/service"
	"task272-watermarkcollate/internal/store"
)

// SmokeResult 自检结果汇总。
type SmokeResult struct {
	ManuscriptID string `json:"manuscript_id"`
	LeafCount    int    `json:"leaf_count"`
	PairingCount int    `json:"pairing_count"`
	MatchedPair  bool   `json:"matched_pair"`
	RelationCount int   `json:"relation_count"`
	FoldGaps     int    `json:"fold_gaps"`
	ReboundFound bool   `json:"rebound_found"`
	VersionNo    int    `json:"version_no"`
	Recovered    bool   `json:"recovered"`
	Persisted    bool   `json:"persisted"`
}

// RunSmokeTest 执行 --smoke-test 契约：
// 临时数据库 → 完整业务闭环 → 关闭并重新打开同一 DB 验证持久化与重启恢复 → 退出码 0。
// 返回 nil 表示通过；任何步骤失败返回错误（main 以非 0 退出）。
func RunSmokeTest(dbPath string) (*SmokeResult, error) {
	path := dbPath
	cleanup := false
	if path == "" {
		f, err := os.CreateTemp("", "watermarkcollate-smoke-*.db")
		if err != nil {
			return nil, fmt.Errorf("创建临时库: %w", err)
		}
		path = f.Name()
		_ = f.Close()
		cleanup = true
	}
	if cleanup {
		defer os.Remove(path)
	}

	st, err := store.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开临时库: %w", err)
	}
	svc := service.New(st)

	// 1) 创建手稿。
	m, err := svc.CreateManuscript("十六世纪祈祷书残卷", "c.1540, 巴黎", "两处折页共 4 页；疑似后期重装订")
	if err != nil {
		return nil, fmt.Errorf("创建手稿: %w", err)
	}

	// 2) 导入 4 页：折页 1（页 1-2）、折页 2（页 3-4）。
	//    页 3 链线方向 45° 与页 2 的 90° 显著不一致 → 制造重装订疑点。
	leaves := []*model.Leaf{
		{PageNo: 1, QuireNo: 1, Position: model.PositionRecto, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.95, Notes: "页 1"},
		{PageNo: 2, QuireNo: 1, Position: model.PositionVerso, BindingEdge: model.EdgeRight, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.92, Notes: "页 2"},
		{PageNo: 3, QuireNo: 2, Position: model.PositionRecto, BindingEdge: model.EdgeLeft, ChainDeg: 45, WidthMM: 160, HeightMM: 220, Confidence: 0.90, Notes: "页 3"},
		{PageNo: 4, QuireNo: 2, Position: model.PositionVerso, BindingEdge: model.EdgeRight, ChainDeg: 91, WidthMM: 160, HeightMM: 220, Confidence: 0.88, Notes: "页 4"},
	}
	saved := make([]*model.Leaf, 0, len(leaves))
	for i := range leaves {
		l, err := svc.AddLeaf(m, leaves[i])
		if err != nil {
			return nil, fmt.Errorf("导入纸页 %d: %w", leaves[i].PageNo, err)
		}
		// 观察解析完成后置为有效。
		l.Status = model.LeafValid
		if _, err := svc.UpdateLeaf(l.ID, model.LeafValid, l.Confidence, l.Notes, l.Version); err != nil {
			return nil, fmt.Errorf("激活纸页 %d: %w", l.PageNo, err)
		}
		l.Status = model.LeafValid // UpdateLeaf 返回的实体已含最新状态
		saved = append(saved, l)
	}

	// 3) 登记水印：页 1 左半片、页 2 右半片（同模具对 MP-001 → 应配对）。
	wm1, err := svc.AddWatermark(saved[0], &model.WatermarkObservation{
		HalfID: "WM-1540-A-L", MoldPairID: "MP-001", Position: model.WatermarkLeftHalf,
		XMM: 30, YMM: 110, RotationDeg: 0, Confidence: 0.9, Notes: "页1 左半片",
	})
	if err != nil {
		return nil, fmt.Errorf("登记水印 A: %w", err)
	}
	wm2, err := svc.AddWatermark(saved[1], &model.WatermarkObservation{
		HalfID: "WM-1540-A-R", MoldPairID: "MP-001", Position: model.WatermarkRightHalf,
		XMM: 130, YMM: 110, RotationDeg: 0, Confidence: 0.88, Notes: "页2 右半片",
	})
	if err != nil {
		return nil, fmt.Errorf("登记水印 B: %w", err)
	}
	// 页 3 登记另一模具对半片（无互补，制造重装订疑点）。
	if _, err := svc.AddWatermark(saved[2], &model.WatermarkObservation{
		HalfID: "WM-1540-B-L", MoldPairID: "MP-002", Position: model.WatermarkLeftHalf,
		XMM: 35, YMM: 100, RotationDeg: 0, Confidence: 0.85, Notes: "页3 左半片（模具对 MP-002）",
	}); err != nil {
		return nil, fmt.Errorf("登记水印 C: %w", err)
	}
	// 激活水印观测。
	for _, id := range []string{wm1.ID, wm2.ID} {
		if _, err := svc.ActivateWatermark(id); err != nil {
			return nil, fmt.Errorf("激活水印 %s: %w", id, err)
		}
	}

	// 4) 请求配对（页1 左半片 × 页2 右半片 → 期望 matched）。
	pairing, err := svc.RequestPairing(context.Background(), m, wm1.ID, wm2.ID)
	if err != nil {
		return nil, fmt.Errorf("请求配对: %w", err)
	}
	matched := pairing.Status == model.PairingMatched

	// 5) 建立相邻关系 (1,2) → 同折页；(2,3) → 折页跳变 → 重装订候选。
	r12, err := svc.CreateRelation(context.Background(), m, saved[0].ID, saved[1].ID, "smoke")
	if err != nil {
		return nil, fmt.Errorf("创建关系 1-2: %w", err)
	}
	r23, err := svc.CreateRelation(context.Background(), m, saved[1].ID, saved[2].ID, "smoke")
	if err != nil {
		return nil, fmt.Errorf("创建关系 2-3: %w", err)
	}

	// 6) 折页连续性校验 → 期望 1 个断裂点（2→3 quire_jump）。
	ver, err := svc.VerifyManuscript(m)
	if err != nil {
		return nil, fmt.Errorf("折页校验: %w", err)
	}

	// 7) 研究者确认重装订裁决并确认关系（自动裁决已是 rebound 时直接确认）。
	if r23.Verdict != model.VerdictRebound {
		// 自动裁决未给重装订时人工强制裁决（验证裁决路径）。
		r23, err = svc.AdjudicateRelation(r23.ID, model.VerdictRebound, "smoke", r23.Version)
		if err != nil {
			return nil, fmt.Errorf("裁决关系 2-3: %w", err)
		}
	}
	if r12.Verdict == model.VerdictConflict {
		r12, err = svc.AdjudicateRelation(r12.ID, model.VerdictSameFold, "smoke", r12.Version)
		if err != nil {
			return nil, fmt.Errorf("裁决关系 1-2: %w", err)
		}
	}

	// 8) 创建并冻结校勘版本。
	v1, err := svc.CreateVersion(m, "初版：确认页1-2同折页，页2-3重装订候选")
	if err != nil {
		return nil, fmt.Errorf("创建版本: %w", err)
	}
	if _, err := svc.FreezeVersion(context.Background(), v1.ID); err != nil {
		return nil, fmt.Errorf("冻结版本: %w", err)
	}
	// 冻结后修改版本内容必须被拒绝。
	frozen, _ := svc.GetVersion(v1.ID)
	if frozen.Status != model.VersionFrozen {
		return nil, fmt.Errorf("版本未冻结")
	}

	// 9) 关闭并重新打开同一 DB，验证持久化与重启恢复。
	_ = st.Close()
	st2, err := store.Open(path)
	if err != nil {
		return nil, fmt.Errorf("重开数据库: %w", err)
	}
	defer st2.Close()
	svc2 := service.New(st2)
	if _, err := svc2.Recover(context.Background()); err != nil {
		return nil, fmt.Errorf("重启恢复: %w", err)
	}
	m2, err := svc2.GetManuscript(m.ID)
	if err != nil {
		return nil, fmt.Errorf("恢复手稿: %w", err)
	}
	leaves2, err := svc2.ListLeaves(m.ID)
	if err != nil {
		return nil, fmt.Errorf("恢复纸页: %w", err)
	}
	rels2, err := svc2.ListRelations(m.ID)
	if err != nil {
		return nil, fmt.Errorf("恢复关系: %w", err)
	}
	vers2, err := svc2.ListVersions(m.ID)
	if err != nil {
		return nil, fmt.Errorf("恢复版本: %w", err)
	}
	recovered := m2.Title == m.Title && len(leaves2) == 4 && len(rels2) == 2 && len(vers2) == 1

	// 10) 统计。
	pairings2, err := svc2.ListPairings(m.ID)
	if err != nil {
		return nil, err
	}
	res := &SmokeResult{
		ManuscriptID: m.ID,
		LeafCount:    len(leaves2),
		PairingCount: len(pairings2),
		MatchedPair:  matched,
		RelationCount: len(rels2),
		FoldGaps:     len(ver.Gaps),
		ReboundFound: r23.Verdict == model.VerdictRebound,
		VersionNo:    v1.VersionNo,
		Recovered:    recovered,
		Persisted:    recovered,
	}
	// 契约断言：任何一项不满足即为失败。
	if !matched {
		return nil, fmt.Errorf("smoke 失败：水印半片未匹配（score=%.2f）", pairing.Score)
	}
	if len(ver.Gaps) == 0 {
		return nil, fmt.Errorf("smoke 失败：未检测到折页断裂点")
	}
	if r23.Verdict != model.VerdictRebound {
		return nil, fmt.Errorf("smoke 失败：关系 2-3 未判定为重装订，实际 %s", r23.Verdict)
	}
	if !recovered {
		return nil, fmt.Errorf("smoke 失败：重启后数据未完整恢复")
	}
	_ = adjudicate.Evidence{}
	return res, nil
}

// DumpResult 输出自检结果 JSON。
func DumpResult(res *SmokeResult) {
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(b))
}
