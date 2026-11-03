package goals

import (
	"context"
	"fmt"
	"github.com/jhern254/go-thoughts/internal/data"
)

func readNumberedGoalSubjects(ctx context.Context, memberships Memberships, userID string, goalID int64) ([]data.Subject, map[int64]int64, error) {
	refs, err := memberships.ListLiveNumberedSubjectsForGoal(ctx, userID, goalID)
	if err != nil {
		return nil, nil, err
	}
	subjects := make([]data.Subject, 0, len(refs))
	numbers := make(map[int64]int64, len(refs))
	for _, ref := range refs {
		subjects = append(subjects, ref.Subject)
		numbers[ref.Subject.SubjectID] = ref.GoalNumber
	}
	return subjects, numbers, nil
}

func numberedSubjectNames(subjects []data.Subject, numbers map[int64]int64) []string {
	names := subjectNames(subjects)
	for i, subject := range subjects {
		names[i] = fmt.Sprintf("%s %d", subject.SubjectName, numbers[subject.SubjectID])
	}
	return names
}
