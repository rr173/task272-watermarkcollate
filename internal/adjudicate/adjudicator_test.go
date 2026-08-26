package adjudicate

import (
	"testing"

	"task272-watermarkcollate/internal/model"
)

func TestAggregateSameFold(t *testing.T) {
	// 水印高分 + 链线一致 + 折页连续 → 同折页
	v, ev, err := Aggregate(Input{
		WatermarkScore:  0.95,
		ChainConsistent: true,
		FoldContinuous:  true,
		HasWatermark:    true,
	})
	if err != nil {
		t.Fatalf("裁决失败: %v", err)
	}
	if v != model.VerdictSameFold {
		t.Fatalf("应判同折页，实际 %s（总分 %v）", v, ev.Total)
	}
}

func TestAggregateRebound(t *testing.T) {
	// 无水印配对 + 链线反向 + 折页断裂 → 重装订
	v, ev, err := Aggregate(Input{
		WatermarkScore:  0.3,
		ChainConsistent: false,
		FoldContinuous:  false,
		HasWatermark:    true,
	})
	if err != nil {
		t.Fatalf("裁决失败: %v", err)
	}
	if v != model.VerdictRebound {
		t.Fatalf("应判重装订，实际 %s（总分 %v）", v, ev.Total)
	}
}

func TestAggregateConflict(t *testing.T) {
	// 链线一致但折页断裂且无配对水印 → 矛盾 → conflict
	v, _, err := Aggregate(Input{
		WatermarkScore:  0.5,
		ChainConsistent: true,
		FoldContinuous:  false,
		HasWatermark:    false, // 中性 0.5
	})
	if err != nil {
		t.Fatalf("裁决失败: %v", err)
	}
	if v != model.VerdictConflict {
		t.Fatalf("应判冲突待人工，实际 %s", v)
	}
}

func TestAggregateNoWatermarkNeutral(t *testing.T) {
	// 无配对水印 → 中性 0.5；其余全正 → 0.2+0.3+0.3=0.8 → 同折页
	v, ev, err := Aggregate(Input{
		WatermarkScore:  0,
		ChainConsistent: true,
		FoldContinuous:  true,
		HasWatermark:    false,
	})
	if err != nil {
		t.Fatalf("裁决失败: %v", err)
	}
	if v != model.VerdictSameFold {
		t.Fatalf("链线与折页均连续时应判同折页，实际 %s（总分 %v）", v, ev.Total)
	}
}

func TestAggregateInvalidScore(t *testing.T) {
	if _, _, err := Aggregate(Input{WatermarkScore: 1.5, HasWatermark: true}); err == nil {
		t.Fatal("越界评分应报错")
	}
}

func TestRenderEvidence(t *testing.T) {
	_, ev, _ := Aggregate(Input{WatermarkScore: 0.9, ChainConsistent: true, FoldContinuous: true, HasWatermark: true})
	s := RenderEvidence(ev)
	if s == "" {
		t.Fatal("证据摘要不应为空")
	}
}
