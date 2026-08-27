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

// TestFrozenVersionSnapshotImmutable 回归测试：
// 已冻结版本的关系快照不可改写——同一 frozen 状态下再次 SaveVersion 携带不同
// content_json 必须被拒绝（FROZEN），保证冻结快照在存储层即被锁定。
func TestFrozenVersionSnapshotImmutable(t *testing.T) {
	st := newTestStore(t)
	m := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptOrganizing, Version: 1}
	_ = st.SaveManuscript(m)
	// 创建并冻结版本：快照为 []。
	v := &model.CollationVersion{ID: "v1", ManuscriptID: "m1", VersionNo: 1, Status: model.VersionShared, ContentJSON: "[]"}
	if err := st.SaveVersion(v); err != nil {
		t.Fatalf("保存草稿版本: %v", err)
	}
	v.Status = model.VersionFrozen
	if err := st.SaveVersion(v); err != nil {
		t.Fatalf("冻结版本: %v", err)
	}
	// 再次保存但篡改 content_json → 必须报 FROZEN。
	tampered := *v
	tampered.ContentJSON = `[{"verdict":"tampered"}]`
	if err := st.SaveVersion(&tampered); err == nil {
		t.Fatal("篡改冻结快照应被拒绝")
	} else if de := model.AsDomainError(err); de == nil || de.Code != model.ErrFrozen {
		t.Fatalf("应返回 FROZEN，实际 %v", err)
	}
	// 落库的快照应仍是原值 []。
	got, err := st.GetVersion("v1")
	if err != nil {
		t.Fatalf("读取冻结版本: %v", err)
	}
	if got.ContentJSON != "[]" {
		t.Fatalf("冻结快照被篡改：期望 []，实际 %s", got.ContentJSON)
	}
}

// TestSupersedeKeepsFrozenSnapshot 回归测试：
// 已冻结版本被 supersede 时不得改写快照——状态迁移允许但 content_json 必须保持不变。
func TestSupersedeKeepsFrozenSnapshot(t *testing.T) {
	st := newTestStore(t)
	m := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptOrganizing, Version: 1}
	_ = st.SaveManuscript(m)
	v := &model.CollationVersion{ID: "v1", ManuscriptID: "m1", VersionNo: 1, Status: model.VersionFrozen, ContentJSON: "[]"}
	if err := st.SaveVersion(v); err != nil {
		t.Fatalf("保存冻结版本: %v", err)
	}
	// supersede：状态迁移 frozen → superseded，content_json 未变，应成功且快照保持。
	v.Status = model.VersionSuperseded
	if err := st.SaveVersion(v); err != nil {
		t.Fatalf("替代冻结版本应允许状态迁移: %v", err)
	}
	got, err := st.GetVersion("v1")
	if err != nil {
		t.Fatalf("读取替代版本: %v", err)
	}
	if got.Status != model.VersionSuperseded {
		t.Fatalf("状态应为 superseded，实际 %s", got.Status)
	}
	if got.ContentJSON != "[]" {
		t.Fatalf("替代不应改写快照：期望 []，实际 %s", got.ContentJSON)
	}
}
