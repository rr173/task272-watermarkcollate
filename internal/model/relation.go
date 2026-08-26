package model

import "time"

// PairingStatus 水印配对状态机：
// candidate(候选) → matched(匹配) / unmatched(不匹配)；matched/unmatched 均可经人工确认 → confirmed / rejected。
type PairingStatus string

const (
	PairingCandidate  PairingStatus = "candidate"
	PairingMatched    PairingStatus = "matched"
	PairingUnmatched  PairingStatus = "unmatched"
	PairingConfirmed  PairingStatus = "confirmed"
	PairingRejected   PairingStatus = "rejected"
)

// WatermarkPairing 两个水印半片观测之间的配对结论：记录匹配度评分与证据摘要。
// (watermark_a_id, watermark_b_id) 唯一。
type WatermarkPairing struct {
	ID            string        `json:"id"`
	ManuscriptID  string        `json:"manuscript_id"`
	WatermarkAID  string        `json:"watermark_a_id"`
	WatermarkBID  string        `json:"watermark_b_id"`
	MoldPairID    string        `json:"mold_pair_id"`
	Score         float64       `json:"score"` // 0..1，匹配度
	Status        PairingStatus `json:"status"`
	Evidence      string        `json:"evidence"` // 判定依据摘要（JSON）
	Version       int           `json:"version"`
	CreatedAt     time.Time     `json:"created_at"`
	ConfirmedAt   *time.Time    `json:"confirmed_at,omitempty"`
}

// ValidTransitions 配对状态迁移。
func (s PairingStatus) ValidTransitions() []PairingStatus {
	switch s {
	case PairingCandidate:
		return []PairingStatus{PairingMatched, PairingUnmatched, PairingRejected}
	case PairingMatched:
		return []PairingStatus{PairingUnmatched, PairingConfirmed, PairingRejected}
	case PairingUnmatched:
		return []PairingStatus{PairingMatched, PairingConfirmed, PairingRejected}
	default:
		return nil
	}
}

// CanTransitionTo 校验配对状态迁移。终态（confirmed/rejected）禁止任何迁移，包括自身。
func (s PairingStatus) CanTransitionTo(next PairingStatus) bool {
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

// IsDecided 是否已产生机器判定（非候选）。
func (s PairingStatus) IsDecided() bool {
	return s == PairingMatched || s == PairingUnmatched
}

// RelationVerdict 相邻纸页关系裁决：
// candidate(候选) → same_fold(同折页) / rebound(重装订) / conflict(冲突) → confirmed(确认)。
type RelationVerdict string

const (
	VerdictCandidate RelationVerdict = "candidate"
	VerdictSameFold  RelationVerdict = "same_fold"
	VerdictRebound   RelationVerdict = "rebound"
	VerdictConflict  RelationVerdict = "conflict"
	VerdictConfirmed RelationVerdict = "confirmed"
)

// LeafRelation 相邻两页（左页、右页）的纸张关系：聚合水印配对分、链线一致性、折页连续性证据，
// 由裁决器给出同折页/重装订/冲突结论，研究者可确认后固化。
// (left_leaf_id, right_leaf_id) 唯一。
type LeafRelation struct {
	ID              string         `json:"id"`
	ManuscriptID    string         `json:"manuscript_id"`
	LeftLeafID      string         `json:"left_leaf_id"`
	RightLeafID     string         `json:"right_leaf_id"`
	PageDelta       int            `json:"page_delta"`
	ChainConsistent bool           `json:"chain_consistent"`
	WatermarkScore  float64        `json:"watermark_score"`
	FoldContinuous  bool           `json:"fold_continuous"`
	GapReasons      []string       `json:"gap_reasons,omitempty"`
	Verdict         RelationVerdict `json:"verdict"`
	Evidence        string         `json:"evidence"`
	Adjudicator     string         `json:"adjudicator,omitempty"`
	Version         int            `json:"version"`
	CreatedAt       time.Time      `json:"created_at"`
	AdjudicatedAt   *time.Time     `json:"adjudicated_at,omitempty"`
}

// ValidTransitions 关系裁决状态迁移。
func (v RelationVerdict) ValidTransitions() []RelationVerdict {
	switch v {
	case VerdictCandidate:
		return []RelationVerdict{VerdictSameFold, VerdictRebound, VerdictConflict, VerdictConfirmed}
	case VerdictSameFold, VerdictRebound, VerdictConflict:
		return []RelationVerdict{VerdictConfirmed}
	default:
		return nil
	}
}

// CanTransitionTo 校验关系裁决迁移。终态（confirmed）禁止任何迁移，包括自身。
func (v RelationVerdict) CanTransitionTo(next RelationVerdict) bool {
	trans := v.ValidTransitions()
	if len(trans) == 0 {
		return false
	}
	if v == next {
		return true
	}
	for _, t := range trans {
		if t == next {
			return true
		}
	}
	return false
}

// IsConfirmed 是否已确认。
func (v RelationVerdict) IsConfirmed() bool { return v == VerdictConfirmed }
