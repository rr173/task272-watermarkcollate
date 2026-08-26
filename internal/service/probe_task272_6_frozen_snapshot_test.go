package service

import (
	"context"
	"encoding/json"
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

func TestFrozenVersionKeepsSnapshotAfterAdjudication(t *testing.T) {
	svc := newTestService(t)
	m := mustManuscript(t, svc)
	l1 := mustValidLeaf(t, svc, m, 1, 1, 90)
	l2 := mustValidLeaf(t, svc, m, 2, 1, 90)
	rel, err := svc.CreateRelation(context.Background(), m, l1.ID, l2.ID, "probe")
	if err != nil {
		t.Fatal(err)
	}
	v, err := svc.CreateVersion(m, "draft")
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := svc.FreezeVersion(context.Background(), v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmRelation(rel.ID, "later", rel.Version); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetVersion(frozen.ID)
	if err != nil {
		t.Fatal(err)
	}
	var snap []*model.LeafRelation
	if err := json.Unmarshal([]byte(got.ContentJSON), &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap) != 1 {
		t.Fatalf("want 1 snap relation, got %d", len(snap))
	}
	if snap[0].Verdict == model.VerdictConfirmed {
		t.Fatalf("frozen snapshot mutated by later confirm: %s", snap[0].Verdict)
	}
}
