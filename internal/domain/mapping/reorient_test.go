package mapping

import "testing"

func TestReorientTransform_Table(t *testing.T) {
	tests := []struct {
		from, to string
		want     Transform
	}{
		{"top", "top", TransformNone},
		{"bottom", "top", TransformMirrorY},
		{"top", "bottom", TransformMirrorY},
		{"left", "right", TransformMirrorX},
		{"top", "right", TransformRotateCW},
		{"right", "bottom", TransformRotateCW},
		{"left", "top", TransformRotateCW},
		{"top", "left", TransformRotateCCW},
		{"bottom", "right", TransformRotateCCW},
		{"north", "top", TransformNone},
	}
	for _, tt := range tests {
		t.Run(tt.from+"_"+tt.to, func(t *testing.T) {
			if got := ReorientTransform(tt.from, tt.to); got != tt.want {
				t.Fatalf("ReorientTransform(%s, %s) = %s, want %s", tt.from, tt.to, got, tt.want)
			}
		})
	}
}
