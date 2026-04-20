package vecmath

import (
	"math"
	"testing"
)

func TestNorm(t *testing.T) {
	tests := []struct {
		name string
		in   []float32
		want float32
	}{
		{"unit x", []float32{1, 0, 0}, 1.0},
		{"unit y", []float32{0, 1, 0}, 1.0},
		{"3-4-5 triangle", []float32{3, 4}, 5.0},
		{"zero vector", []float32{0, 0, 0}, 0.0},
		{"negative components", []float32{-3, -4}, 5.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Norm(tc.in)
			if math.Abs(float64(got-tc.want)) > 1e-5 {
				t.Fatalf("Norm(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestDot(t *testing.T) {
	tests := []struct {
		name string
		a, b []float32
		want float32
	}{
		{"orthogonal", []float32{1, 0}, []float32{0, 1}, 0},
		{"parallel", []float32{2, 0}, []float32{3, 0}, 6},
		{"anti-parallel", []float32{1, 0}, []float32{-1, 0}, -1},
		{"general", []float32{1, 2, 3}, []float32{4, 5, 6}, 32},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Dot(tc.a, tc.b)
			if got != tc.want {
				t.Fatalf("Dot(%v,%v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestDotPanicsOnMismatch(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	Dot([]float32{1, 2}, []float32{1, 2, 3})
}

func TestInvNorm(t *testing.T) {
	tests := []struct {
		name string
		in   []float32
		want float32
	}{
		{"unit", []float32{1, 0, 0}, 1.0},
		{"3-4-5", []float32{3, 4}, 1.0 / 5.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := InvNorm(tc.in)
			if math.Abs(float64(got-tc.want)) > 1e-5 {
				t.Fatalf("InvNorm(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestInvNormZeroReturnsZero(t *testing.T) {
	got := InvNorm([]float32{0, 0, 0})
	if got != 0 {
		t.Fatalf("InvNorm of zero vector must be 0 to avoid NaN, got %v", got)
	}
}
