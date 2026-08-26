package model

import (
	"strconv"
	"time"
)

// LeafStatus 纸页观察状态机：
// pending(待解析) → valid(有效) / damaged(破损)；damaged 可恢复 valid 后再次转为 excluded(排除)。
// 只有 valid 纸页参与水印配对、链线校验与折页连续性计算。
type LeafStatus string

const (
	LeafPending LeafStatus = "pending"
	LeafValid   LeafStatus = "valid"
	LeafDamaged LeafStatus = "damaged"
	LeafExcluded LeafStatus = "excluded"
)

// LeafPosition 纸页开面：recto(正面) / verso(背面)。
type LeafPosition string

const (
	PositionRecto LeafPosition = "recto"
	PositionVerso LeafPosition = "verso"
)

// BindingEdge 装订边：left(靠装订线在左) / right(靠装订线在右)。
type BindingEdge string

const (
	EdgeLeft  BindingEdge = "left"
	EdgeRight BindingEdge = "right"
)

// Leaf 一次纸页观测：记录页码、折页归属、开面、装订边、链线方向与观察置信度。
// (manuscript_id, page_no) 全局唯一；(manuscript_id, quire_no) 参与折页连续性验证。
type Leaf struct {
	ID            string       `json:"id"`
	ManuscriptID  string       `json:"manuscript_id"`
	PageNo        int          `json:"page_no"`
	QuireNo       int          `json:"quire_no"`
	Position      LeafPosition `json:"position"`
	Status        LeafStatus   `json:"status"`
	BindingEdge   BindingEdge  `json:"binding_edge"`
	ChainDeg      int          `json:"chain_deg"` // 链线方向 0-359 度；-1 表示未观测/未知
	WidthMM       float64      `json:"width_mm"`
	HeightMM      float64      `json:"height_mm"`
	Confidence    float64      `json:"confidence"` // 0..1
	Notes         string       `json:"notes"`
	Version       int          `json:"version"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

// ValidTransitions 纸页状态迁移：damaged 可回 valid；valid/damaged 均可转 excluded（终态）。
func (s LeafStatus) ValidTransitions() []LeafStatus {
	switch s {
	case LeafPending:
		return []LeafStatus{LeafValid, LeafDamaged, LeafExcluded}
	case LeafValid:
		return []LeafStatus{LeafDamaged, LeafExcluded}
	case LeafDamaged:
		return []LeafStatus{LeafValid, LeafExcluded}
	default:
		return nil
	}
}

// CanTransitionTo 校验纸页状态迁移。终态（无迁移）禁止任何迁移，包括自身。
func (s LeafStatus) CanTransitionTo(next LeafStatus) bool {
	trans := s.ValidTransitions()
	if len(trans) == 0 {
		return false
	}
	if s == next {
		return true
	}
	for _, t := range trans {
		if t == next {
			return true
		}
	}
	return false
}

// IsExcluded 是否被排除出证据计算。
func (s LeafStatus) IsExcluded() bool { return s == LeafExcluded }

// IsValidObservation 是否可作为有效证据。
func (s LeafStatus) IsValidObservation() bool { return s == LeafValid }

// ValidPosition 判断开面枚举合法性。
func ValidPosition(p LeafPosition) bool {
	return p == PositionRecto || p == PositionVerso
}

// ValidEdge 判断装订边枚举合法性。
func ValidEdge(e BindingEdge) bool { return e == EdgeLeft || e == EdgeRight }

// LeafKey 用于内存去重与错误提示的稳定标识。
func (l Leaf) LeafKey() string {
	return l.ManuscriptID + "#" + strconv.Itoa(l.PageNo)
}
