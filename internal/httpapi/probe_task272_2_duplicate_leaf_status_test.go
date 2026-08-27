package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"task272-watermarkcollate/internal/model"
	"task272-watermarkcollate/internal/service"
	"task272-watermarkcollate/internal/store"
)

func TestDuplicateLeafMapsConflict(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := service.New(st)
	m, err := svc.CreateManuscript("probe", "c.1540", "")
	if err != nil {
		t.Fatal(err)
	}
	leafBody, _ := json.Marshal(model.Leaf{
		PageNo: 1, QuireNo: 1, Position: model.PositionRecto, BindingEdge: model.EdgeLeft,
		ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9,
	})
	srv := New(svc)
	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/manuscripts/"+m.ID+"/leaves", bytes.NewReader(leafBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}
	if rec := post(); rec.Code != http.StatusCreated {
		t.Fatalf("first insert want 201, got %d %s", rec.Code, rec.Body.String())
	}
	rec := post()
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate leaf want 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != model.ErrDuplicateKey {
		t.Fatalf("want code DUPLICATE_KEY, got %#v", payload)
	}
}
