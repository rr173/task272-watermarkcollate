package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"task272-watermarkcollate/internal/demo"
	"task272-watermarkcollate/internal/model"
)

// createVersion 创建校勘版本（POST /api/versions {manuscript_id, summary}）。
func (s *Server) createVersion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ManuscriptID string `json:"manuscript_id"`
		Summary      string `json:"summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, model.InvalidInput("请求体解析失败: %v", err))
		return
	}
	m, err := s.svc.GetManuscript(req.ManuscriptID)
	if err != nil {
		writeErr(w, err)
		return
	}
	v, err := s.svc.CreateVersion(m, req.Summary)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

// listVersions 版本列表（GET /api/versions?manuscript_id=xxx）。
func (s *Server) listVersions(w http.ResponseWriter, r *http.Request) {
	mid := r.URL.Query().Get("manuscript_id")
	if mid == "" {
		writeErr(w, model.InvalidInput("缺少查询参数 manuscript_id"))
		return
	}
	vs, err := s.svc.ListVersions(mid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, vs)
}

// getVersion 版本详情。
func (s *Server) getVersion(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.GetVersion(pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// freezeVersion 冻结版本（POST /api/versions/{id}/freeze）。
func (s *Server) freezeVersion(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.FreezeVersion(context.Background(), pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// supersedeVersion 替代版本（POST /api/versions/{id}/supersede {summary}）。
func (s *Server) supersedeVersion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Summary string `json:"summary"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	v, err := s.svc.SupersedeVersion(pathID(r, "id"), req.Summary)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// stats 统计（GET /api/stats）。
func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	out, err := s.svc.Stats()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// selfcheck 自检（POST /api/selfcheck）：在内存库上跑完整 smoke 场景并返回结果。
func (s *Server) selfcheck(w http.ResponseWriter, r *http.Request) {
	res, err := demo.RunSmokeTest("")
	if err != nil {
		writeErr(w, model.NewDomainError(model.ErrUnprocessable, "自检失败: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, res)
}
