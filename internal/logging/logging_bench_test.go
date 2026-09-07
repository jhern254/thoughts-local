package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/rs/zerolog"
)

// Caller-off construction is a benchmark control, not a feature-code escape hatch.
func benchmarkLoggers(tb testing.TB, out io.Writer, level string, caller bool) (zerolog.Logger, Logger) {
	tb.Helper()
	parsed, err := zerolog.ParseLevel(level)
	if err != nil {
		tb.Fatal(err)
	}
	context := zerolog.New(out).With().Timestamp().Str("application", "benchmark")
	direct := context.Logger().Level(parsed)
	if caller {
		direct = direct.With().CallerWithSkipFrameCount(2).Logger()
		adapter, err := New(out, "benchmark", level)
		if err != nil {
			tb.Fatal(err)
		}
		return direct, adapter
	}
	if parsed == zerolog.Disabled {
		return direct, Nop()
	}
	raw := context.Logger().Level(parsed)
	return direct, Logger{logger: &raw}
}

func BenchmarkLogging(b *testing.B) {
	for _, event := range []string{"Mutation", "Failure"} {
		b.Run(event, func(b *testing.B) {
			for _, mode := range []struct {
				name   string
				level  string
				caller bool
			}{
				{"caller", "info", true},
				{"no-caller", "info", false},
				{"filtered", "fatal", true},
				{"disabled", "disabled", true},
			} {
				// Error-level loggers filter Info mutations; fatal filters Error failures.
				if event == "Mutation" && mode.name == "filtered" {
					mode.level = "error"
				}
				b.Run(mode.name, func(b *testing.B) {
					b.Run("direct", func(b *testing.B) {
						logger, _ := benchmarkLoggers(b, io.Discard, mode.level, mode.caller)
						b.ReportAllocs()
						if event == "Mutation" {
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								logger.Info().Int64("subject_id", int64(i)).Msg("subject created")
							}
						} else {
							operations := [...]string{"subject_create", "subject_get"}
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								logger.Error().Str("operation", operations[i&1]).Str("category", "unexpected_failure").Msg("operation failed")
							}
						}
					})
					b.Run("adapter", func(b *testing.B) {
						_, logger := benchmarkLoggers(b, io.Discard, mode.level, mode.caller)
						b.ReportAllocs()
						if event == "Mutation" {
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								logger.Mutation(SubjectCreated, int64(i))
							}
						} else {
							operations := [...]Operation{SubjectCreate, SubjectGet}
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								logger.Failure(operations[i&1], UnexpectedFailure)
							}
						}
					})
				})
			}
		})
	}
}

func TestBenchmarkLogging_EquivalentEvents(t *testing.T) {
	for _, caller := range []bool{true, false} {
		for _, event := range []string{"mutation", "failure"} {
			t.Run(fmt.Sprintf("%s caller=%t", event, caller), func(t *testing.T) {
				var output bytes.Buffer
				direct, adapter := benchmarkLoggers(t, &output, "info", caller)
				var file string
				var directLine, adapterLine int
				if event == "mutation" {
					_, file, directLine, _ = runtime.Caller(0)
					direct.Info().Int64("subject_id", 7).Msg("subject created")
					_, _, adapterLine, _ = runtime.Caller(0)
					adapter.Mutation(SubjectCreated, 7)
				} else {
					_, file, directLine, _ = runtime.Caller(0)
					direct.Error().Str("operation", "subject_get").Str("category", "unexpected_failure").Msg("operation failed")
					_, _, adapterLine, _ = runtime.Caller(0)
					adapter.Failure(SubjectGet, UnexpectedFailure)
				}
				decoder := json.NewDecoder(&output)
				events := make([]map[string]any, 2)
				for index, line := range []int{directLine, adapterLine} {
					if err := decoder.Decode(&events[index]); err != nil {
						t.Fatal(err)
					}
					value, exists := events[index]["caller"]
					if caller {
						if !exists || filepath.Base(value.(string)) != fmt.Sprintf("%s:%d", filepath.Base(file), line+1) {
							t.Fatalf("wrong application caller: %v", value)
						}
					} else if exists {
						t.Fatalf("unexpected caller: %v", value)
					}
					if events[index]["time"] == nil {
						t.Fatal("missing timestamp")
					}
					delete(events[index], "caller")
					delete(events[index], "time")
				}
				if !reflect.DeepEqual(events[0], events[1]) {
					t.Fatalf("different events: %v", events)
				}
				if err := decoder.Decode(new(any)); err != io.EOF {
					t.Fatalf("unexpected extra output: %v", err)
				}
			})
		}
	}
}
