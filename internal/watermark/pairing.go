package watermark

import (
	"encoding/json"
	"fmt"
	"math"

	"task272-watermarkcollate/internal/model"
)

// 配对评分权重（可调常量，测试与文档依赖）。
const (
	weightMoldPair  = 0.5 // 模具对一致
	weightSymmetry  = 0.3 // 左右半片位置对称
	weightConfidence = 0.2 // 观测置信度

	scoreMatchedThreshold   = 0.80 // ≥ 判定 matched
	scoreCandidateThreshold = 0.60 // ≥ 判定 candidate，否则 unmatched
)

// PairingEvidence 配对判定依据摘要（JSON 序列化后存入 Evidence 字段）。
type PairingEvidence struct {
	MoldPairEqual bool    `json:"mold_pair_equal"`
	HalfPosition  string  `json:"half_position"`  // complementary / identical / unknown
	SymmetryMM    float64 `json:"symmetry_mm"`    // 左右半片对称偏差（毫米）
	SymmetryScore float64 `json:"symmetry_score"` // 0..1
	Confidence    float64 `json:"confidence"`
	TotalScore    float64 `json:"total_score"`
	Reason        string  `json:"reason"`
}

// PairCandidate 判断两个水印观测是否具备配对资格：
//  1. 均处于 valid 状态；
//  2. 模具对号相同（同一对模具压印）；
//  3. 半片位置互补（left_half ↔ right_half）。
//
// 资格不满足时返回原因字符串，可用于错误提示。
func PairCandidate(a, b *model.WatermarkObservation) (bool, string) {
	if !a.Status.ValidObservation() {
		return false, fmt.Sprintf("观测 %s 状态 %s 不可作为证据", a.HalfID, a.Status)
	}
	if !b.Status.ValidObservation() {
		return false, fmt.Sprintf("观测 %s 状态 %s 不可作为证据", b.HalfID, b.Status)
	}
	if a.MoldPairID != b.MoldPairID {
		return false, fmt.Sprintf("模具对不同：%s vs %s", a.MoldPairID, b.MoldPairID)
	}
	if a.Position == b.Position {
		return false, fmt.Sprintf("半片位置不互补：两者均为 %s", a.Position)
	}
	return true, ""
}

// symmetryScore 计算左右半片位置对称度：
// 两半片质心 x 坐标之和应接近纸页宽度（互为镜像拼接）；偏差超过纸宽 25% 视为不对称。
func symmetryScore(aX, bX, sheetWidthMM float64) float64 {
	if sheetWidthMM <= 0 {
		return 0.5 // 宽度未知时取中性分，不贡献正/负证据
	}
	ideal := sheetWidthMM
	delta := math.Abs(aX + bX - ideal)
	tolerance := 0.25 * ideal
	score := 1.0 - delta/tolerance
	if score < 0 {
		score = 0
	}
	return score
}

// ComputeScore 计算两个水印观测的匹配度（0..1）。
// sheetWidthMM 为纸页宽度（毫米），用于对称性计算。
func ComputeScore(a, b *model.WatermarkObservation, sheetWidthMM float64) float64 {
	if ok, reason := PairCandidate(a, b); !ok {
		_ = reason
		return 0
	}
	moldScore := 1.0 // 模具对相同已由资格检查保证
	symScore := symmetryScore(a.XMM, b.XMM, sheetWidthMM)
	confScore := math.Min(a.Confidence, b.Confidence)
	return weightMoldPair*moldScore + weightSymmetry*symScore + weightConfidence*confScore
}

// Classify 按评分阈值分类：matched / candidate / unmatched。
func Classify(score float64) model.PairingStatus {
	switch {
	case score >= scoreMatchedThreshold:
		return model.PairingMatched
	case score >= scoreCandidateThreshold:
		return model.PairingCandidate
	default:
		return model.PairingUnmatched
	}
}

// Pair 对两个水印观测执行完整配对判定，返回配对实体（含评分与证据摘要）。
// 资格不满足时返回领域错误。
func Pair(a, b *model.WatermarkObservation, sheetWidthMM float64) (*model.WatermarkPairing, error) {
	if ok, reason := PairCandidate(a, b); !ok {
		return nil, model.NewDomainError(model.ErrUnprocessable, "配对失败：%s", reason)
	}
	score := ComputeScore(a, b, sheetWidthMM)
	symScore := symmetryScore(a.XMM, b.XMM, sheetWidthMM)
	position := "complementary"
	if a.Position == b.Position {
		position = "identical"
	}
	ev := PairingEvidence{
		MoldPairEqual: true,
		HalfPosition:  position,
		SymmetryMM:    math.Abs(a.XMM + b.XMM - sheetWidthMM),
		SymmetryScore: symScore,
		Confidence:    math.Min(a.Confidence, b.Confidence),
		TotalScore:    score,
	}
	ev.Reason = fmt.Sprintf("模具对 %s 一致；半片 %s/%s 位置互补；对称偏差 %.1fmm", a.MoldPairID, a.HalfID, b.HalfID, ev.SymmetryMM)
	evJSON, _ := json.Marshal(ev)
	return &model.WatermarkPairing{
		WatermarkAID: a.ID,
		WatermarkBID: b.ID,
		MoldPairID:   a.MoldPairID,
		Score:        score,
		Status:       Classify(score),
		Evidence:     string(evJSON),
		Version:      1,
	}, nil
}
