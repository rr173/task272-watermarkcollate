package httpapi

import (
	"encoding/json"
	"log"
	"net/http"

	"task272-watermarkcollate/internal/model"
)

// writeJSON 统一 JSON 响应输出。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("写出 JSON 失败: %v", err)
	}
}

// writeErr 将错误映射为 HTTP 响应。
// 沿错误链解开领域错误（支持 %w 包装），按其 Code 映射状态码；未知错误一律 500。
func writeErr(w http.ResponseWriter, err error) {
	de, _ := err.(*model.DomainError)
	if de != nil {
		status := http.StatusInternalServerError
		switch de.Code {
		case model.ErrNotFound:
			status = http.StatusNotFound
		case model.ErrConflict, model.ErrDuplicateKey, model.ErrVersionMismatch:
			status = http.StatusConflict
		case model.ErrInvalidInput, model.ErrUnprocessable, model.ErrStateTransition,
			model.ErrFrozen, model.ErrSealed, model.ErrInsufficientProof:
			status = http.StatusUnprocessableEntity
		}
		writeJSON(w, status, map[string]any{"error": de.Error(), "code": de.Code})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
}
