package domain_test

import (
	"testing"

	domain "kita-be/internal/book/domain"
)

func TestNewBook(t *testing.T) {
	book := domain.NewBook("id-1", "978-test", "Test Book", "Author", 5)

	if book.TotalStock != 5 {
		t.Errorf("expected total_stock 5, got %d", book.TotalStock)
	}
	if book.AvailableStock != 5 {
		t.Errorf("expected available_stock 5, got %d", book.AvailableStock)
	}
	if book.Status != domain.BookStatusAvailable {
		t.Errorf("expected status AVAILABLE, got %s", book.Status)
	}
	if !book.CanBorrow() {
		t.Error("expected CanBorrow true")
	}
}

func TestNewBookZeroStock(t *testing.T) {
	book := domain.NewBook("id-1", "978-test", "Test Book", "Author", 0)

	if book.Status != domain.BookStatusOutOfStock {
		t.Errorf("expected status OUT_OF_STOCK, got %s", book.Status)
	}
	if book.CanBorrow() {
		t.Error("expected CanBorrow false")
	}
}

func TestDecreaseStock(t *testing.T) {
	book := domain.NewBook("id-1", "978-test", "Test Book", "Author", 3)

	err := book.DecreaseStock(2)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if book.AvailableStock != 1 {
		t.Errorf("expected available_stock 1, got %d", book.AvailableStock)
	}
	if !book.CanBorrow() {
		t.Error("expected CanBorrow true after partial decrease")
	}
}

func TestDecreaseStockToZero(t *testing.T) {
	book := domain.NewBook("id-1", "978-test", "Test Book", "Author", 1)

	err := book.DecreaseStock(1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if book.AvailableStock != 0 {
		t.Errorf("expected available_stock 0, got %d", book.AvailableStock)
	}
	if book.Status != domain.BookStatusOutOfStock {
		t.Errorf("expected status OUT_OF_STOCK, got %s", book.Status)
	}
	if book.CanBorrow() {
		t.Error("expected CanBorrow false")
	}
}

func TestDecreaseStockInsufficient(t *testing.T) {
	book := domain.NewBook("id-1", "978-test", "Test Book", "Author", 1)

	err := book.DecreaseStock(2)
	if err == nil {
		t.Fatal("expected error for insufficient stock")
	}
	if book.AvailableStock != 1 {
		t.Errorf("expected available_stock unchanged (1), got %d", book.AvailableStock)
	}
}

func TestIncreaseStock(t *testing.T) {
	book := domain.NewBook("id-1", "978-test", "Test Book", "Author", 3)
	if err := book.DecreaseStock(3); err != nil {
		t.Fatalf("failed to arrange stock decrease: %v", err)
	}

	if err := book.IncreaseStock(3); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if book.AvailableStock != 3 {
		t.Errorf("expected available_stock 3, got %d", book.AvailableStock)
	}
	if book.Status != domain.BookStatusAvailable {
		t.Errorf("expected status AVAILABLE, got %s", book.Status)
	}
	if !book.CanBorrow() {
		t.Error("expected CanBorrow true")
	}
}

func TestIncreaseStockExceedsTotal(t *testing.T) {
	book := domain.NewBook("id-1", "978-test", "Test Book", "Author", 1)

	err := book.IncreaseStock(1)
	if err == nil {
		t.Fatal("expected error when stock increase exceeds total")
	}
	if book.AvailableStock != 1 {
		t.Errorf("expected available_stock unchanged (1), got %d", book.AvailableStock)
	}
}
