package service

import (
	"context"
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

func TestRecoverCreatesMissingPairing(t *testing.T) {
	svc := newTestService(t)
	m := mustManuscript(t, svc)
	left := mustValidLeaf(t, svc, m, 1, 1, 90)
	right := mustValidLeaf(t, svc, m, 2, 1, 90)
	mustWatermark(t, svc, left, "A-L", "MP-001", model.WatermarkLeftHalf, 30)
	mustWatermark(t, svc, right, "A-R", "MP-001", model.WatermarkRightHalf, 130)
	before, err := svc.ListPairings(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatal("setup must not already pair")
	}
	rep, err := svc.Recover(context.Background())
	if err != nil {
		t.Fatalf("Recover must succeed: %v", err)
	}
	if rep.PairingsCreated < 1 {
		t.Fatalf("Recover must create missing pairing, created=%d", rep.PairingsCreated)
	}
	after, err := svc.ListPairings(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) == 0 {
		t.Fatal("missing complementary pairing was not recovered")
	}
}
