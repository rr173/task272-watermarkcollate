package model

import "time"

// VersionStatus 校勘版本状态机：
// draft(草稿) → shared(共享) → frozen(冻结) → superseded(替代)。
// 冻结后 ContentJSON 不可修改；替代仅允许为冻结版本产生新版本，不改写历史快照。
type VersionStatus string

const (
	VersionDraft      VersionStatus = "draft"
	VersionShared     VersionStatus = "shared"
	VersionFrozen     VersionStatus = "frozen"
	VersionSuperseded VersionStatus = "superseded"
)

// CollationVersion 一次校勘版本：冻结时保存纸张关系与裁决的不可变快照。
// (manuscript_id, version_no) 唯一；发布新版本前旧版本必须冻结或替代。
type CollationVersion struct {
	ID           string        `json:"id"`
	ManuscriptID string        `json:"manuscript_id"`
	VersionNo    int           `json:"version_no"`
	Status       VersionStatus `json:"status"`
	Summary      string        `json:"summary"`
	ContentJSON  string        `json:"content_json"` // 冻结的纸张关系快照（JSON）
	CreatedAt    time.Time     `json:"created_at"`
	FrozenAt     *time.Time    `json:"frozen_at,omitempty"`
	SupersededAt *time.Time    `json:"superseded_at,omitempty"`
}

// ValidTransitions 版本状态迁移：frozen 可被 superseded；其余只能前进。
func (s VersionStatus) ValidTransitions() []VersionStatus {
	switch s {
	case VersionDraft:
		return []VersionStatus{VersionShared, VersionFrozen}
	case VersionShared:
		return []VersionStatus{VersionFrozen, VersionDraft}
	case VersionFrozen:
		return []VersionStatus{VersionSuperseded}
	default:
		return nil
	}
}

// CanTransitionTo 校验版本状态迁移。终态（superseded）禁止任何迁移，包括自身。
func (s VersionStatus) CanTransitionTo(next VersionStatus) bool {
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

// IsFrozen 是否已冻结。
func (s VersionStatus) IsFrozen() bool { return s == VersionFrozen }

// IsMutable 是否仍可修改内容。
func (s VersionStatus) IsMutable() bool {
	return s == VersionDraft || s == VersionShared
}
