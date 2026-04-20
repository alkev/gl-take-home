// Package vecmath provides primitives for vector math used by the store.
package vecmath

import "math"

// Norm returns the L2 (Euclidean) norm of v.
func Norm(v []float32) float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	return float32(math.Sqrt(sum))
}

// Dot returns the dot product of a and b. Panics if lengths differ; callers
// must enforce dimension at insert time so this only fires on a bug.
func Dot(a, b []float32) float32 {
	if len(a) != len(b) {
		panic("vecmath.Dot: length mismatch")
	}
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// InvNorm returns 1 / ‖v‖. Returns 0 if v is the zero vector so the caller
// can easily detect and reject that case.
func InvNorm(v []float32) float32 {
	n := Norm(v)
	if n == 0 {
		return 0
	}
	return 1.0 / n
}
