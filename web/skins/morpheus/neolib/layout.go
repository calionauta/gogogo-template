package neo

import "github.com/a-h/templ"

type FlexDirection string

const (
	DirectionDefault FlexDirection = ""
	Row              FlexDirection = "row"
	RowReverse       FlexDirection = "row-reverse"
	Column           FlexDirection = "column"
	ColumnReverse    FlexDirection = "column-reverse"
)

type FlexWrap string

const (
	WrapDefault FlexWrap = ""
	Nowrap      FlexWrap = "nowrap"
	Wrap        FlexWrap = "wrap"
	WrapReverse FlexWrap = "wrap-reverse"
)

type AlignItems string

const (
	AlignItemsDefault   AlignItems = ""
	AlignItemsFlexStart AlignItems = "flex-start"
	AlignItemsFlexEnd   AlignItems = "flex-end"
	AlignItemsCenter    AlignItems = "center"
	AlignItemsStretch   AlignItems = "stretch"
	AlignItemsBaseline  AlignItems = "baseline"
)

type JustifyContent string

const (
	JustifyContentDefault      JustifyContent = ""
	JustifyContentFlexStart    JustifyContent = "flex-start"
	JustifyContentFlexEnd      JustifyContent = "flex-end"
	JustifyContentCenter       JustifyContent = "center"
	JustifyContentSpaceBetween JustifyContent = "space-between"
	JustifyContentSpaceAround  JustifyContent = "space-around"
	JustifyContentSpaceEvenly  JustifyContent = "space-evenly"
)

type AlignContent string

const (
	AlignContentDefault      AlignContent = ""
	AlignContentFlexStart    AlignContent = "flex-start"
	AlignContentFlexEnd      AlignContent = "flex-end"
	AlignContentCenter       AlignContent = "center"
	AlignContentSpaceBetween AlignContent = "space-between"
	AlignContentSpaceAround  AlignContent = "space-around"
	AlignContentSpaceEvenly  AlignContent = "space-evenly"
	AlignContentStretch      AlignContent = "stretch"
)

type AlignSelf string

const (
	AlignSelfDefault   AlignSelf = ""
	AlignSelfFlexStart AlignSelf = "flex-start"
	AlignSelfFlexEnd   AlignSelf = "flex-end"
	AlignSelfCenter    AlignSelf = "center"
	AlignSelfStretch   AlignSelf = "stretch"
	AlignSelfBaseline  AlignSelf = "baseline"
)

type Gap string

const (
	GapDefault Gap = ""
	GapNone    Gap = "none"
	GapXs      Gap = "xs"
	GapSm      Gap = "sm"
	GapMd      Gap = "md"
	GapLg      Gap = "lg"
	GapXl      Gap = "xl"
	Gap2xl     Gap = "2xl"
)

type MinHeight string

const (
	MinHeightDefault MinHeight = ""
	MinHeightScreen  MinHeight = "100vh"
	MinHeightFull    MinHeight = "100%"
)

type Collapse string

const (
	CollapseDefault Collapse = ""
	CollapseSm      Collapse = "sm"
	CollapseMd      Collapse = "md"
	CollapseLg      Collapse = "lg"
)

type FlexBasis string

const (
	FlexBasisDefault FlexBasis = ""
	FlexBasis0       FlexBasis = "0"
	FlexBasisAuto    FlexBasis = "auto"
	FlexBasisFull    FlexBasis = "100%"
)

type Overflow string

const (
	OverflowDefault Overflow = ""
	OverflowVisible Overflow = "visible"
	OverflowHidden  Overflow = "hidden"
	OverflowScroll  Overflow = "scroll"
	OverflowAuto    Overflow = "auto"
)

type ItemSize string

const (
	ItemSizeDefault ItemSize = ""
	ItemSizeFull    ItemSize = "100%"
)

type LayoutOpts struct {
	Direction      Attr[FlexDirection]
	Wrap           Attr[FlexWrap]
	AlignItems     Attr[AlignItems]
	JustifyContent Attr[JustifyContent]
	AlignContent   Attr[AlignContent]
	Gap            Attr[Gap]
	ColumnGap      Attr[Gap]
	RowGap         Attr[Gap]
	MinHeight      Attr[MinHeight]
	Inline         Attr[bool]
	Container      Attr[bool]
	Collapse       Attr[Collapse]
}

type ItemOpts struct {
	Grow       Attr[bool]
	NoShrink   Attr[bool]
	Basis      Attr[FlexBasis]
	AlignSelf  Attr[AlignSelf]
	Width      Attr[ItemSize]
	Height     Attr[ItemSize]
	MinWidth0  Attr[bool]
	MinHeight0 Attr[bool]
	Overflow   Attr[Overflow]
	Truncate   Attr[bool]
	HideBelow  Attr[Collapse]
	ShowBelow  Attr[Collapse]
}

func itemAttr[T ~string](a templ.Attributes, key string, v Attr[T]) {
	if val := v.Or(""); val != "" {
		a[key] = string(val)
	}
}

func Item(opts ItemOpts) templ.Attributes {
	a := templ.Attributes{}
	if opts.Grow.Or(false) {
		a["neo-flex-grow"] = "1"
	}
	if opts.NoShrink.Or(false) {
		a["neo-flex-shrink"] = "0"
	}
	itemAttr(a, "neo-flex-basis", opts.Basis)
	itemAttr(a, "neo-align-self", opts.AlignSelf)
	itemAttr(a, "neo-width", opts.Width)
	itemAttr(a, "neo-height", opts.Height)
	if opts.MinWidth0.Or(false) {
		a["neo-min-width"] = "0"
	}
	if opts.MinHeight0.Or(false) {
		a["neo-min-height"] = "0"
	}
	itemAttr(a, "neo-overflow", opts.Overflow)
	if opts.Truncate.Or(false) {
		a["neo-truncate"] = true
	}
	itemAttr(a, "neo-hide-below", opts.HideBelow)
	itemAttr(a, "neo-show-below", opts.ShowBelow)
	return a
}
