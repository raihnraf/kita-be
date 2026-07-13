package validation

import (
	"net/mail"

	"github.com/google/uuid"
)

func Email(value string) bool {
	_, err := mail.ParseAddress(value)
	return err == nil
}

func UUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}
