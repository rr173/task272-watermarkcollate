package adjudicate

import (
	"encoding/json"
	"fmt"

	"task272-watermarkcollate/internal/model"
)

// 重装订裁决权重与阈值。
const (
	weightWatermark = 0.4 // 水印配对证据
	weightChainline = 0.3 // 链线一致性证据
	weightFold      = 0.3 // 折页连续性证据

	sameFoldThreshold  = 0.75 // 总分 ≥ 判同折页
	reboundThreshold   = 0.45 // 总分 ≤ 判重装订候选
)

// Input 裁决输入：聚合三个证据通道的归一化分值。
// WatermarkScore 取配对评分（0..1；无观测时取 0.5 中性）。
// ChainConsistent/FoldContinuous 为布尔证据，转换后贡献满分或零分。
type Input struct {
	WatermarkScore  float64 `json:"watermark_score"`
	ChainConsistent bool    `json:"chain_consistent"`
	FoldContinuous  bool    `json:"fold_continuous"`
	HasWatermark    bool    `json:"has_watermark"`
}

// Evidence 裁决证据摘要（JSON 序列化）。
type Evidence struct {
	WeightedWatermark float64 `json:"weighted_watermark"`
	WeightedChainline float64 `json:"weighted_chainline"`
	WeightedFold      float64 `json:"weighted_fold"`
	Total             float64 `json:"total"`
	Verdict           string  `json:"verdict"`
	Reason            string  `json:"reason"`
}

// Aggregate 聚合三个证据通道，给出关系裁决：
//   - Total ≥ sameFoldThreshold → 同折页（same_fold）
//   - Total ≤ reboundThreshold → 重装订候选（rebound）
//   - 其余 → 冲突待人工（conflict）
//
// 裁决只读，不修改任何状态；调用方负责落库与状态机校验。
func Aggregate(in Input) (model.RelationVerdict, Evidence, error) {
	if in.WatermarkScore < 0 || in.WatermarkScore > 1 {
		return "", Evidence{}, fmt.Errorf("水印配对分越界: %v", in.WatermarkScore)
	}
	wm := in.WatermarkScore
	if !in.HasWatermark {
		wm = 0.5 // 无水印证据时取中性值，不偏向任何结论
	}
	chain := 0.0
	if in.ChainConsistent {
		chain = 1.0
	}
	fold := 0.0
	if in.FoldContinuous {
		fold = 1.0
	}
	total := weightWatermark*wm + weightChainline*chain + weightFold*fold

	ev := Evidence{
		WeightedWatermark: weightWatermark * wm,
		WeightedChainline: weightChainline * chain,
		WeightedFold:      weightFold * fold,
		Total:             total,
	}
	var verdict model.RelationVerdict
	switch {
	case total >= sameFoldThreshold:
		verdict = model.VerdictSameFold
		ev.Reason = fmt.Sprintf("证据总分 %.2f ≥ %.2f：水印配对、链线方向与折页连续性共同支持同折页", total, sameFoldThreshold)
	case total <= reboundThreshold:
		verdict = model.VerdictRebound
		ev.Reason = fmt.Sprintf("证据总分 %.2f ≤ %.2f：物理证据断裂，判定为重装订候选", total, reboundThreshold)
	default:
		verdict = model.VerdictConflict
		ev.Reason = fmt.Sprintf("证据总分 %.2f 介于阈值之间：证据互相矛盾，需研究者人工裁决", total)
	}
	ev.Verdict = string(verdict)
	return verdict, ev, nil
}

// RenderEvidence 将裁决证据序列化为 JSON 字符串（存入关系表的 Evidence 字段）。
func RenderEvidence(ev Evidence) string {
	b, _ := json.Marshal(ev)
	return string(b)
}
