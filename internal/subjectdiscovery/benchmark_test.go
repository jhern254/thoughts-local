package subjectdiscovery

import (
	"context"
	"fmt"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

func BenchmarkSpokenPrefix(b *testing.B) {
	for _, text := range []string{"Programming in Go thought, today I learned about interfaces.", "I thought this was interesting."} {
		b.Run(text, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				ParseSpokenPrefix(text)
			}
		})
	}
}
func BenchmarkResolve(b *testing.B) {
	for _, size := range []int{100, 1000} {
		candidates := make([]Candidate, size)
		for index := range candidates {
			candidates[index] = Candidate{Subject: data.Subject{SubjectID: int64(index + 1), SubjectName: fmt.Sprintf("Subject %d", index)}, Aliases: []string{fmt.Sprintf("Topic %d", index)}}
		}
		candidates[size-1] = Candidate{Subject: data.Subject{SubjectID: int64(size), SubjectName: "Go Programming"}, Aliases: []string{"Programming in Go"}}
		snapshot := NewSnapshot(candidates)
		service := &Service{recommender: NameRecommender{}}
		for _, cue := range []string{"Programming in Go", "Go programming practice"} {
			b.Run(fmt.Sprintf("%d/%s", size, cue), func(b *testing.B) {
				ctx := context.Background()
				b.ReportAllocs()
				for b.Loop() {
					if _, err := service.Resolve(ctx, cue, snapshot); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
