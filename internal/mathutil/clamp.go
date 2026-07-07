// Package mathutil provides small generic numeric helpers.
package mathutil

// Clamp restricts v to the inclusive range [lo, hi].
func Clamp[T int | int64 | float64](v, lo, hi T) T {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
