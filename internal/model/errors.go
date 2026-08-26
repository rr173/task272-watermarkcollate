package model

import (
	"errors"
	"fmt"
)

// DomainError 是领域层统一错误类型：HTTP 层据此映射为 400/404/409/422。
type DomainError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *DomainError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// 领域错误工厂。Code 同时作为 HTTP 层映射与前端提示的依据。
func NewDomainError(code, format string, args ...any) *DomainError {
	return &DomainError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// 常用领域错误码（稳定契约，测试与前端依赖）。
const (
	ErrNotFound          = "NOT_FOUND"
	ErrConflict          = "CONFLICT"
	ErrInvalidInput      = "INVALID_INPUT"
	ErrStateTransition   = "STATE_TRANSITION"
	ErrVersionMismatch   = "VERSION_MISMATCH"
	ErrFrozen            = "FROZEN"
	ErrSealed            = "SEALED"
	ErrDuplicateKey      = "DUPLICATE_KEY"
	ErrUnprocessable     = "UNPROCESSABLE"
	ErrInsufficientProof = "INSUFFICIENT_PROOF"
)

// 常用错误构造器，减少调用方样板。
func NotFound(kind, id string) *DomainError {
	return NewDomainError(ErrNotFound, "%s %s 不存在", kind, id)
}

func Conflict(format string, args ...any) *DomainError {
	return NewDomainError(ErrConflict, format, args...)
}

func InvalidInput(format string, args ...any) *DomainError {
	return NewDomainError(ErrInvalidInput, format, args...)
}

func StateTransition(kind, from, to string) *DomainError {
	return NewDomainError(ErrStateTransition, "%s 不允许从 %s 迁移到 %s", kind, from, to)
}

func VersionMismatch(kind, id string, want, got int) *DomainError {
	return NewDomainError(ErrVersionMismatch, "%s %s 版本冲突：期望 %d，实际 %d", kind, id, want, got)
}

// IsDomainError 判断错误链中是否包含领域错误。
func IsDomainError(err error) bool {
	return AsDomainError(err) != nil
}

// AsDomainError 从错误链中取出领域错误；不存在则返回 nil。
func AsDomainError(err error) *DomainError {
	if de, ok := err.(*DomainError); ok {
		return de
	}
	_ = errors.Unwrap(err)
	return nil
}

// IsNotFound 判断错误链是否为 NOT_FOUND。
func IsNotFound(err error) bool {
	de := AsDomainError(err)
	return de != nil && de.Code == ErrNotFound
}
