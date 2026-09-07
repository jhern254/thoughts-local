package main

import "github.com/rs/zerolog"

// Only a single causal chain establishes one expected outcome. Aggregated errors
// may also contain infrastructure failures, so they use the generic fallback.
func diagnosticCause(err error) error {
	for range 32 {
		switch e := err.(type) {
		case interface{ Unwrap() []error }, interface{ Errors() []error }:
			return nil
		case interface{ Unwrap() error }:
			err = e.Unwrap()
		default:
			return err
		}
	}
	return nil
}

func safeValidationFields(fields map[string]string) map[string]string {
	safe := make(map[string]string)
	for field, message := range fields {
		switch {
		case field == "user_id" && message == "must be provided",
			field == "subject_name" && message == "must be between 1 and 255 characters long",
			field == "thought" && (message == "must be provided" || message == "must not be more than 1000000 characters long"):
			safe[field] = message
		default:
			return map[string]string{"request": "The submitted values are invalid."}
		}
	}
	if len(safe) == 0 {
		return map[string]string{"request": "The submitted values are invalid."}
	}
	return safe
}

// net/http sends already-formatted diagnostics here. Do not forward their bytes.
// Its existing recovery and connection handling remain unchanged.
type serverDiagnosticWriter struct{ logger zerolog.Logger }

func (w serverDiagnosticWriter) Write(p []byte) (int, error) {
	w.logger.Error().Str("operation", "http_server").Msg("HTTP server diagnostic")
	return len(p), nil
}
