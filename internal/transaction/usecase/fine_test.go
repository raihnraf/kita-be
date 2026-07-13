package usecase_test

import (
	"testing"
	"time"

	"kita-be/internal/transaction/usecase"
)

func TestFineCalculatorOnTime(t *testing.T) {
	fc := usecase.NewFineCalculator(50000)

	dueAt := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	returnedAt := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)

	lateDays, fine := fc.Calculate(dueAt, returnedAt)
	if lateDays != 0 {
		t.Errorf("expected 0 late days, got %d", lateDays)
	}
	if fine != 0 {
		t.Errorf("expected 0 fine cents, got %d", fine)
	}
}

func TestFineCalculatorExactDueDate(t *testing.T) {
	fc := usecase.NewFineCalculator(50000)

	dueAt := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	returnedAt := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)

	lateDays, fine := fc.Calculate(dueAt, returnedAt)
	if lateDays != 0 {
		t.Errorf("expected 0 late days for same day, got %d", lateDays)
	}
	if fine != 0 {
		t.Errorf("expected 0 fine cents, got %d", fine)
	}
}

func TestFineCalculatorLateByOneDay(t *testing.T) {
	fc := usecase.NewFineCalculator(50000)

	dueAt := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	returnedAt := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)

	lateDays, fine := fc.Calculate(dueAt, returnedAt)
	if lateDays != 1 {
		t.Errorf("expected 1 late day, got %d", lateDays)
	}
	if fine != 50000 {
		t.Errorf("expected fine 50000 cents, got %d", fine)
	}
}

func TestFineCalculatorLateByFiveDays(t *testing.T) {
	fc := usecase.NewFineCalculator(50000)

	dueAt := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	returnedAt := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)

	lateDays, fine := fc.Calculate(dueAt, returnedAt)
	if lateDays != 5 {
		t.Errorf("expected 5 late days, got %d", lateDays)
	}
	if fine != 250000 {
		t.Errorf("expected fine 250000 cents, got %d", fine)
	}
}

func TestFineCalculatorLateByPartialDay(t *testing.T) {
	fc := usecase.NewFineCalculator(50000)

	dueAt := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	returnedAt := time.Date(2026, 7, 11, 0, 1, 0, 0, time.UTC)

	lateDays, fine := fc.Calculate(dueAt, returnedAt)
	if lateDays != 1 {
		t.Errorf("expected 1 late day for partial day, got %d", lateDays)
	}
	if fine != 50000 {
		t.Errorf("expected fine 50000 cents, got %d", fine)
	}
}
