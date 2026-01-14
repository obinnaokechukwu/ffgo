//go:build !ios && !android && (amd64 || arm64)

package avutil

import (
	"github.com/obinnaokechukwu/ffgo/internal/platform"
)

// Rational represents a rational number (fraction) as used by FFmpeg (AVRational).
type Rational struct {
	Num int32 // Numerator
	Den int32 // Denominator
}

// NewRational creates a new Rational with the given numerator and denominator.
func NewRational(num, den int32) Rational {
	return Rational{Num: num, Den: den}
}

// Float64 converts the rational to a float64.
// Returns 0 if the denominator is 0.
func (r Rational) Float64() float64 {
	if r.Den == 0 {
		return 0
	}
	return float64(r.Num) / float64(r.Den)
}

// Invert returns the inverted rational (den/num).
func (r Rational) Invert() Rational {
	return Rational{Num: r.Den, Den: r.Num}
}

// IsZero returns true if the rational is zero.
func (r Rational) IsZero() bool {
	return r.Num == 0
}

// Mul multiplies two rationals.
// Uses pure Go implementation (AVRational struct-by-value not supported on non-Darwin).
func (r Rational) Mul(other Rational) Rational {
	// Per design doc: implement in pure Go for cross-platform compatibility
	// FFmpeg's av_mul_q returns struct by value which panics on non-Darwin
	if !platform.SupportsStructByValue {
		return pureGoMul(r, other)
	}
	return pureGoMul(r, other) // Use pure Go always for consistency
}

// Div divides two rationals.
func (r Rational) Div(other Rational) Rational {
	// a/b / c/d = a/b * d/c
	return r.Mul(other.Invert())
}

// Add adds two rationals.
func (r Rational) Add(other Rational) Rational {
	if !platform.SupportsStructByValue {
		return pureGoAdd(r, other)
	}
	return pureGoAdd(r, other)
}

// Sub subtracts two rationals.
func (r Rational) Sub(other Rational) Rational {
	if !platform.SupportsStructByValue {
		return pureGoSub(r, other)
	}
	return pureGoSub(r, other)
}

// Cmp compares two rationals.
// Returns -1 if r < other, 0 if r == other, 1 if r > other.
func (r Rational) Cmp(other Rational) int {
	// Cross-multiply to compare: r.Num/r.Den vs other.Num/other.Den
	// r.Num * other.Den vs other.Num * r.Den
	left := int64(r.Num) * int64(other.Den)
	right := int64(other.Num) * int64(r.Den)

	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

// Reduce reduces the rational to lowest terms.
func (r Rational) Reduce() Rational {
	if r.Den == 0 {
		return r
	}
	g := gcd(abs(r.Num), abs(r.Den))
	if g == 0 {
		return r
	}
	return Rational{Num: r.Num / g, Den: r.Den / g}
}

// Pure Go implementations of rational arithmetic

func pureGoMul(a, b Rational) Rational {
	// a/b * c/d = (a*c)/(b*d)
	return Rational{
		Num: a.Num * b.Num,
		Den: a.Den * b.Den,
	}.Reduce()
}

func pureGoAdd(a, b Rational) Rational {
	// a/b + c/d = (a*d + c*b)/(b*d)
	return Rational{
		Num: a.Num*b.Den + b.Num*a.Den,
		Den: a.Den * b.Den,
	}.Reduce()
}

func pureGoSub(a, b Rational) Rational {
	// a/b - c/d = (a*d - c*b)/(b*d)
	return Rational{
		Num: a.Num*b.Den - b.Num*a.Den,
		Den: a.Den * b.Den,
	}.Reduce()
}

// gcd computes the greatest common divisor.
func gcd(a, b int32) int32 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// abs returns the absolute value.
func abs(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}

// Common frame rates
var (
	FrameRate24    = NewRational(24, 1)
	FrameRate25    = NewRational(25, 1)
	FrameRate30    = NewRational(30, 1)
	FrameRate2997  = NewRational(30000, 1001) // 29.97 fps (NTSC)
	FrameRate50    = NewRational(50, 1)
	FrameRate60    = NewRational(60, 1)
	FrameRate5994  = NewRational(60000, 1001) // 59.94 fps
	FrameRate23976 = NewRational(24000, 1001) // 23.976 fps (film)
)

// TimeBase constants
var (
	TimeBaseMicro  = NewRational(1, 1000000) // Microsecond time base
	TimeBaseMilli  = NewRational(1, 1000)    // Millisecond time base
	TimeBaseSecond = NewRational(1, 1)       // Second time base
)
