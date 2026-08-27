package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"task272-watermarkcollate/internal/model"
	"task272-watermarkcollate/internal/service"
	"task272-watermarkcollate/internal/store"
)

// newTestServer 打开内存库并构造 HTTP Server。
func newTestServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("打开内存库: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(service.New(st))
}

// TestAddLeafDuplicatePageConflict 验证同一手稿再次导入相同页码的纸页时，
// 领域 DUPLICATE_KEY 冲突必须沿错误链传到接口层，返回 409 而非 500。
func TestAddLeafDuplicatePageConflict(t *testing.T) {
	srv := newTestServer(t)

	// 创建手稿。
	rec := postJSON(srv, "/api/manuscripts", `{"title":"残卷","period":"c.1540"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("创建手稿应返回 201，实际 %d: %s", rec.Code, rec.Body.String())
	}
	var m model.Manuscript
	if err := json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&m); err != nil {
		t.Fatalf("解码手稿响应失败: %v (body=%s)", err, rec.Body.String())
	}

	body := `{"page_no":1,"quire_no":1,"position":"recto","status":"pending","binding_edge":"left","chain_deg":90,"width_mm":160,"height_mm":220,"confidence":0.9}`
	rec = postJSON(srv, "/api/manuscripts/"+m.ID+"/leaves", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("首次导入纸页应返回 201，实际 %d: %s", rec.Code, rec.Body.String())
	}

	// 再次导入同页码 → 必须 409，且响应体携带冲突 code。
	rec = postJSON(srv, "/api/manuscripts/"+m.ID+"/leaves", body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("重复页码应返回 409，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), model.ErrDuplicateKey) {
		t.Fatalf("响应体应含 %s，实际 %s", model.ErrDuplicateKey, rec.Body.String())
	}
}

// postJSON 向 Server 发起 JSON POST。
func postJSON(srv *Server, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}
