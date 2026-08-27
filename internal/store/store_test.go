package store

import (
	"testing"

	"task272-watermarkcollate/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("打开内存库: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestManuscriptLifecycle(t *testing.T) {
	st := newTestStore(t)
	m := &model.Manuscript{ID: "m1", Title: "残卷", Period: "c.1540", Status: model.ManuscriptOrganizing, Version: 1}
	if err := st.SaveManuscript(m); err != nil {
		t.Fatalf("保存手稿: %v", err)
	}
	got, err := st.GetManuscript("m1")
	if err != nil {
		t.Fatalf("读取手稿: %v", err)
	}
	if got.Title != "残卷" || got.Status != model.ManuscriptOrganizing {
		t.Fatalf("手稿数据错误: %+v", got)
	}
	// 正常更新：读取 → 修改 → 保存，版本推进到 2
	got.Status = model.ManuscriptCollating
	if err := st.SaveManuscript(got); err != nil {
		t.Fatalf("正常更新应成功: %v", err)
	}
	if got.Version != 2 {
		t.Fatalf("更新后版本应为 2，实际 %d", got.Version)
	}
	// 并发冲突：用过期版本 1 保存应报 VERSION_MISMATCH
	stale := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptAdjudicating, Version: 1}
	if err := st.SaveManuscript(stale); err == nil {
		t.Fatal("过期版本更新应报错")
	} else if de := model.AsDomainError(err); de == nil || de.Code != model.ErrVersionMismatch {
		t.Fatalf("应返回 VERSION_MISMATCH，实际 %v", err)
	}
}

func TestLeafUniquePage(t *testing.T) {
	st := newTestStore(t)
	m := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptOrganizing, Version: 1}
	_ = st.SaveManuscript(m)
	l1 := &model.Leaf{ID: "l1", ManuscriptID: "m1", PageNo: 1, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafPending, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	if err := st.SaveLeaf(l1); err != nil {
		t.Fatalf("保存纸页: %v", err)
	}
	// 同手稿同页码重复 → DUPLICATE_KEY
	l2 := &model.Leaf{ID: "l2", ManuscriptID: "m1", PageNo: 1, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafPending, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	if err := st.SaveLeaf(l2); err == nil {
		t.Fatal("重复页码应报错")
	} else if de := model.AsDomainError(err); de == nil || de.Code != model.ErrDuplicateKey {
		t.Fatalf("应返回 DUPLICATE_KEY，实际 %v", err)
	}
}

func TestLeafStateTransition(t *testing.T) {
	st := newTestStore(t)
	m := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptOrganizing, Version: 1}
	_ = st.SaveManuscript(m)
	l := &model.Leaf{ID: "l1", ManuscriptID: "m1", PageNo: 1, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafPending, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	_ = st.SaveLeaf(l)
	// pending → valid 合法
	l.Status = model.LeafValid
	if err := st.SaveLeaf(l); err != nil {
		t.Fatalf("pending→valid 应允许: %v", err)
	}
	// excluded → valid 非法
	l.Status = model.LeafExcluded
	if err := st.SaveLeaf(l); err != nil {
		t.Fatalf("valid→excluded 应允许: %v", err)
	}
	if err := st.SaveLeaf(l); err == nil {
		t.Fatal("excluded→excluded 应报错（终态不可迁移）")
	}
}

func TestWatermarkUniqueHalf(t *testing.T) {
	st := newTestStore(t)
	m := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptOrganizing, Version: 1}
	_ = st.SaveManuscript(m)
	l := &model.Leaf{ID: "l1", ManuscriptID: "m1", PageNo: 1, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafValid, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	_ = st.SaveLeaf(l)
	w1 := &model.WatermarkObservation{ID: "w1", LeafID: "l1", HalfID: "A-L", MoldPairID: "MP-001",
		Position: model.WatermarkLeftHalf, XMM: 30, YMM: 110, Confidence: 0.9, Status: model.WatermarkPending}
	if err := st.SaveWatermark(w1); err != nil {
		t.Fatalf("保存水印: %v", err)
	}
	w2 := &model.WatermarkObservation{ID: "w2", LeafID: "l1", HalfID: "A-L", MoldPairID: "MP-001",
		Position: model.WatermarkLeftHalf, XMM: 30, YMM: 110, Confidence: 0.9, Status: model.WatermarkPending}
	if err := st.SaveWatermark(w2); err == nil {
		t.Fatal("同纸页同半片重复应报错")
	}
}

func TestPersistenceReopen(t *testing.T) {
	path := t.TempDir() + "/test.db"
	st, err := Open(path)
	if err != nil {
		t.Fatalf("打开: %v", err)
	}
	m := &model.Manuscript{ID: "m1", Title: "持久化验证", Status: model.ManuscriptOrganizing, Version: 1}
	if err := st.SaveManuscript(m); err != nil {
		t.Fatalf("保存: %v", err)
	}
	_ = st.Close()
	st2, err := Open(path)
	if err != nil {
		t.Fatalf("重开: %v", err)
	}
	defer st2.Close()
	got, err := st2.GetManuscript("m1")
	if err != nil {
		t.Fatalf("重开后读取: %v", err)
	}
	if got.Title != "持久化验证" {
		t.Fatalf("重启后数据丢失: %+v", got)
	}
}

func TestRelationUniquePair(t *testing.T) {
	st := newTestStore(t)
	m := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptOrganizing, Version: 1}
	_ = st.SaveManuscript(m)
	l1 := &model.Leaf{ID: "l1", ManuscriptID: "m1", PageNo: 1, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafValid, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	l2 := &model.Leaf{ID: "l2", ManuscriptID: "m1", PageNo: 2, QuireNo: 1, Position: model.PositionVerso,
		Status: model.LeafValid, BindingEdge: model.EdgeRight, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	_ = st.SaveLeaf(l1)
	_ = st.SaveLeaf(l2)
	r1 := &model.LeafRelation{ID: "r1", ManuscriptID: "m1", LeftLeafID: "l1", RightLeafID: "l2", PageDelta: 1,
		Verdict: model.VerdictCandidate, Version: 1}
	if err := st.SaveRelation(r1); err != nil {
		t.Fatalf("保存关系: %v", err)
	}
	r2 := &model.LeafRelation{ID: "r2", ManuscriptID: "m1", LeftLeafID: "l1", RightLeafID: "l2", PageDelta: 1,
		Verdict: model.VerdictCandidate, Version: 1}
	if err := st.SaveRelation(r2); err == nil {
		t.Fatal("重复关系对应报错")
	}
}

// TestRelationGapReasonsIndependent 验证每条关系的 GapReasons 是独立副本：
// 列出多条关系后，改其中一条的断裂原因，其余关系不得跟着变。
func TestRelationGapReasonsIndependent(t *testing.T) {
	st := newTestStore(t)
	m := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptOrganizing, Version: 1}
	_ = st.SaveManuscript(m)
	// 三页：l1/l2 同折页连续、l2/l3 折页跳变、l1/l3 页码跳变，给出不同的断裂原因。
	l1 := &model.Leaf{ID: "l1", ManuscriptID: "m1", PageNo: 1, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafValid, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	l2 := &model.Leaf{ID: "l2", ManuscriptID: "m1", PageNo: 2, QuireNo: 2, Position: model.PositionVerso,
		Status: model.LeafValid, BindingEdge: model.EdgeRight, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	l3 := &model.Leaf{ID: "l3", ManuscriptID: "m1", PageNo: 4, QuireNo: 2, Position: model.PositionRecto,
		Status: model.LeafValid, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	_ = st.SaveLeaf(l1)
	_ = st.SaveLeaf(l2)
	_ = st.SaveLeaf(l3)

	rels := []*model.LeafRelation{
		{ID: "r12", ManuscriptID: "m1", LeftLeafID: "l1", RightLeafID: "l2", PageDelta: 1,
			GapReasons: []string{"quire_jump"}, Verdict: model.VerdictCandidate, Version: 1},
		{ID: "r23", ManuscriptID: "m1", LeftLeafID: "l2", RightLeafID: "l3", PageDelta: 2,
			GapReasons: []string{"page_gap"}, Verdict: model.VerdictCandidate, Version: 1},
	}
	for _, r := range rels {
		if err := st.SaveRelation(r); err != nil {
			t.Fatalf("保存关系 %s: %v", r.ID, err)
		}
	}

	got, err := st.ListRelations("m1")
	if err != nil {
		t.Fatalf("列出关系: %v", err)
	}
	// 找到 r12 与 r23，记下各自的断裂原因。
	var r12, r23 *model.LeafRelation
	for _, r := range got {
		switch r.ID {
		case "r12":
			r12 = r
		case "r23":
			r23 = r
		}
	}
	if r12 == nil || r23 == nil {
		t.Fatalf("应列出 r12 与 r23，实际 %+v", got)
	}
	if len(r12.GapReasons) != 1 || r12.GapReasons[0] != "quire_jump" {
		t.Fatalf("r12 断裂原因应为 [quire_jump]，实际 %v", r12.GapReasons)
	}
	if len(r23.GapReasons) != 1 || r23.GapReasons[0] != "page_gap" {
		t.Fatalf("r23 断裂原因应为 [page_gap]，实际 %v", r23.GapReasons)
	}

	// 改其中一条的断裂原因，另一条不得跟着变。
	r12.GapReasons[0] = "page_gap"
	if r23.GapReasons[0] != "page_gap" {
		t.Fatalf("改 r12 的断裂原因后 r23 被牵连：%v（应为 page_gap 不变）", r23.GapReasons)
	}
	// 追加也应只影响自身。
	r12.GapReasons = append(r12.GapReasons, "quire_jump")
	if len(r23.GapReasons) != 1 {
		t.Fatalf("追加 r12 后 r23 长度被牵连：%v", r23.GapReasons)
	}

	// 重新读取，确认落库值仍独立。
	got2, err := st.ListRelations("m1")
	if err != nil {
		t.Fatalf("重新列出关系: %v", err)
	}
	for _, r := range got2 {
		switch r.ID {
		case "r12":
			if len(r.GapReasons) != 1 || r.GapReasons[0] != "quire_jump" {
				t.Fatalf("r12 落库断裂原因应为 [quire_jump]，实际 %v", r.GapReasons)
			}
		case "r23":
			if len(r.GapReasons) != 1 || r.GapReasons[0] != "page_gap" {
				t.Fatalf("r23 落库断裂原因应为 [page_gap]，实际 %v", r.GapReasons)
			}
		}
	}
}

func TestVersionUniqueNo(t *testing.T) {
	st := newTestStore(t)
	m := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptOrganizing, Version: 1}
	_ = st.SaveManuscript(m)
	v1 := &model.CollationVersion{ID: "v1", ManuscriptID: "m1", VersionNo: 1, Status: model.VersionDraft}
	if err := st.SaveVersion(v1); err != nil {
		t.Fatalf("保存版本: %v", err)
	}
	v2 := &model.CollationVersion{ID: "v2", ManuscriptID: "m1", VersionNo: 1, Status: model.VersionDraft}
	if err := st.SaveVersion(v2); err == nil {
		t.Fatal("同手稿同版本号重复应报错")
	}
	no, err := st.NextVersionNo("m1")
	if err != nil || no != 2 {
		t.Fatalf("下一版本号应为 2，实际 %d (%v)", no, err)
	}
}
