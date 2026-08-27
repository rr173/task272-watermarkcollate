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

// TestLeafDuplicateThenValid 复现：第一次导入因页码重复失败后，紧接着再导入另一页合法纸页也会失败。
// 根因是 WithTx 失败路径未回滚事务，遗留的 *sql.Tx 占满了 SetMaxOpenConns(1) 的唯一连接，
// 致使下一次 Begin 取不到连接而失败。修复后失败写入必须回滚、归还连接。
func TestLeafDuplicateThenValid(t *testing.T) {
	st := newTestStore(t)
	m := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptOrganizing, Version: 1}
	if err := st.SaveManuscript(m); err != nil {
		t.Fatalf("保存手稿: %v", err)
	}
	l1 := &model.Leaf{ID: "l1", ManuscriptID: "m1", PageNo: 1, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafPending, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	if err := st.SaveLeaf(l1); err != nil {
		t.Fatalf("首次保存纸页: %v", err)
	}
	// 同页码重复导入 → 必须失败回滚，不得泄漏连接。
	dup := &model.Leaf{ID: "l2", ManuscriptID: "m1", PageNo: 1, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafPending, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	if err := st.SaveLeaf(dup); err == nil {
		t.Fatal("重复页码导入应失败")
	}
	// 紧接着导入另一页合法纸页 → 修复后必须成功。
	l3 := &model.Leaf{ID: "l3", ManuscriptID: "m1", PageNo: 2, QuireNo: 1, Position: model.PositionVerso,
		Status: model.LeafPending, BindingEdge: model.EdgeRight, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	if err := st.SaveLeaf(l3); err != nil {
		t.Fatalf("重复导入后合法导入应成功，实际失败: %v", err)
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
