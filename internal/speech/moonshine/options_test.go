package moonshine

import "testing"

func TestNativeOptions_Privacy(t *testing.T) {
	t.Run("explicitly disables audio return persistence and diagnostic content", func(t *testing.T) {
		options := smallStreamingRuntimeOptions()
		for _, optionName := range []string{"return_audio_data", "log_api_calls", "log_ort_run", "log_output_text", "identify_speakers", "word_timestamps"} {
			if options[optionName] != "false" {
				t.Fatalf("%s got %q, want false", optionName, options[optionName])
			}
		}
		if options["save_input_wav_path"] != "" {
			t.Fatal("audio persistence enabled")
		}
		if _, explicit := options["save_input_wav_path"]; !explicit {
			t.Fatal("audio persistence setting relies on default")
		}
		if options["ort_providers"] != "CPU" {
			t.Fatal("unexpected execution provider")
		}
		if _, present := options["skip_transcription"]; present {
			t.Fatal("upstream interprets this option's presence as skipping inference")
		}
	})
}
