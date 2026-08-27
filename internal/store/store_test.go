package store

import (
	"sync"
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

// TestListLeavesReflectsLatestStatus 回归：纸页状态落库后 ListLeaves 必须立即读到新状态，
// 不能复用落库前的过期缓存。曾存在按手稿缓存纸页列表的实现，pending→valid 迁移后
// 仍返回旧状态，导致折页连续性校验把刚生效的纸页当成未参与而误报断裂。
func TestListLeavesReflectsLatestStatus(t *testing.T) {
	st := newTestStore(t)
	m := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptOrganizing, Version: 1}
	_ = st.SaveManuscript(m)
	l := &model.Leaf{ID: "l1", ManuscriptID: "m1", PageNo: 1, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafPending, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	if err := st.SaveLeaf(l); err != nil {
		t.Fatalf("保存纸页: %v", err)
	}
	// 预热：首次读取把列表「读入缓存」（若有缓存）。
	got, err := st.ListLeaves("m1")
	if err != nil {
		t.Fatalf("首次列出纸页: %v", err)
	}
	if len(got) != 1 || got[0].Status != model.LeafPending {
		t.Fatalf("首次读取状态错误: %+v", got)
	}
	// 状态迁移落库：pending → valid。
	l.Status = model.LeafValid
	if err := st.SaveLeaf(l); err != nil {
		t.Fatalf("pending→valid 应允许: %v", err)
	}
	// 再次列出必须读到最新落库状态 valid，而非过期缓存里的 pending。
	got, err = st.ListLeaves("m1")
	if err != nil {
		t.Fatalf("迁移后列出纸页: %v", err)
	}
	if len(got) != 1 || got[0].Status != model.LeafValid || got[0].Version != 2 {
		t.Fatalf("ListLeaves 未反映最新落库状态，可能复用了过期缓存: %+v", got)
	}
}

// TestListLeavesConcurrentNoCorruption 回归：并发改状态 + 列表读取不应错乱。
// 曾存在无锁 map 缓存，与写入 SaveLeaf 并发时触发「concurrent map read and map write」
// 致列表错乱或崩溃；移除缓存后每次都直读数据库，读写经 SetMaxOpenConns(1) 串行化。
func TestListLeavesConcurrentNoCorruption(t *testing.T) {
	st := newTestStore(t)
	m := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptOrganizing, Version: 1}
	_ = st.SaveManuscript(m)
	leaf := &model.Leaf{ID: "l1", ManuscriptID: "m1", PageNo: 1, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafPending, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	if err := st.SaveLeaf(leaf); err != nil {
		t.Fatalf("保存纸页: %v", err)
	}

	var wg sync.WaitGroup
	done := make(chan struct{})
	// 读取方：持续列出纸页，迁移期间每份列表必须自洽（非空且单页）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			got, err := st.ListLeaves("m1")
			if err != nil {
				t.Errorf("并发列出纸页失败: %v", err)
				return
			}
			if len(got) != 1 {
				t.Errorf("并发列表长度错乱: 期望 1，得到 %d", len(got))
				return
			}
		}
	}()
	// 写入方：在 valid / pending 之间反复迁移（valid→damaged→valid 合法链）。
	cur := leaf
	for i := 0; i < 50; i++ {
		next := model.LeafDamaged
		if i%2 == 1 {
			next = model.LeafValid
		}
		if !cur.Status.CanTransitionTo(next) {
			t.Fatalf("非法迁移 %s→%s", cur.Status, next)
		}
		cur.Status = next
		if err := st.SaveLeaf(cur); err != nil {
			t.Errorf("并发迁移落库失败: %v", err)
			break
		}
	}
	close(done)
	wg.Wait()
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
