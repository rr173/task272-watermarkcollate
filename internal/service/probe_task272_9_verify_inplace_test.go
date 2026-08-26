package service

import (
	"path/filepath"
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

func TestVerifyDoesNotMutateCallerLeaves(t *testing.T) {
	svc := newTestService(t)
	m := mustManuscript(t, svc)
	l1 := mustValidLeaf(t, svc, m, 1, 1, 90)
	l2, err := svc.AddLeaf(m, &model.Leaf{
		PageNo: 2, QuireNo: 1, Position: model.PositionRecto, BindingEdge: model.EdgeLeft,
		ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateLeaf(l2.ID, model.LeafDamaged, l2.Confidence, "torn", l2.Version); err != nil {
		t.Fatal(err)
	}
	l3 := mustValidLeaf(t, svc, m, 3, 2, 45)
	leaves, err := svc.ListLeaves(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(leaves))
	statuses := make([]model.LeafStatus, len(leaves))
	for i, l := range leaves {
		ids[i], statuses[i] = l.ID, l.Status
	}
	if _, err := svc.VerifyManuscript(m); err != nil {
		t.Fatal(err)
	}
	if len(leaves) != 3 {
		t.Fatalf("caller slice length changed: %d", len(leaves))
	}
	for i, l := range leaves {
		if l == nil || l.ID != ids[i] || l.Status != statuses[i] {
			t.Fatalf("caller slice mutated at %d: id=%v status=%v", i, leafID(l), leafStatus(l))
		}
	}
	if leaves[1].Status != model.LeafDamaged {
		t.Fatalf("damaged leaf dropped from caller list: %+v", leaves[1])
	}
	if l1.ID == "" || l3.ID == "" {
		t.Fatal("sanity")
	}
}

func leafID(l *model.Leaf) string {
	if l == nil {
		return "nil"
	}
	return l.ID
}

func leafStatus(l *model.Leaf) model.LeafStatus {
	if l == nil {
		return ""
	}
	return l.Status
}
