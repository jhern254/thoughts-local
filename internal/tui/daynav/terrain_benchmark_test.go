package daynav

import (
	"fmt"
	"testing"
	"time"
)

func BenchmarkDayMountainCachedNavigation(b *testing.B) {
	today := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	for _, height := range []int{27, 29, 31} {
		b.Run(fmt.Sprint(height), func(b *testing.B) {
			reader := &readerStub{start: today.AddDate(-1, 0, 0)}
			m := New(b.Context(), "u", reader)
			m.now = func() time.Time { return today.Add(12 * time.Hour) }
			m.Resize(80, height)
			m = m.Update(activityReply(m.Sync(today, true, false)))
			defer m.Close()
			reads := reader.reads
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				if cmd := m.Sync(today.AddDate(0, 0, -i%10), true, false); cmd != nil {
					b.Fatal("cached navigation queried activity")
				}
				_ = m.View()
				i++
			}
			if reader.reads != reads {
				b.Fatal("cached navigation performed reads")
			}
		})
	}
}
