package model

import "time"

// ManuscriptStatus 手稿批次状态机：
// organizing(整理中) → collating(待校对) → adjudicating(待裁决) → published(已发布) → sealed(封存)。
// sealed 为终态；published 可退回 adjudicating 补充裁决，但封存后不可再导入或流转。
type ManuscriptStatus string

const (
	ManuscriptOrganizing   ManuscriptStatus = "organizing"
	ManuscriptCollating    ManuscriptStatus = "collating"
	ManuscriptAdjudicating ManuscriptStatus = "adjudicating"
	ManuscriptPublished    ManuscriptStatus = "published"
	ManuscriptSealed       ManuscriptStatus = "sealed"
)

// Manuscript 一份待复核的历史手稿批次：聚合其纸页观测、水印证据、纸张关系与校勘版本。
type Manuscript struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Period      string           `json:"period"`      // 年代/时期描述
	Description string           `json:"description"` // 来源、物理状态说明
	Status      ManuscriptStatus `json:"status"`
	Version     int              `json:"version"` // 乐观锁版本
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// ValidTransitions 返回手稿允许的目标状态集合；nil 表示仅终态。
func (m ManuscriptStatus) ValidTransitions() []ManuscriptStatus {
	switch m {
	case ManuscriptOrganizing:
		return []ManuscriptStatus{ManuscriptCollating, ManuscriptAdjudicating}
	case ManuscriptCollating:
		return []ManuscriptStatus{ManuscriptAdjudicating, ManuscriptSealed}
	case ManuscriptAdjudicating:
		return []ManuscriptStatus{ManuscriptPublished, ManuscriptSealed}
	case ManuscriptPublished:
		return []ManuscriptStatus{ManuscriptAdjudicating, ManuscriptSealed}
	default:
		return nil
	}
}

// CanTransitionTo 校验状态迁移合法性（唯一显式裁决入口，所有写入路径复用）。
func (m ManuscriptStatus) CanTransitionTo(next ManuscriptStatus) bool {
	if m == next {
		return true
	}
	for _, t := range m.ValidTransitions() {
		if t == next {
			return true
		}
	}
	return false
}

// IsTerminal 是否终态。
func (m ManuscriptStatus) IsTerminal() bool { return m == ManuscriptSealed }
