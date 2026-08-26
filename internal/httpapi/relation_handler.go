package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"task272-watermarkcollate/internal/model"
)

// requestPairing 请求水印配对（POST /api/pairings {manuscript_id, watermark_a_id, watermark_b_id}）。
func (s *Server) requestPairing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ManuscriptID string `json:"manuscript_id"`
		WatermarkAID string `json:"watermark_a_id"`
		WatermarkBID string `json:"watermark_b_id"`
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
	p, err := s.svc.RequestPairing(context.Background(), m, req.WatermarkAID, req.WatermarkBID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// listPairings 配对列表（GET /api/pairings?manuscript_id=xxx）。
func (s *Server) listPairings(w http.ResponseWriter, r *http.Request) {
	mid := r.URL.Query().Get("manuscript_id")
	if mid == "" {
		writeErr(w, model.InvalidInput("缺少查询参数 manuscript_id"))
		return
	}
	ps, err := s.svc.ListPairings(mid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

// getPairing 配对详情。
func (s *Server) getPairing(w http.ResponseWriter, r *http.Request) {
	p, err := s.svc.PairingByID(pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// confirmPairing 确认/否决配对（POST /api/pairings/{id}/confirm {confirm, version}）。
func (s *Server) confirmPairing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm bool `json:"confirm"`
		Version int  `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, model.InvalidInput("请求体解析失败: %v", err))
		return
	}
	p, err := s.svc.ConfirmPairing(r.Context(), pathID(r, "id"), req.Confirm, req.Version)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// createRelation 建立相邻纸页关系（POST /api/relations {manuscript_id, left_leaf_id, right_leaf_id, adjudicator}）。
func (s *Server) createRelation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ManuscriptID string `json:"manuscript_id"`
		LeftLeafID   string `json:"left_leaf_id"`
		RightLeafID  string `json:"right_leaf_id"`
		Adjudicator  string `json:"adjudicator"`
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
	rel, err := s.svc.CreateRelation(r.Context(), m, req.LeftLeafID, req.RightLeafID, req.Adjudicator)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rel)
}

// listRelations 关系列表（GET /api/relations?manuscript_id=xxx）。
func (s *Server) listRelations(w http.ResponseWriter, r *http.Request) {
	mid := r.URL.Query().Get("manuscript_id")
	if mid == "" {
		writeErr(w, model.InvalidInput("缺少查询参数 manuscript_id"))
		return
	}
	rels, err := s.svc.ListRelations(mid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rels)
}

// getRelation 关系详情。
func (s *Server) getRelation(w http.ResponseWriter, r *http.Request) {
	rel, err := s.svc.RelationByID(pathID(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

// adjudicateRelation 裁决关系（POST /api/relations/{id}/adjudicate {verdict, adjudicator, version}）。
func (s *Server) adjudicateRelation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Verdict    string `json:"verdict"`
		Adjudicator string `json:"adjudicator"`
		Version    int    `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, model.InvalidInput("请求体解析失败: %v", err))
		return
	}
	rel, err := s.svc.AdjudicateRelation(pathID(r, "id"), model.RelationVerdict(req.Verdict), req.Adjudicator, req.Version)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

// confirmRelation 确认关系（POST /api/relations/{id}/confirm {adjudicator, version}）。
func (s *Server) confirmRelation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Adjudicator string `json:"adjudicator"`
		Version     int    `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, model.InvalidInput("请求体解析失败: %v", err))
		return
	}
	rel, err := s.svc.ConfirmRelation(pathID(r, "id"), req.Adjudicator, req.Version)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

// rebindCandidates 重装订候选汇总（GET /api/candidates?manuscript_id=xxx）。
func (s *Server) rebindCandidates(w http.ResponseWriter, r *http.Request) {
	mid := r.URL.Query().Get("manuscript_id")
	if mid == "" {
		writeErr(w, model.InvalidInput("缺少查询参数 manuscript_id"))
		return
	}
	m, err := s.svc.GetManuscript(mid)
	if err != nil {
		writeErr(w, err)
		return
	}
	out, err := s.svc.RebindCandidates(m)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
