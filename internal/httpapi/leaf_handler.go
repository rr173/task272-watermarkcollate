package httpapi

import (
	"encoding/json"
	"net/http"

	"task272-watermarkcollate/internal/model"
)

// addLeaf 导入纸页观测（POST /api/manuscripts/{id}/leaves）。
func (s *Server) addLeaf(w http.ResponseWriter, r *http.Request) {
	m, err := s.svc.GetManuscript(pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var req model.Leaf
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, model.InvalidInput("请求体解析失败: %v", err))
		return
	}
	l, err := s.svc.AddLeaf(m, &req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

// listLeaves 手稿纸页列表。
func (s *Server) listLeaves(w http.ResponseWriter, r *http.Request) {
	ls, err := s.svc.ListLeaves(pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ls)
}

// getLeaf 纸页详情。
func (s *Server) getLeaf(w http.ResponseWriter, r *http.Request) {
	l, err := s.svc.GetLeaf(pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

// updateLeaf 更新纸页（PATCH {status, confidence, notes, version}）。
func (s *Server) updateLeaf(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status     string  `json:"status"`
		Confidence float64 `json:"confidence"`
		Notes      string  `json:"notes"`
		Version    int     `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, model.InvalidInput("请求体解析失败: %v", err))
		return
	}
	l, err := s.svc.UpdateLeaf(pathID(r, "id"), model.LeafStatus(req.Status), req.Confidence, req.Notes, req.Version)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

// verifyManuscript 折页连续性校验（GET /api/manuscripts/{id}/verify）。
func (s *Server) verifyManuscript(w http.ResponseWriter, r *http.Request) {
	m, err := s.svc.GetManuscript(pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	ver, err := s.svc.VerifyManuscript(m)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ver)
}

// listManuscriptRelations 手稿全部关系（GET /api/manuscripts/{id}/relations）。
func (s *Server) listManuscriptRelations(w http.ResponseWriter, r *http.Request) {
	rels, err := s.svc.ListRelations(pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rels)
}
