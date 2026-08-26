package httpapi

import (
	"encoding/json"
	"net/http"

	"task272-watermarkcollate/internal/model"
)

// createManuscript 创建手稿批次。
func (s *Server) createManuscript(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string `json:"title"`
		Period      string `json:"period"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, model.InvalidInput("请求体解析失败: %v", err))
		return
	}
	m, err := s.svc.CreateManuscript(req.Title, req.Period, req.Description)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// listManuscripts 手稿列表。
func (s *Server) listManuscripts(w http.ResponseWriter, r *http.Request) {
	ms, err := s.svc.ListManuscripts()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ms)
}

// getManuscript 手稿详情。
func (s *Server) getManuscript(w http.ResponseWriter, r *http.Request) {
	m, err := s.svc.GetManuscript(pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// updateManuscriptStatus 流转手稿状态（PATCH {status, version}）。
func (s *Server) updateManuscriptStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status  string `json:"status"`
		Version int    `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, model.InvalidInput("请求体解析失败: %v", err))
		return
	}
	m, err := s.svc.UpdateManuscriptStatus(pathID(r, "id"), model.ManuscriptStatus(req.Status), req.Version)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// sealManuscript 封存手稿。
func (s *Server) sealManuscript(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version int `json:"version"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	m, err := s.svc.SealManuscript(pathID(r, "id"), req.Version)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}
