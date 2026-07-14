package tests_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAsyncBorrowReturnLiveFlow(t *testing.T) {
	if os.Getenv("RUN_LIVE_BACKEND_INTEGRATION") != "1" {
		t.Skip("set RUN_LIVE_BACKEND_INTEGRATION=1 to run live backend integration flow")
	}

	identityURL := getenvOrDefault("KITA_IDENTITY_URL", "http://localhost:3000")
	bookURL := getenvOrDefault("KITA_BOOK_URL", "http://localhost:3001")
	transactionURL := getenvOrDefault("KITA_TRANSACTION_URL", "http://localhost:3002")

	client := &http.Client{Timeout: 10 * time.Second}
	email := fmt.Sprintf("integration_user_%d@example.com", time.Now().UnixNano())
	password := "securePassword123"

	registerPayload := map[string]string{
		"full_name": "Integration User",
		"email":     email,
		"password":  password,
	}

	registerResp, err := postJSON[registerResponse](client, identityURL+"/api/v1/auth/register", registerPayload, "")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if !registerResp.Success || registerResp.Data.AccessToken == "" {
		t.Fatalf("unexpected register response: %+v", registerResp)
	}

	loginResp, err := postForm[tokenResponse](client, identityURL+"/api/v1/auth/token", "grant_type=password&email="+email+"&password="+password)
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if !loginResp.Success || loginResp.Data.AccessToken == "" {
		t.Fatalf("unexpected login response: %+v", loginResp)
	}
	token := loginResp.Data.AccessToken

	booksResp, booksStatus, err := getJSON[bookListResponse](client, bookURL+"/api/v1/books?page=1&per_page=10", "")
	if err != nil {
		t.Fatalf("list books failed: %v", err)
	}
	if booksStatus != http.StatusOK {
		t.Fatalf("expected books status 200, got %d", booksStatus)
	}

	var selectedBook bookSummary
	foundBook := false
	for _, book := range booksResp.Data {
		if book.AvailableStock > 0 {
			selectedBook = book
			foundBook = true
			break
		}
	}
	if !foundBook {
		t.Fatal("no borrowable book found for live integration test")
	}

	borrowPayload := map[string]string{
		"book_id":         selectedBook.ID,
		"idempotency_key": fmt.Sprintf("borrow-%d", time.Now().UnixNano()),
	}
	borrowResp, borrowStatus, err := postJSONWithStatus[transactionResponse](client, transactionURL+"/api/v1/transactions/borrow", borrowPayload, token)
	if err != nil {
		t.Fatalf("borrow failed: %v", err)
	}
	if borrowStatus != http.StatusAccepted {
		t.Fatalf("expected borrow status 202, got %d", borrowStatus)
	}
	if !borrowResp.Success || borrowResp.Data.ID == "" || borrowResp.Data.Status != "PENDING" {
		t.Fatalf("unexpected borrow response: %+v", borrowResp)
	}

	borrowFinal, err := pollTransactionStatus(client, transactionURL, token, borrowResp.Data.ID, 15, "ACTIVE", "CANCELLED")
	if err != nil {
		t.Fatalf("borrow finalization failed: %v", err)
	}
	if borrowFinal.Status != "ACTIVE" {
		t.Fatalf("expected borrow to become ACTIVE, got %s", borrowFinal.Status)
	}

	availabilityAfterBorrow, availabilityStatus, err := getJSON[availabilityResponse](client, bookURL+"/api/v1/books/"+selectedBook.ID+"/availability", "")
	if err != nil {
		t.Fatalf("availability after borrow failed: %v", err)
	}
	if availabilityStatus != http.StatusOK {
		t.Fatalf("expected availability status 200 after borrow, got %d", availabilityStatus)
	}
	if availabilityAfterBorrow.Data.AvailableStock != selectedBook.AvailableStock-1 {
		t.Fatalf("expected stock %d after borrow, got %d", selectedBook.AvailableStock-1, availabilityAfterBorrow.Data.AvailableStock)
	}

	returnPayload := map[string]string{
		"idempotency_key": fmt.Sprintf("return-%d", time.Now().UnixNano()),
	}
	returnResp, returnStatus, err := postJSONWithStatus[transactionResponse](client, transactionURL+"/api/v1/transactions/"+borrowResp.Data.ID+"/return", returnPayload, token)
	if err != nil {
		t.Fatalf("return failed: %v", err)
	}
	if returnStatus != http.StatusAccepted {
		t.Fatalf("expected return status 202, got %d", returnStatus)
	}
	if !returnResp.Success || returnResp.Data.Status != "RETURN_PENDING" {
		t.Fatalf("unexpected return response: %+v", returnResp)
	}

	returnFinal, err := pollTransactionStatus(client, transactionURL, token, borrowResp.Data.ID, 15, "RETURNED", "RETURNED_LATE", "ACTIVE")
	if err != nil {
		t.Fatalf("return finalization failed: %v", err)
	}
	if returnFinal.Status != "RETURNED" && returnFinal.Status != "RETURNED_LATE" {
		t.Fatalf("expected return to finish as RETURNED/RETURNED_LATE, got %s", returnFinal.Status)
	}

	availabilityAfterReturn, availabilityAfterReturnStatus, err := getJSON[availabilityResponse](client, bookURL+"/api/v1/books/"+selectedBook.ID+"/availability", "")
	if err != nil {
		t.Fatalf("availability after return failed: %v", err)
	}
	if availabilityAfterReturnStatus != http.StatusOK {
		t.Fatalf("expected availability status 200 after return, got %d", availabilityAfterReturnStatus)
	}
	if availabilityAfterReturn.Data.AvailableStock != selectedBook.AvailableStock {
		t.Fatalf("expected stock %d after return, got %d", selectedBook.AvailableStock, availabilityAfterReturn.Data.AvailableStock)
	}
	if returnFinal.Status == "ACTIVE" {
		t.Fatal("return unexpectedly reverted to ACTIVE in happy-path live flow")
	}
	_ = registerResp
	_ = borrowFinal
	_ = returnFinal
}

func pollTransactionStatus(client *http.Client, transactionURL, token, transactionID string, maxRetries int, allowed ...string) (*transactionData, error) {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, status := range allowed {
		allowedSet[status] = struct{}{}
	}

	for i := 0; i < maxRetries; i++ {
		time.Sleep(1 * time.Second)
		resp, statusCode, err := getJSON[transactionResponse](client, transactionURL+"/api/v1/transactions/"+transactionID, token)
		if err != nil {
			return nil, err
		}
		if statusCode != http.StatusOK {
			return nil, fmt.Errorf("expected transaction detail status 200, got %d", statusCode)
		}
		if _, ok := allowedSet[resp.Data.Status]; ok {
			return &resp.Data, nil
		}
	}

	return nil, fmt.Errorf("transaction %s did not reach expected statuses %v", transactionID, allowed)
}

func postJSON[T any](client *http.Client, url string, payload any, token string) (*T, error) {
	resp, _, err := postJSONWithStatus[T](client, url, payload, token)
	return resp, err
}

func postJSONWithStatus[T any](client *http.Client, url string, payload any, token string) (*T, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return doJSON[T](client, req)
}

func postForm[T any](client *http.Client, url, body string) (*T, error) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _, err := doJSON[T](client, req)
	return resp, err
}

func getJSON[T any](client *http.Client, url, token string) (*T, int, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return doJSON[T](client, req)
}

func doJSON[T any](client *http.Client, req *http.Request) (*T, int, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, resp.StatusCode, err
	}
	return &out, resp.StatusCode, nil
}

func getenvOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

type registerResponse struct {
	Success bool `json:"success"`
	Data    struct {
		AccessToken string `json:"access_token"`
	} `json:"data"`
}

type tokenResponse struct {
	Success bool `json:"success"`
	Data    struct {
		AccessToken string `json:"access_token"`
	} `json:"data"`
}

type bookListResponse struct {
	Success bool          `json:"success"`
	Data    []bookSummary `json:"data"`
}

type bookSummary struct {
	ID             string `json:"id"`
	AvailableStock int    `json:"available_stock"`
}

type transactionResponse struct {
	Success bool            `json:"success"`
	Data    transactionData `json:"data"`
}

type transactionData struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type availabilityResponse struct {
	Success bool `json:"success"`
	Data    struct {
		AvailableStock int `json:"available_stock"`
	} `json:"data"`
}
