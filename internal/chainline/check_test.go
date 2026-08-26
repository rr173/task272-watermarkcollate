package chainline

import "testing"

func TestCheckSameDirection(t *testing.T) {
	r := Check(90, 90)
	if !r.Consistent || r.Mode != "same_direction" {
		t.Fatalf("同向应一致: %+v", r)
	}
	// 10 度内容差
	r = Check(90, 95)
	if !r.Consistent {
		t.Fatalf("小角度差应视为一致: %+v", r)
	}
}

func TestCheckMirror(t *testing.T) {
	// 90° 与 270° 是折叠轴对称 → 一致
	r := Check(90, 270)
	if !r.Consistent || r.Mode != "mirror" {
		t.Fatalf("镜像方向应一致: %+v", r)
	}
}

func TestCheckReversed(t *testing.T) {
	// 90° 与 45° 既不同向也不镜像 → 不一致
	r := Check(90, 45)
	if r.Consistent {
		t.Fatalf("显著方向差应判不一致: %+v", r)
	}
	if r.Mode != "reversed" {
		t.Fatalf("模式应为 reversed: %+v", r)
	}
}

func TestCheckUnknown(t *testing.T) {
	r := Check(Unknown, 90)
	if r.Consistent || r.Mode != "unknown" {
		t.Fatalf("未知方向应判 unknown: %+v", r)
	}
	r = Check(90, Unknown)
	if r.Consistent || r.Mode != "unknown" {
		t.Fatalf("未知方向应判 unknown: %+v", r)
	}
}

func TestCheckOutOfRange(t *testing.T) {
	r := Check(-2, 90)
	if r.Consistent || r.Mode != "reversed" {
		t.Fatalf("越界方向应判 reversed: %+v", r)
	}
	r = Check(90, 400)
	if r.Consistent {
		t.Fatalf("越界方向应判不一致: %+v", r)
	}
}
