package data

import (
	"testing"
	"time"
)

func TestTimeFromUnixSec(t *testing.T) {
	got := TimeFromUnixSec(1_700_000_000)
	want := time.Date(2023, time.November, 14, 22, 13, 20, 0, time.UTC)

	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got.Location() != time.UTC {
		t.Errorf("got location %v, want UTC", got.Location())
	}
}

func TestUnixSec(t *testing.T) {
	input := time.Date(2023, time.November, 14, 14, 13, 20, 0, time.FixedZone("PST", -8*60*60))

	got := UnixSec(input)
	const want int64 = 1_700_000_000

	if got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

func TestScanUnixNullable(t *testing.T) {
	t.Run("nil returns zero time", func(t *testing.T) {
		got, err := ScanUnixNullable(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.IsZero() {
			t.Errorf("got %v, want zero time", got)
		}
	})

	t.Run("integer returns UTC time", func(t *testing.T) {
		got, err := ScanUnixNullable(int64(1_700_000_000))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := time.Date(2023, time.November, 14, 22, 13, 20, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
		if got.Location() != time.UTC {
			t.Errorf("got location %v, want UTC", got.Location())
		}
	})

	tests := []struct {
		name  string
		input any
	}{
		{name: "bytes", input: []byte("1700000000")},
		{name: "string", input: "1700000000"},
		{name: "unsupported type", input: float64(1_700_000_000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ScanUnixNullable(tt.input)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}
