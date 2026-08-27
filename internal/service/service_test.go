package service

import (
	"context"
	"encoding/json"
	"testing"

	"task272-watermarkcollate/internal/model"
	"task272-watermarkcollate/internal/store"
)

// 建立一份带两页纸页与一条相邻关系的手稿，便于回归测试复用。
func setupFrozenVersionFixture(t *testing.T) (*Service, *model.Manuscript, *model.LeafRelation, *model.CollationVersion) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("打开内存库: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := New(st)

	m, err := svc.CreateManuscript("回归手稿", "c.1540", "")
	if err != nil {
		t.Fatalf("创建手稿: %v", err)
	}
	// 两页跨折页（页 1 折页 1、页 2 折页 2）且链线显著不一致 → 关系自动裁决为重装订（rebound），
	// 以便后续可经 confirm 迁移到终态 confirmed，用于验证冻结快照不被改写。
	l1, err := svc.AddLeaf(m, &model.Leaf{
		PageNo: 1, QuireNo: 1, Position: model.PositionRecto, BindingEdge: model.EdgeLeft,
		ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.95,
	})
	if err != nil {
		t.Fatalf("导入纸页 1: %v", err)
	}
	if _, err := svc.UpdateLeaf(l1.ID, model.LeafValid, l1.Confidence, l1.Notes, l1.Version); err != nil {
		t.Fatalf("激活纸页 1: %v", err)
	}
	l2, err := svc.AddLeaf(m, &model.Leaf{
		PageNo: 2, QuireNo: 2, Position: model.PositionVerso, BindingEdge: model.EdgeRight,
		ChainDeg: 45, WidthMM: 160, HeightMM: 220, Confidence: 0.92,
	})
	if err != nil {
		t.Fatalf("导入纸页 2: %v", err)
	}
	if _, err := svc.UpdateLeaf(l2.ID, model.LeafValid, l2.Confidence, l2.Notes, l2.Version); err != nil {
		t.Fatalf("激活纸页 2: %v", err)
	}

	rel, err := svc.CreateRelation(context.Background(), m, l1.ID, l2.ID, "tester")
	if err != nil {
		t.Fatalf("建立关系: %v", err)
	}
	// 自动裁决应为重装订（rebound）；若未给重装订则强制裁决，保证后续可迁移到终态。
	if rel.Verdict != model.VerdictRebound {
		rel, err = svc.AdjudicateRelation(rel.ID, model.VerdictRebound, "tester", rel.Version)
		if err != nil {
			t.Fatalf("裁决重装订: %v", err)
		}
	}
	if rel.Verdict != model.VerdictRebound {
		t.Fatalf("裁决应为重装订，实际 %s", rel.Verdict)
	}

	// 创建并冻结校勘版本 → 快照固化为当时的裁决（rebound）。
	v, err := svc.CreateVersion(m, "冻结快照")
	if err != nil {
		t.Fatalf("创建版本: %v", err)
	}
	v, err = svc.FreezeVersion(context.Background(), v.ID)
	if err != nil {
		t.Fatalf("冻结版本: %v", err)
	}
	return svc, m, rel, v
}

// decodeSnapshotContent 解析版本快照为关系切片。
func decodeSnapshotContent(t *testing.T, content string) []model.LeafRelation {
	t.Helper()
	var rels []model.LeafRelation
	if err := json.Unmarshal([]byte(content), &rels); err != nil {
		t.Fatalf("解析关系快照: %v", err)
	}
	return rels
}

// TestConfirmRelationDoesNotRewriteFrozenSnapshot 回归测试：
// 确认关系（rebound → confirmed 终态迁移）不得回写已冻结版本的关系快照，
// 且重新读取冻结版本时快照必须保持冻结当时的裁决（rebound），
// 不得被当前关系覆盖（曾出现的两个 bug：ConfirmRelation 重写快照 + GetVersion 重新生成快照）。
func TestConfirmRelationDoesNotRewriteFrozenSnapshot(t *testing.T) {
	svc, m, rel, v := setupFrozenVersionFixture(t)
	_ = m

	// 记录冻结当时的快照裁决（rebound）。
	frozen0, err := svc.GetVersion(v.ID)
	if err != nil {
		t.Fatalf("读取冻结版本: %v", err)
	}
	before := decodeSnapshotContent(t, frozen0.ContentJSON)
	if len(before) != 1 || before[0].Verdict != rel.Verdict {
		t.Fatalf("冻结快照应记录冻结当时的裁决 %s，实际 %+v", rel.Verdict, before)
	}

	// 研究者确认关系：rebound → confirmed（合法终态迁移）。
	changed, err := svc.ConfirmRelation(rel.ID, "tester", rel.Version)
	if err != nil {
		t.Fatalf("确认关系: %v", err)
	}
	if changed.Verdict != model.VerdictConfirmed {
		t.Fatalf("裁决应已确认，实际 %s", changed.Verdict)
	}

	// 重新读取冻结版本：快照必须仍是冻结当时的裁决，不得跟随当前关系变化。
	frozen1, err := svc.GetVersion(v.ID)
	if err != nil {
		t.Fatalf("重读冻结版本: %v", err)
	}
	if frozen1.ContentJSON != frozen0.ContentJSON {
		t.Fatalf("冻结版本快照被篡改：冻结时=%s, 重读时=%s", frozen0.ContentJSON, frozen1.ContentJSON)
	}
	after := decodeSnapshotContent(t, frozen1.ContentJSON)
	if len(after) != 1 || after[0].Verdict != rel.Verdict {
		t.Fatalf("冻结快照应保持冻结当时裁决 %s，实际 %+v", rel.Verdict, after)
	}
	if after[0].Verdict == model.VerdictConfirmed {
		t.Fatalf("冻结快照泄漏了冻结后的裁决变更（应为 %s，实为 %s）", rel.Verdict, after[0].Verdict)
	}
}

// TestFreezeVersionPersistsAcrossReopen 回归测试：
// 冻结快照落库后，关闭并重开数据库，冻结版本读回的快照应与冻结当时一致，
// 不因重启而丢失或被重新生成。
func TestFreezeVersionPersistsAcrossReopen(t *testing.T) {
	path := t.TempDir() + "/frozen.db"
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("打开库: %v", err)
	}
	svc := New(st)
	m, err := svc.CreateManuscript("重启手稿", "c.1540", "")
	if err != nil {
		t.Fatalf("创建手稿: %v", err)
	}
	l1, err := svc.AddLeaf(m, &model.Leaf{
		PageNo: 1, QuireNo: 1, Position: model.PositionRecto, BindingEdge: model.EdgeLeft,
		ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.95,
	})
	if err != nil {
		t.Fatalf("导入纸页 1: %v", err)
	}
	if _, err := svc.UpdateLeaf(l1.ID, model.LeafValid, l1.Confidence, l1.Notes, l1.Version); err != nil {
		t.Fatalf("激活纸页 1: %v", err)
	}
	l2, err := svc.AddLeaf(m, &model.Leaf{
		PageNo: 2, QuireNo: 2, Position: model.PositionVerso, BindingEdge: model.EdgeRight,
		ChainDeg: 45, WidthMM: 160, HeightMM: 220, Confidence: 0.92,
	})
	if err != nil {
		t.Fatalf("导入纸页 2: %v", err)
	}
	if _, err := svc.UpdateLeaf(l2.ID, model.LeafValid, l2.Confidence, l2.Notes, l2.Version); err != nil {
		t.Fatalf("激活纸页 2: %v", err)
	}
	rel, err := svc.CreateRelation(context.Background(), m, l1.ID, l2.ID, "tester")
	if err != nil {
		t.Fatalf("建立关系: %v", err)
	}
	// 自动裁决应为重装订（rebound）；若未给重装订则强制裁决，保证后续可迁移到终态。
	if rel.Verdict != model.VerdictRebound {
		rel, err = svc.AdjudicateRelation(rel.ID, model.VerdictRebound, "tester", rel.Version)
		if err != nil {
			t.Fatalf("裁决重装订: %v", err)
		}
	}
	v, err := svc.CreateVersion(m, "重启冻结")
	if err != nil {
		t.Fatalf("创建版本: %v", err)
	}
	v, err = svc.FreezeVersion(context.Background(), v.ID)
	if err != nil {
		t.Fatalf("冻结版本: %v", err)
	}
	frozenAtFreeze, err := svc.GetVersion(v.ID)
	if err != nil {
		t.Fatalf("读取冻结版本: %v", err)
	}

	// 确认关系（rebound → confirmed 终态迁移），再关闭重开，确认冻结快照不变。
	if _, err := svc.ConfirmRelation(rel.ID, "tester", rel.Version); err != nil {
		t.Fatalf("确认关系: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("关闭库: %v", err)
	}
	st2, err := store.Open(path)
	if err != nil {
		t.Fatalf("重开库: %v", err)
	}
	defer st2.Close()
	svc2 := New(st2)
	if _, err := svc2.Recover(context.Background()); err != nil {
		t.Fatalf("重启恢复: %v", err)
	}
	frozenAfterReopen, err := svc2.GetVersion(v.ID)
	if err != nil {
		t.Fatalf("重开后读取冻结版本: %v", err)
	}
	if frozenAfterReopen.ContentJSON != frozenAtFreeze.ContentJSON {
		t.Fatalf("重启后冻结快照变化：冻结时=%s, 重开后=%s", frozenAtFreeze.ContentJSON, frozenAfterReopen.ContentJSON)
	}
	after := decodeSnapshotContent(t, frozenAfterReopen.ContentJSON)
	if len(after) != 1 || after[0].Verdict == model.VerdictConfirmed {
		t.Fatalf("重启后冻结快照泄漏了确认后的终态裁决，实际 %+v", after)
	}
}
