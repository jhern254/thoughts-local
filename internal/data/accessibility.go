package data

type BorderWeight string

const (
	BorderNormal     BorderWeight = "normal"
	BorderHeavy      BorderWeight = "heavy"
	BorderExtraHeavy BorderWeight = "extra-heavy"
)

func (w BorderWeight) Valid() bool {
	return w == BorderNormal || w == BorderHeavy || w == BorderExtraHeavy
}
func (w BorderWeight) Step(direction int) BorderWeight {
	values := []BorderWeight{BorderNormal, BorderHeavy, BorderExtraHeavy}
	for i, value := range values {
		if value == w {
			return values[(i+direction+len(values))%len(values)]
		}
	}
	if direction < 0 {
		return BorderExtraHeavy
	}
	return BorderHeavy
}
func (w BorderWeight) Label() string {
	switch w {
	case BorderHeavy:
		return "Heavy"
	case BorderExtraHeavy:
		return "Extra heavy"
	}
	return "Normal"
}

type TextSize string

const (
	TextStandard TextSize = "standard"
	TextLarger   TextSize = "larger"
	TextLargest  TextSize = "largest"
)

func (s TextSize) Valid() bool { return s == TextStandard || s == TextLarger || s == TextLargest }
func (s TextSize) Step(direction int) TextSize {
	values := []TextSize{TextStandard, TextLarger, TextLargest}
	for i, value := range values {
		if value == s {
			return values[(i+direction+len(values))%len(values)]
		}
	}
	if direction < 0 {
		return TextLargest
	}
	return TextLarger
}
func (s TextSize) Label() string {
	switch s {
	case TextLarger:
		return "Larger"
	case TextLargest:
		return "Largest"
	}
	return "Standard"
}

type AccessibilityPreferences struct {
	BorderWeight BorderWeight `json:"borderWeight"`
	HeavierText  bool         `json:"heavierText"`
	TextSize     TextSize     `json:"textSize"`
	HighContrast bool         `json:"highContrast"`
}

func (p AccessibilityPreferences) Valid() bool { return p.BorderWeight.Valid() && p.TextSize.Valid() }
