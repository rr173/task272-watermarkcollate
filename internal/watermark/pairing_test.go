package watermark

import (
	"math"
	"testing"

	"task272-watermarkcollate/internal/model"
)

func validWM(id, halfID, mold string, pos model.WatermarkPosition, x, conf float64) *model.WatermarkObservation {
	return &model.WatermarkObservation{
		ID: id, HalfID: halfID, MoldPairID: mold, Position: pos,
		XMM: x, YMM: 110, RotationDeg: 0, Confidence: conf, Status: model.WatermarkValid,
	}
}

func TestPairCandidate(t *testing.T) {
	a := validWM("a", "A-L", "MP-001", model.WatermarkLeftHalf, 30, 0.9)
	b := validWM("b", "A-R", "MP-001", model.WatermarkRightHalf, 130, 0.9)
	if ok, reason := PairCandidate(a, b); !ok {
		t.Fatalf("互补半片应具备配对资格: %s", reason)
	}

	// 模具对不同 → 拒绝
	b2 := validWM("b2", "B-R", "MP-002", model.WatermarkRightHalf, 130, 0.9)
	if ok, _ := PairCandidate(a, b2); ok {
		t.Fatal("模具对不同不应配对")
	}

	// 半片位置相同 → 拒绝
	b3 := validWM("b3", "A-L2", "MP-001", model.WatermarkLeftHalf, 130, 0.9)
	if ok, _ := PairCandidate(a, b3); ok {
		t.Fatal("两个左半片不应配对")
	}

	// 观测未激活 → 拒绝
	b4 := validWM("b4", "A-R", "MP-001", model.WatermarkRightHalf, 130, 0.9)
	b4.Status = model.WatermarkPending
	if ok, _ := PairCandidate(a, b4); ok {
		t.Fatal("未激活观测不应参与配对")
	}
}

func TestComputeScore(t *testing.T) {
	a := validWM("a", "A-L", "MP-001", model.WatermarkLeftHalf, 30, 0.9)
	b := validWM("b", "A-R", "MP-001", model.WatermarkRightHalf, 130, 0.88)
	score := ComputeScore(a, b, 160)
	// 期望：模具 0.5 + 对称 0.3 + 置信 0.176 ≈ 0.976
	want := 0.5 + 0.3*1.0 + 0.2*0.88
	if math.Abs(score-want) > 1e-9 {
		t.Fatalf("评分期望 %v，实际 %v", want, score)
	}
	if Classify(score) != model.PairingMatched {
		t.Fatalf("评分 %v 应判 matched", score)
	}
}

func TestComputeScoreSymmetryPenalty(t *testing.T) {
	// 半片明显不对齐（x 和远偏离纸宽）→ 对称分下降
	a := validWM("a", "A-L", "MP-001", model.WatermarkLeftHalf, 30, 0.9)
	b := validWM("b", "A-R", "MP-001", model.WatermarkRightHalf, 60, 0.88)
	score := ComputeScore(a, b, 160)
	if score >= 0.8 {
		t.Fatalf("对称偏差大时不应判 matched，实际 %v", score)
	}
	if Classify(score) != model.PairingUnmatched && Classify(score) != model.PairingCandidate {
		t.Fatalf("对称偏差大时应降级，实际 %v", Classify(score))
	}
}

func TestPairProducesEvidence(t *testing.T) {
	a := validWM("a", "A-L", "MP-001", model.WatermarkLeftHalf, 30, 0.9)
	b := validWM("b", "A-R", "MP-001", model.WatermarkRightHalf, 130, 0.88)
	p, err := Pair(a, b, 160)
	if err != nil {
		t.Fatalf("配对失败: %v", err)
	}
	if p.Status != model.PairingMatched {
		t.Fatalf("期望 matched，实际 %s", p.Status)
	}
	if p.Evidence == "" {
		t.Fatal("应生成证据摘要")
	}
	if p.MoldPairID != "MP-001" {
		t.Fatalf("模具对号错误: %s", p.MoldPairID)
	}
}
