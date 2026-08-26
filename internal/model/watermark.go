package model

import (
	"fmt"
	"time"
)

// WatermarkPosition 水印半片在纸页上的位置：
// left_half / right_half —— 同一模具对压印的左右互补半片。
type WatermarkPosition string

const (
	WatermarkLeftHalf  WatermarkPosition = "left_half"
	WatermarkRightHalf WatermarkPosition = "right_half"
)

// WatermarkStatus 水印观测状态：pending → valid / excluded。
type WatermarkStatus string

const (
	WatermarkPending  WatermarkStatus = "pending"
	WatermarkValid    WatermarkStatus = "valid"
	WatermarkExcluded WatermarkStatus = "excluded"
)

// WatermarkObservation 一次水印半片观测：
// MoldPairID 标识制造该水印的模具对（同一对模具压出同一批纸）；
// HalfID 是该半片的稳定标识（如 "WM-2026-07-A-L"）；
// XMM/YMM 为半片质心相对纸页装订边与顶边的毫米坐标，用于左右半片对称性配对。
type WatermarkObservation struct {
	ID         string            `json:"id"`
	LeafID     string            `json:"leaf_id"`
	HalfID     string            `json:"half_id"`
	MoldPairID string            `json:"mold_pair_id"`
	Position   WatermarkPosition `json:"position"`
	XMM        float64           `json:"x_mm"`
	YMM        float64           `json:"y_mm"`
	RotationDeg float64          `json:"rotation_deg"`
	Confidence float64           `json:"confidence"` // 0..1
	Status     WatermarkStatus   `json:"status"`
	Notes      string            `json:"notes"`
	CreatedAt  time.Time         `json:"created_at"`
}

// ValidPosition 判断水印半片位置枚举。
func ValidWatermarkPosition(p WatermarkPosition) bool {
	return p == WatermarkLeftHalf || p == WatermarkRightHalf
}

// ValidTransitions 水印观测状态迁移。
func (s WatermarkStatus) ValidTransitions() []WatermarkStatus {
	switch s {
	case WatermarkPending:
		return []WatermarkStatus{WatermarkValid, WatermarkExcluded}
	case WatermarkValid:
		return []WatermarkStatus{WatermarkExcluded}
	default:
		return nil
	}
}

// CanTransitionTo 校验水印状态迁移。终态（excluded）禁止任何迁移，包括自身。
func (s WatermarkStatus) CanTransitionTo(next WatermarkStatus) bool {
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

// ValidObservation 是否可作为配对证据。
func (s WatermarkStatus) ValidObservation() bool { return s == WatermarkValid }

// ComplementaryPosition 返回与给定位置互补的半片位置（left ↔ right）。
func ComplementaryPosition(p WatermarkPosition) (WatermarkPosition, error) {
	switch p {
	case WatermarkLeftHalf:
		return WatermarkRightHalf, nil
	case WatermarkRightHalf:
		return WatermarkLeftHalf, nil
	default:
		return "", fmt.Errorf("未知水印半片位置 %q", p)
	}
}
