package data

type BorderWeight string

const (
	BorderNormal BorderWeight = "normal"
	BorderHeavy  BorderWeight = "heavy"
)

func (w BorderWeight) Valid() bool {
	return w == BorderNormal || w == BorderHeavy
}
func (w BorderWeight) Step(direction int) BorderWeight {
	values := []BorderWeight{BorderNormal, BorderHeavy}
	for i, value := range values {
		if value == w {
			return values[(i+direction+len(values))%len(values)]
		}
	}
	return BorderHeavy
}
func (w BorderWeight) Label() string {
	switch w {
	case BorderHeavy:
		return "Heavy"
	}
	return "Normal"
}

type TextSize string

const (
	TextStandard TextSize = "standard"
	TextLarger   TextSize = "larger"
)

func (s TextSize) Valid() bool { return s == TextStandard || s == TextLarger }
func (s TextSize) Step(direction int) TextSize {
	values := []TextSize{TextStandard, TextLarger}
	for i, value := range values {
		if value == s {
			return values[(i+direction+len(values))%len(values)]
		}
	}
	return TextLarger
}
func (s TextSize) Label() string {
	switch s {
	case TextLarger:
		return "Larger"
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
