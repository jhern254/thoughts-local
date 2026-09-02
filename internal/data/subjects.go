package data

import (
	"strings"
	"unicode/utf8"

	"github.com/jhern254/go-thoughts/internal/validator"
)

const maxSubjectNameCharacters = 255

func ValidateSubjectCreate(v *validator.Validator, subject *Subject) {
	trimmedName := strings.Trim(subject.SubjectName, " ")
	nameLength := utf8.RuneCountInString(trimmedName)
	v.Check(subject.UserID != "", "user_id", "must be provided")
	v.Check(
		nameLength >= 1 && nameLength <= maxSubjectNameCharacters,
		"subject_name",
		"must be between 1 and 255 characters long",
	)
}
