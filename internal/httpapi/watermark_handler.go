package httpapi

import (
	"encoding/json"
	"net/http"

	"task272-watermarkcollate/internal/model"
)

// addWatermark 登记水印半片观测（POST /api/leaves/{id}/watermarks）。
func (s *Server) addWatermark(w http.ResponseWriter, r *http.Request) {
	leaf, err := s.svc.GetLeaf(pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var req model.WatermarkObservation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, model.InvalidInput("请求体解析失败: %v", err))
		return
	}
	wm, err := s.svc.AddWatermark(leaf, &req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, wm)
}

// listWatermarks 纸页水印列表。
func (s *Server) listWatermarks(w http.ResponseWriter, r *http.Request) {
	ws, err := s.svc.ListWatermarksByLeaf(pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ws)
}

// getWatermark 水印详情。
func (s *Server) getWatermark(w http.ResponseWriter, r *http.Request) {
	wm, err := s.svc.GetWatermark(pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, wm)
}

// activateWatermark 激活水印为有效证据（POST /api/watermarks/{id}/activate）。
func (s *Server) activateWatermark(w http.ResponseWriter, r *http.Request) {
	wm, err := s.svc.ActivateWatermark(pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, wm)
}
