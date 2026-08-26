package chainline

import "math"

// 链线方向一致性校验。
// 造纸网（laid mold）留下的链线在同一折页内按折叠规律排列：
// 相邻两页若来自同一折页，其链线方向要么同向，要么按折叠轴对称（方向互补）。
// 本模块只消费纸页观测的 chain_deg（0-359 度；-1 表示未观测），不依赖任何存储。

const (
	// degreeTolerance 同向/镜像允许的角度容差。
	degreeTolerance = 15.0
	// Unknown 表示未观测到链线方向。
	Unknown = -1
)

// Result 一次相邻页链线校验的结果。
type Result struct {
	Consistent bool    `json:"consistent"`
	Mode       string  `json:"mode"`        // same_direction / mirror / unknown / reversed
	DeltaDeg   float64 `json:"delta_deg"`   // 最小角差
	Reason     string  `json:"reason"`
}

// Check 校验相邻两页链线方向是否一致。
// aDeg/bDeg 为两页链线方向（0-359 或 -1=未知）。
// 返回 Consistent=true 表示支持同折页假设。
func Check(aDeg, bDeg int) Result {
	if aDeg == Unknown || bDeg == Unknown {
		return Result{
			Consistent: false,
			Mode:       "unknown",
			Reason:     "存在未观测的链线方向，无法提供一致性证据",
		}
	}
	if aDeg < 0 || aDeg > 359 || bDeg < 0 || bDeg > 359 {
		return Result{
			Consistent: false,
			Mode:       "reversed",
			Reason:     "链线方向越界（必须为 0-359 或 -1）",
		}
	}
	// 同向最小角差
	same := angleDelta(aDeg, bDeg)
	// 镜像角差：方向互补（如 90° 与 270°）
	mirror := angleDelta(aDeg, (bDeg+180)%360)
	switch {
	case same <= degreeTolerance:
		return Result{Consistent: true, Mode: "same_direction", DeltaDeg: same, Reason: "链线方向一致"}
	case mirror <= degreeTolerance:
		return Result{Consistent: true, Mode: "mirror", DeltaDeg: mirror, Reason: "链线方向按折叠轴对称"}
	default:
		return Result{Consistent: false, Mode: "reversed", DeltaDeg: same, Reason: "链线方向既不同向也不轴对称，提示不同纸张批次或重装订"}
	}
}

// angleDelta 返回两角度在 [0,180] 内的最小差值。
func angleDelta(a, b int) float64 {
	d := math.Abs(float64(a - b))
	if d > 180 {
		d = 360 - d
	}
	return d
}
