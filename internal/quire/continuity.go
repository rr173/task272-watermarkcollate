package quire

import (
	"fmt"
	"sort"

	"task272-watermarkcollate/internal/model"
)

// FoldGap 折页连续性断裂点：页码连续但折页结构跳变，或页码本身不连续。
type FoldGap struct {
	LeftPage  int    `json:"left_page"`
	RightPage int    `json:"right_page"`
	LeftQuire int    `json:"left_quire"`
	RightQuire int   `json:"right_quire"`
	Reason    string `json:"reason"` // page_gap / quire_jump
	Detail    string `json:"detail"`
}

// Verification 一次折页连续性校验的结果。
type Verification struct {
	ManuscriptID string     `json:"manuscript_id"`
	LeafCount    int        `json:"leaf_count"`
	Gaps         []FoldGap  `json:"gaps"`
	Continuous   bool       `json:"continuous"` // 无断裂点
}

// Verify 校验手稿纸页的折页连续性：
// 按页码升序遍历有效纸页，相邻两页必须满足「页码连续且折页号连续」才算同折页连续；
// 任何断裂点都会被列为重装订候选。
func Verify(leaves []*model.Leaf) Verification {
	ver := Verification{ManuscriptID: "", LeafCount: len(leaves), Gaps: []FoldGap{}}
	if len(leaves) == 0 {
		ver.Continuous = true
		return ver
	}
	sorted := make([]*model.Leaf, 0, len(leaves))
	for _, l := range leaves {
		if !l.Status.IsValidObservation() {
			continue // 破损/排除纸页不参与连续性计算
		}
		cp := *l
		sorted = append(sorted, &cp)
	}
	ver.LeafCount = len(sorted)
	if len(sorted) < 2 {
		ver.Continuous = true
		return ver
	}
	ver.ManuscriptID = sorted[0].ManuscriptID
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].PageNo < sorted[j].PageNo })

	for i := 0; i+1 < len(sorted); i++ {
		left, right := sorted[i], sorted[i+1]
		pageDelta := right.PageNo - left.PageNo
		switch {
		case pageDelta != 1:
			ver.Gaps = append(ver.Gaps, FoldGap{
				LeftPage: left.PageNo, RightPage: right.PageNo,
				LeftQuire: left.QuireNo, RightQuire: right.QuireNo,
				Reason: "page_gap",
				Detail: fmt.Sprintf("页码从 %d 跳变到 %d（缺页或重装订丢失纸页）", left.PageNo, right.PageNo),
			})
		case right.QuireNo != left.QuireNo:
			ver.Gaps = append(ver.Gaps, FoldGap{
				LeftPage: left.PageNo, RightPage: right.PageNo,
				LeftQuire: left.QuireNo, RightQuire: right.QuireNo,
				Reason: "quire_jump",
				Detail: fmt.Sprintf("页码连续但折页从 %d 跳到 %d（折页结构断裂，重装订候选）", left.QuireNo, right.QuireNo),
			})
		}
	}
	ver.Continuous = len(ver.Gaps) == 0
	return ver
}

// GapReasonsOf 将断裂点归纳为简短原因标签（供关系证据摘要使用）。
func GapReasonsOf(gaps []FoldGap) []string {
	out := make([]string, 0, len(gaps))
	for _, g := range gaps {
		out = append(out, g.Reason)
	}
	return out
}
