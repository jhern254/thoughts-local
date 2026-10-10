package data

const DefaultBackgroundDarkness = 70

// Visual contains only native-owned browser presentation preferences.
type Visual struct {
	BackgroundAsset string
	Darkness        int
	Framing         BackgroundFraming
}

// BackgroundFraming uses normalized overflow positions so framing adapts to
// viewport changes without rewriting the original image. Zoom is a percentage
// of Fill scale (100–300); positions span 0–10000, with 5000 centered.
type BackgroundFraming struct {
	Fit       string `json:"fit"`
	Zoom      int    `json:"zoom"`
	PositionX int    `json:"positionX"`
	PositionY int    `json:"positionY"`
}

func DefaultBackgroundFraming() BackgroundFraming {
	return BackgroundFraming{
		Fit:       "fill",
		Zoom:      100,
		PositionX: 5000,
		PositionY: 5000,
	}
}

func (f BackgroundFraming) Valid() bool {
	return (f.Fit == "fill" || f.Fit == "fit") && f.Zoom >= 100 && f.Zoom <= 300 &&
		f.PositionX >= 0 && f.PositionX <= 10000 && f.PositionY >= 0 && f.PositionY <= 10000
}
