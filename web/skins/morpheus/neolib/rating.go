package neo

import (
	"math"
	"strconv"
)

// DefaultMax is the default max rating value when none is specified.
const DefaultMax = 5

type RatingOpts struct {
	Value, Max, Precision Attr[float64]
	Icon                  Attr[string]
	Label                 Attr[string]
	Readonly              Attr[bool]
	Disabled              Attr[bool]
	Size                  Attr[string]
	AriaLabel             Attr[string]
}

func (o RatingOpts) max() int {
	m := o.Max.Or(DefaultMax)
	if m < 1 {
		m = 1
	}
	return int(math.Floor(m))
}

func (o RatingOpts) precision() float64 {
	if p, ok := o.Precision.Value(); ok && p > 0 && p <= 1 {
		return p
	}
	return 1
}

func (o RatingOpts) value() float64 {
	v, ok := o.Value.Value()
	if !ok {
		return 0
	}
	p := o.precision()
	snapped := math.Round(v/p) * p
	clamped := math.Min(float64(o.max()), math.Max(0, snapped))
	d := decimalDigits(p)
	r, _ := strconv.ParseFloat(strconv.FormatFloat(clamped, 'f', d, 64), 64)
	return r
}

func (o RatingOpts) icon() string {
	if icon := o.Icon.Or(""); icon != "" {
		return icon
	}
	return "star"
}

func (o RatingOpts) symbolPct(i int) float64 {
	frac := o.value() - float64(i-1)
	if frac <= 0 {
		return 0
	}
	if frac >= 1 {
		return 100
	}
	return frac * 100
}

func ratingValueText(value float64, max int) string {
	return formatNumber(value) + " / " + strconv.Itoa(max)
}

func (o RatingOpts) maxStr() string       { return strconv.Itoa(o.max()) }
func (o RatingOpts) valueStr() string     { return formatNumber(o.value()) }
func (o RatingOpts) precisionStr() string { return formatNumber(o.precision()) }
func (o RatingOpts) valueText() string    { return ratingValueText(o.value(), o.max()) }

func (o RatingOpts) ariaLabel() string {
	if label := o.Label.Or(""); label != "" {
		return label
	}
	return o.AriaLabel.Or("")
}

func (o RatingOpts) symbolIndices() []int {
	n := o.max()
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i + 1
	}
	return idx
}
