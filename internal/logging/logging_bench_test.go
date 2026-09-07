package logging_test

import (
	"io"
	"testing"

	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/rs/zerolog"
)

func BenchmarkMutation(b *testing.B) {
	for _, level := range []string{"info", "disabled"} {
		b.Run(level, func(b *testing.B) {
			b.Run("direct", func(b *testing.B) {
				parsed, err := zerolog.ParseLevel(level)
				if err != nil {
					b.Fatal(err)
				}
				// The direct call has one fewer frame than the adapter. Both report
				// the application caller and otherwise use identical event settings.
				logger := zerolog.New(io.Discard).With().Timestamp().
					CallerWithSkipFrameCount(2).Str("application", "benchmark").
					Logger().Level(parsed)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					logger.Info().Int64("subject_id", 7).Msg("subject created")
				}
			})
			b.Run("adapter", func(b *testing.B) {
				logger, err := logging.New(io.Discard, "benchmark", level)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					logger.Mutation(logging.SubjectCreated, 7)
				}
			})
		})
	}
}
