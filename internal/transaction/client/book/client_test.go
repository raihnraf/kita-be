package book_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	bookclient "kita-be/internal/transaction/client/book"
)

func TestClientGetBook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/books/book-1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"isbn":"978","title":"Clean Architecture","author":"Robert C. Martin"}}`))
	}))
	defer server.Close()

	client := bookclient.NewClient(server.URL, "internal-token")
	book, err := client.GetBook(context.Background(), "book-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if book.ISBN != "978" || book.Title != "Clean Architecture" || book.Author != "Robert C. Martin" {
		t.Fatalf("unexpected book snapshot: %+v", book)
	}
}

func TestClientReadyReturnsErrorWhenServiceNotReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := bookclient.NewClient(server.URL, "internal-token")
	if err := client.Ready(context.Background()); err == nil {
		t.Fatal("expected readiness error")
	}
}
