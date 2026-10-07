package data

const DefaultBackgroundDarkness = 70

// Appearance contains only native-owned browser presentation preferences.
type Appearance struct {
	BackgroundAsset string
	Darkness        int
}
