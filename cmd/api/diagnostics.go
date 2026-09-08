package main

import "github.com/rs/zerolog"

// net/http sends already-formatted diagnostics here. Do not forward their bytes.
// Its existing recovery and connection handling remain unchanged.
type serverDiagnosticWriter struct{ logger zerolog.Logger }

func (w serverDiagnosticWriter) Write(p []byte) (int, error) {
	w.logger.Error().Str("operation", "http_server").Msg("HTTP server diagnostic")
	return len(p), nil
}
