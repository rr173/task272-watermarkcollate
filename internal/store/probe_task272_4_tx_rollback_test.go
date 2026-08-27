package store

import (
	"testing"
	"time"

	"task272-watermarkcollate/internal/model"
)

func TestFailedTxDoesNotBlockLaterInsert(t *testing.T) {
	st, err := Open(t.TempDir() + "/probe.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	m := &model.Manuscript{ID: "m1", Title: "probe", Status: model.ManuscriptOrganizing, Version: 1}
	if err := st.SaveManuscript(m); err != nil {
		t.Fatal(err)
	}
	l1 := &model.Leaf{ID: "l1", ManuscriptID: "m1", PageNo: 1, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafPending, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	if err := st.SaveLeaf(l1); err != nil {
		t.Fatal(err)
	}
	dup := &model.Leaf{ID: "l2", ManuscriptID: "m1", PageNo: 1, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafPending, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	if err := st.SaveLeaf(dup); err == nil {
		t.Fatal("duplicate page must fail")
	}
	l3 := &model.Leaf{ID: "l3", ManuscriptID: "m1", PageNo: 2, QuireNo: 1, Position: model.PositionRecto,
		Status: model.LeafPending, BindingEdge: model.EdgeLeft, ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9, Version: 1}
	done := make(chan error, 1)
	go func() { done <- st.SaveLeaf(l3) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("insert after failed tx must succeed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("insert after failed tx blocked; failed transaction was not rolled back")
	}
}
