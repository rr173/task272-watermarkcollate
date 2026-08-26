package quire

import (
	"testing"

	"task272-watermarkcollate/internal/model"
)

func leaf(page, quire int, status model.LeafStatus) *model.Leaf {
	return &model.Leaf{
		ID: "", ManuscriptID: "m1", PageNo: page, QuireNo: quire,
		Position: model.PositionRecto, Status: status, BindingEdge: model.EdgeLeft,
		ChainDeg: 90, WidthMM: 160, HeightMM: 220, Confidence: 0.9,
	}
}

func TestVerifyContinuous(t *testing.T) {
	// 全部纸页处于同一折页且页码连续 → 无断裂点
	leaves := []*model.Leaf{
		leaf(1, 1, model.LeafValid),
		leaf(2, 1, model.LeafValid),
		leaf(3, 1, model.LeafValid),
		leaf(4, 1, model.LeafValid),
	}
	ver := Verify(leaves)
	if !ver.Continuous || len(ver.Gaps) != 0 {
		t.Fatalf("连续折页不应有断裂点: %+v", ver.Gaps)
	}
}

func TestVerifyQuireJump(t *testing.T) {
	leaves := []*model.Leaf{
		leaf(1, 1, model.LeafValid),
		leaf(2, 1, model.LeafValid),
		leaf(3, 5, model.LeafValid), // 折页跳变：2→5
		leaf(4, 5, model.LeafValid),
	}
	ver := Verify(leaves)
	if len(ver.Gaps) != 1 {
		t.Fatalf("应检测到 1 个断裂点: %+v", ver.Gaps)
	}
	g := ver.Gaps[0]
	if g.Reason != "quire_jump" || g.LeftPage != 2 || g.RightPage != 3 {
		t.Fatalf("断裂点信息错误: %+v", g)
	}
}

func TestVerifyPageGap(t *testing.T) {
	leaves := []*model.Leaf{
		leaf(1, 1, model.LeafValid),
		leaf(3, 1, model.LeafValid), // 页码跳变：1→3
		leaf(4, 1, model.LeafValid),
	}
	ver := Verify(leaves)
	if len(ver.Gaps) != 1 {
		t.Fatalf("应检测到页码断裂: %+v", ver.Gaps)
	}
	if ver.Gaps[0].Reason != "page_gap" {
		t.Fatalf("断裂原因应为 page_gap: %+v", ver.Gaps[0])
	}
}

func TestVerifySkipsExcluded(t *testing.T) {
	leaves := []*model.Leaf{
		leaf(1, 1, model.LeafValid),
		leaf(2, 1, model.LeafExcluded), // 排除页不参与
		leaf(3, 2, model.LeafValid),
	}
	ver := Verify(leaves)
	// 只剩 1、3 两页参与：页码跳变 → 断裂点
	if len(ver.Gaps) != 1 || ver.Gaps[0].Reason != "page_gap" {
		t.Fatalf("排除页应被跳过并检测断裂: %+v", ver.Gaps)
	}
	if ver.LeafCount != 2 {
		t.Fatalf("参与计算的纸页数应为 2，实际 %d", ver.LeafCount)
	}
}

func TestVerifyEmptyAndSingle(t *testing.T) {
	if ver := Verify(nil); !ver.Continuous {
		t.Fatal("空列表应连续")
	}
	if ver := Verify([]*model.Leaf{leaf(1, 1, model.LeafValid)}); !ver.Continuous {
		t.Fatal("单页应连续")
	}
}

func TestGapReasonsOf(t *testing.T) {
	gaps := []FoldGap{{Reason: "quire_jump"}, {Reason: "page_gap"}}
	reasons := GapReasonsOf(gaps)
	if len(reasons) != 2 || reasons[0] != "quire_jump" {
		t.Fatalf("原因标签提取错误: %v", reasons)
	}
}
