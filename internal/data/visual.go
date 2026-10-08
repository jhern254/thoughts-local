package data

const DefaultBackgroundDarkness = 70

// Visual contains only native-owned browser presentation preferences.
type Visual struct {
	BackgroundAsset string
	Darkness        int
}
