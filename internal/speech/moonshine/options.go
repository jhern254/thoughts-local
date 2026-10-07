package moonshine

// Upstream returns audio by default. Explicit privacy settings keep native
// buffers, authored text and debug WAVs out of output and operational diagnostics.
func smallStreamingRuntimeOptions() map[string]string {
	return map[string]string{
		"return_audio_data":   "false",
		"log_api_calls":       "false",
		"log_ort_run":         "false",
		"log_output_text":     "false",
		"save_input_wav_path": "",
		"identify_speakers":   "false",
		"word_timestamps":     "false",
		"ort_providers":       "CPU",
	}
}
