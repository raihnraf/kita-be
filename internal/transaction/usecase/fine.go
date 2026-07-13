package usecase

import (
	"time"
)

type FineCalculator struct {
	dailyFineAmountCents int64
}

func NewFineCalculator(dailyFineAmountCents int64) *FineCalculator {
	return &FineCalculator{dailyFineAmountCents: dailyFineAmountCents}
}

func (fc *FineCalculator) Calculate(dueAt, returnedAt time.Time) (int, int64) {
	if !returnedAt.After(dueAt) {
		return 0, 0
	}

	dueDate := time.Date(dueAt.Year(), dueAt.Month(), dueAt.Day(), 0, 0, 0, 0, dueAt.Location())
	returnDate := time.Date(returnedAt.Year(), returnedAt.Month(), returnedAt.Day(), 0, 0, 0, 0, returnedAt.Location())

	diff := returnDate.Sub(dueDate)
	lateDays := int(diff.Hours() / 24)

	fine := int64(lateDays) * fc.dailyFineAmountCents

	return lateDays, fine
}
