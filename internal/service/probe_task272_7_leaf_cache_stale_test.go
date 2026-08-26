package service

import (
	"path/filepath"
	"sync"
	"testing"

	"task272-watermarkcollate/internal/model"
	"task272-watermarkcollate/internal/store"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(st)
}

func mustManuscript(t *testing.T, svc *Service) *model.Manuscript {
	t.Helper()
	m, err := svc.CreateManuscript("probe-ms", "c.1540", "")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func mustValidLeaf(t *testing.T, svc *Service, m *model.Manuscript, page, quire, chain int) *model.Leaf {
	t.Helper()
	l, err := svc.AddLeaf(m, &model.Leaf{
		PageNo: page, QuireNo: quire, Position: model.PositionRecto, BindingEdge: model.EdgeLeft,
		ChainDeg: chain, WidthMM: 160, HeightMM: 220, Confidence: 0.9,
	})
	if err != nil {
		t.Fatal(err)
	}
	l, err = svc.UpdateLeaf(l.ID, model.LeafValid, l.Confidence, "", l.Version)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func mustWatermark(t *testing.T, svc *Service, leaf *model.Leaf, half, mold string, pos model.WatermarkPosition, x float64) *model.WatermarkObservation {
	t.Helper()
	w, err := svc.AddWatermark(leaf, &model.WatermarkObservation{
		HalfID: half, MoldPairID: mold, Position: pos, XMM: x, YMM: 110, Confidence: 0.9,
	})
	if err != nil {
		t.Fatal(err)
	}
	w, err = svc.ActivateWatermark(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestLeafListSeesUpdatedStatus(t *testing.T) {
	svc := newTestService(t)
	m := mustManuscript(t, svc)
	l, err := svc.AddLeaf(m, &model.Leaf{
		PageNo: 1, QuireNo: 1, Position: model.PositionRecto, BindingEdge: model.EdgeLeft,
		ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.ListLeaves(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Status != model.LeafPending {
		t.Fatalf("setup: %+v", first)
	}
	if _, err := svc.UpdateLeaf(l.ID, model.LeafValid, l.Confidence, "", l.Version); err != nil {
		t.Fatal(err)
	}
	const workers = 20
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			leaves, err := svc.ListLeaves(m.ID)
			if err != nil {
				errCh <- err
				return
			}
			if len(leaves) != 1 || leaves[0].Status != model.LeafValid {
				errCh <- errInvalidLeafList(leaves)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	ver, err := svc.VerifyManuscript(m)
	if err != nil {
		t.Fatal(err)
	}
	if ver.LeafCount != 1 {
		t.Fatalf("verify should see the valid leaf, leaf_count=%d", ver.LeafCount)
	}
}

func errInvalidLeafList(leaves []*model.Leaf) error {
	return &leafListError{n: len(leaves)}
}

type leafListError struct{ n int }

func (e *leafListError) Error() string {
	if e.n == 0 {
		return "list leaves returned empty after update"
	}
	return "list leaves returned stale pending status after update"
}
