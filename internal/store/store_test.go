package store

import (
	"fmt"
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

// TestConcurrentLeafWrites 模拟多位研究者同时往同一份手稿登记不同页码的纸页：
// 并发写入必须全部成功落库、不互相踩踏事务、不因写锁竞争而失败。
// 回归保护：WithTx 必须把整个 Begin/fn/Commit 罩在同一把锁内。
func TestConcurrentLeafWrites(t *testing.T) {
	st := newTestStore(t)
	m := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptOrganizing, Version: 1}
	if err := st.SaveManuscript(m); err != nil {
		t.Fatalf("保存手稿: %v", err)
	}

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start // 同时起跑，最大化并发冲突窗口
			l := &model.Leaf{
				ID:          genTestID(i),
				ManuscriptID: "m1",
				PageNo:      i + 1,
				QuireNo:     1,
				Position:    model.PositionRecto,
				Status:      model.LeafPending,
				BindingEdge: model.EdgeLeft,
				ChainDeg:    90,
				WidthMM:     160,
				HeightMM:    220,
				Confidence:  0.9,
				Version:     1,
			}
			errs[i] = st.SaveLeaf(l)
		}()
	}
	close(start)
	wg.Wait()

	succ := 0
	var firstErr error
	for _, err := range errs {
		if err == nil {
			succ++
		} else if firstErr == nil {
			firstErr = err
		}
	}
	if succ != n {
		t.Fatalf("并发登记应全部成功（n=%d），实际成功 %d；首个错误: %v", n, succ, firstErr)
	}
	// 落库页码必须无重复无丢失。
	leaves, err := st.ListLeaves("m1")
	if err != nil {
		t.Fatalf("列出纸页: %v", err)
	}
	seen := make(map[int]bool, n)
	for _, l := range leaves {
		if seen[l.PageNo] {
			t.Fatalf("页码 %d 重复落库", l.PageNo)
		}
		seen[l.PageNo] = true
	}
	if len(seen) != n {
		t.Fatalf("应落库 %d 页，实际 %d", n, len(seen))
	}
}

// TestConcurrentOptimisticLock 验证乐观锁在并发更新下不丢失更新：
// 同一纸页被多个 goroutine 基于同一版本并发推进状态，至多一个成功，其余须报 VERSION_MISMATCH，
// 且最终版本号恰好等于「成功次数 + 1」——任何丢失更新都会使版本号偏低。
func TestConcurrentOptimisticLock(t *testing.T) {
	st := newTestStore(t)
	m := &model.Manuscript{ID: "m1", Title: "残卷", Status: model.ManuscriptOrganizing, Version: 1}
	_ = st.SaveManuscript(m)
	l := &model.Leaf{ID: "l1", ManuscriptID: "m1", PageNo: 1, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafPending, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	if err := st.SaveLeaf(l); err != nil {
		t.Fatalf("保存纸页: %v", err)
	}

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})
	results := make([]bool, 0, n)
	var mu sync.Mutex
	for range n {
		go func() {
			defer wg.Done()
			<-start
			// 每个 goroutine 基于基线版本 1 尝试 pending→valid。
			clone := *l
			clone.Status = model.LeafValid
			err := st.SaveLeaf(&clone)
			succ := err == nil
			mu.Lock()
			results = append(results, succ)
			mu.Unlock()
			if err != nil && model.AsDomainError(err) == nil {
				t.Errorf("并发更新返回非领域错误: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	succ := 0
	for _, ok := range results {
		if ok {
			succ++
		}
	}
	if succ != 1 {
		t.Fatalf("同一基线版本的并发推进应恰好一个成功，实际 %d", succ)
	}
	got, err := st.GetLeaf("l1")
	if err != nil {
		t.Fatalf("回读纸页: %v", err)
	}
	if got.Version != 2 {
		t.Fatalf("成功一次后版本应为 2，实际 %d（怀疑丢失更新）", got.Version)
	}
}

func genTestID(i int) string {
	// 确定且全局唯一：左补零到 16 位十进制串，再拼固定前缀。
	// 不依赖随机数，避免不同 i 意外碰撞后落到 UPDATE 分支静默覆盖。
	s := fmt.Sprintf("%016d", i)
	return "leaf" + s
}
