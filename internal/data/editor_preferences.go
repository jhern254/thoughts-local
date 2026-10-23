package data

// SpeechPauseMS is an audio-time gap; zero disables automatic newlines.
type EditorPreferences struct {
	HighlightCurrentLine bool
	SpeechPauseMS        int
}

func DefaultEditorPreferences() EditorPreferences { return EditorPreferences{SpeechPauseMS: 2000} }
