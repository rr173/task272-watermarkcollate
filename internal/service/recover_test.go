package service

import (
	"context"
	"testing"

	"task272-watermarkcollate/internal/model"
	"task272-watermarkcollate/internal/store"
)

// newRecoverStore 建一个内存库并装入手稿 + 两张有效纸页。
func newRecoverStore(t *testing.T) (*store.Store, *model.Manuscript, *model.Leaf, *model.Leaf) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("打开内存库: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	m := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptCollating, Version: 1}
	if err := st.SaveManuscript(m); err != nil {
		t.Fatalf("保存手稿: %v", err)
	}
	l1 := &model.Leaf{ID: "l1", ManuscriptID: "m1", PageNo: 1, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafValid, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	l2 := &model.Leaf{ID: "l2", ManuscriptID: "m1", PageNo: 2, QuireNo: 1, Position: model.PositionVerso,
		Status: model.LeafValid, BindingEdge: model.EdgeRight, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	if err := st.SaveLeaf(l1); err != nil {
		t.Fatalf("保存纸页 1: %v", err)
	}
	if err := st.SaveLeaf(l2); err != nil {
		t.Fatalf("保存纸页 2: %v", err)
	}
	return st, m, l1, l2
}

// addComplementaryWatermarks 登记并激活一对互补水印半片（同模具对、左右互补）。
// 注意：不创建任何配对记录——模拟「手稿里已有一对互补且有效的水印半片但缺配对」的恢复场景。
func addComplementaryWatermarks(t *testing.T, st *store.Store, l1, l2 *model.Leaf) (string, string) {
	t.Helper()
	wa := &model.WatermarkObservation{ID: "wa", LeafID: l1.ID, HalfID: "A-L", MoldPairID: "MP-001",
		Position: model.WatermarkLeftHalf, XMM: 30, YMM: 110, Confidence: 0.9, Status: model.WatermarkValid}
	wb := &model.WatermarkObservation{ID: "wb", LeafID: l2.ID, HalfID: "A-R", MoldPairID: "MP-001",
		Position: model.WatermarkRightHalf, XMM: 130, YMM: 110, Confidence: 0.88, Status: model.WatermarkValid}
	if err := st.SaveWatermark(wa); err != nil {
		t.Fatalf("保存水印 A: %v", err)
	}
	if err := st.SaveWatermark(wb); err != nil {
		t.Fatalf("保存水印 B: %v", err)
	}
	return wa.ID, wb.ID
}

// TestRecoverBackfillsMissingPairing 重启恢复必须为「有效互补半片且无配对」补齐配对候选。
// 复现原缺陷：FindPairingByWatermarks 在无配对时返回 NOT_FOUND，旧代码却对 err!=nil continue，
// 导致该补齐的配对被跳过、Recover 仍误报成功。
func TestRecoverBackfillsMissingPairing(t *testing.T) {
	st, m, l1, l2 := newRecoverStore(t)
	addComplementaryWatermarks(t, st, l1, l2)

	// 恢复前不应存在任何配对。
	if before, err := st.ListPairings(m.ID); err != nil {
		t.Fatalf("恢复前列出配对: %v", err)
	} else if len(before) != 0 {
		t.Fatalf("恢复前不应有配对，实际 %d 条", len(before))
	}

	svc := New(st)
	rep, err := svc.Recover(context.Background())
	if err != nil {
		t.Fatalf("重启恢复不应失败: %v", err)
	}
	if rep.PairingsCreated != 1 {
		t.Fatalf("应补齐 1 条配对候选，实际 %d", rep.PairingsCreated)
	}

	// 恢复后该对半片必须有配对记录，且状态至少为 candidate。
	after, err := st.ListPairings(m.ID)
	if err != nil {
		t.Fatalf("恢复后列出配对: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("恢复后应有 1 条配对，实际 %d", len(after))
	}
	p := after[0]
	if p.Status != model.PairingMatched && p.Status != model.PairingCandidate {
		t.Fatalf("补齐的配对应为 matched/candidate，实际 %s", p.Status)
	}
}

// TestRecoverIdempotentDoesNotRecreate 既存配对不被重复创建（幂等）。
func TestRecoverIdempotentDoesNotRecreate(t *testing.T) {
	st, m, l1, l2 := newRecoverStore(t)
	aID, bID := addComplementaryWatermarks(t, st, l1, l2)

	// 先请求一次配对（走正常路径创建记录）。
	svc := New(st)
	ctx := context.Background()
	if _, err := svc.RequestPairing(ctx, m, aID, bID); err != nil {
		t.Fatalf("预建配对: %v", err)
	}

	// 再恢复：不应重复创建。
	rep, err := svc.Recover(ctx)
	if err != nil {
		t.Fatalf("二次恢复不应失败: %v", err)
	}
	if rep.PairingsCreated != 0 {
		t.Fatalf("既存配对应跳过，实际又建了 %d 条", rep.PairingsCreated)
	}
	after, _ := st.ListPairings(m.ID)
	if len(after) != 1 {
		t.Fatalf("幂等恢复后仍应只有 1 条配对，实际 %d", len(after))
	}
}

// TestRecoverNoCandidatesReportsNothingButSuccess 无可配对半片时恢复成功且补齐 0 条。
func TestRecoverNoCandidatesReportsNothingButSuccess(t *testing.T) {
	st, _, l1, l2 := newRecoverStore(t)
	// 两个左半片（同模具对）——位置不互补，无配对候选。
	wa := &model.WatermarkObservation{ID: "wa", LeafID: l1.ID, HalfID: "A-L", MoldPairID: "MP-001",
		Position: model.WatermarkLeftHalf, XMM: 30, YMM: 110, Confidence: 0.9, Status: model.WatermarkValid}
	wb := &model.WatermarkObservation{ID: "wb", LeafID: l2.ID, HalfID: "A-L2", MoldPairID: "MP-001",
		Position: model.WatermarkLeftHalf, XMM: 130, YMM: 110, Confidence: 0.9, Status: model.WatermarkValid}
	_ = st.SaveWatermark(wa)
	_ = st.SaveWatermark(wb)

	svc := New(st)
	rep, err := svc.Recover(context.Background())
	if err != nil {
		t.Fatalf("无候选时恢复应成功: %v", err)
	}
	if rep.PairingsCreated != 0 {
		t.Fatalf("无可配对半片应补齐 0 条，实际 %d", rep.PairingsCreated)
	}
}
