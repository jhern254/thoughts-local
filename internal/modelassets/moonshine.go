package modelassets

// MoonshineSmallStreamingEnglish selects the approved English streaming STT model.
const MoonshineSmallStreamingEnglish ModelID = "moonshine-small-streaming-en"

const moonshineSmallStreamingDownloadBase = "https://download.moonshine.ai/model/small-streaming-en/quantized_26_08_21/"

// The v0.1.5 catalog at upstream commit 234f60faa0eb388b01cdf7e60aca232af37aefda
// names immutable dated directories. These SHA-256 digests were calculated from
// actual CDN bytes and sizes checked against its generated model-file metadata.
// The optional attention decoder is unnecessary with word timestamps disabled.
func moonshineSmallStreamingManifest() modelManifest {
	return modelManifest{
		ID:       MoonshineSmallStreamingEnglish,
		Kind:     "speech-to-text",
		Runtime:  "moonshine-voice-0.1.5",
		Revision: "quantized_26_08_21",
		Files: []modelFileManifest{
			{
				DownloadURL:       moonshineSmallStreamingDownloadBase + "adapter.ort",
				RelativePath:      "adapter.ort",
				ExpectedSizeBytes: 2870368,
				SHA256Hex:         "c665f742364febad597cc9ac1e0b341ffbee0e24a1466e2f3bde95e6e4771762",
			},
			{
				DownloadURL:       moonshineSmallStreamingDownloadBase + "cross_kv.ort",
				RelativePath:      "cross_kv.ort",
				ExpectedSizeBytes: 5356536,
				SHA256Hex:         "e2d3417144e9514055ebfefe8dcc4c0a55a55adcb8530435844c75c53e352bf6",
			},
			{
				DownloadURL:       moonshineSmallStreamingDownloadBase + "decoder_kv.ort",
				RelativePath:      "decoder_kv.ort",
				ExpectedSizeBytes: 81878600,
				SHA256Hex:         "1a05465b1dd955858dfcbee039c0020fb5dd982b0f5094c34e61735d518d771b",
			},
			{
				DownloadURL:       moonshineSmallStreamingDownloadBase + "encoder.ort",
				RelativePath:      "encoder.ort",
				ExpectedSizeBytes: 44148576,
				SHA256Hex:         "2d4d973e91e8aca08c51e7e7efa28a46ab265b63d809d5294d18b86bcd85b993",
			},
			{
				DownloadURL:       moonshineSmallStreamingDownloadBase + "frontend.model.ort",
				RelativePath:      "frontend.model.ort",
				ExpectedSizeBytes: 26944,
				SHA256Hex:         "09b1210ae30dc5f0f3e45f0ebab914c254741323114f53fbbe5ae62cca35058f",
			},
			{
				DownloadURL:       moonshineSmallStreamingDownloadBase + "frontend.weights.ort",
				RelativePath:      "frontend.weights.ort",
				ExpectedSizeBytes: 7769464,
				SHA256Hex:         "7ef97521bd4bad3928f5bb6808586f4fcc6e92bd5990394112eed7d4052ec338",
			},
			{
				DownloadURL:       moonshineSmallStreamingDownloadBase + "streaming_config.json",
				RelativePath:      "streaming_config.json",
				ExpectedSizeBytes: 512,
				SHA256Hex:         "26f02b6afb22d60871a5efd85c3d38e569cc0ddb6c5eb6e93d3260152ae8a47a",
			},
			{
				DownloadURL:       moonshineSmallStreamingDownloadBase + "tokenizer.bin",
				RelativePath:      "tokenizer.bin",
				ExpectedSizeBytes: 249974,
				SHA256Hex:         "6884b35fd6377d4c4d32336a0bc152f36b64d1e45b6503683cdc238250a8472d",
			},
		},
		Attribution: "Moonshine AI / Useful Sensors, Inc.; MIT; https://github.com/moonshine-ai/moonshine/blob/v0.1.5/LICENSE",
	}
}
