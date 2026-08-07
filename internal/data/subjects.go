package data

import (
	"strings"

	"github.com/jhern254/go-thoughts/internal/validator"
)

const maxSubjectNameBytes = 120

func ValidateSubjectCreate(v *validator.Validator, subject *Subject) {
	subject.SubjectName = strings.Join(strings.Fields(strings.TrimSpace(subject.SubjectName)), " ")
	v.Check(subject.UserID != "", "user_id", "must be provided")
	v.Check(subject.SubjectName != "", "subject_name", "must be provided")
	v.Check(len(subject.SubjectName) <= maxSubjectNameBytes, "subject_name", "must not be more than 120 bytes long")
}
