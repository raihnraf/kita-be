package book

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	domain "kita-be/internal/transaction/domain"
	"kita-be/internal/platform/middleware"
)

type Client struct {
	baseURL  string
	apiToken string
	client   *http.Client
}

func NewClient(baseURL, apiToken string) *Client {
	return &Client{
		baseURL:  baseURL,
		apiToken: apiToken,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type StockChangeRequest struct {
	Quantity      int    `json:"quantity"`
	TransactionID string `json:"transaction_id"`
}

type StockChangeResponse struct {
	EventID string `json:"event_id"`
	Status  string `json:"status"`
}

func (c *Client) GetBook(ctx context.Context, bookID string) (*domain.BookSnapshot, error) {
	url := fmt.Sprintf("%s/api/v1/books/%s", c.baseURL, bookID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if reqID, ok := ctx.Value(middleware.RequestIDKey).(string); ok && reqID != "" {
		req.Header.Set("X-Request-ID", reqID)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Success bool `json:"success"`
			Error   struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("book service error: %s - %s", errResp.Error.Code, errResp.Error.Message)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			ISBN   string `json:"isbn"`
			Title  string `json:"title"`
			Author string `json:"author"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &domain.BookSnapshot{
		ISBN:   result.Data.ISBN,
		Title:  result.Data.Title,
		Author: result.Data.Author,
	}, nil
}

func (c *Client) DecreaseStock(ctx context.Context, bookID string, qty int, txnID string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/internal/books/%s/stock/decrease", c.baseURL, bookID)

	body := StockChangeRequest{
		Quantity:      qty,
		TransactionID: txnID,
	}

	eventID, err := c.doRequest(ctx, url, body)
	if err != nil {
		return "", fmt.Errorf("failed to decrease stock: %w", err)
	}

	return eventID, nil
}

func (c *Client) IncreaseStock(ctx context.Context, bookID string, qty int, txnID string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/internal/books/%s/stock/increase", c.baseURL, bookID)

	body := StockChangeRequest{
		Quantity:      qty,
		TransactionID: txnID,
	}

	eventID, err := c.doRequest(ctx, url, body)
	if err != nil {
		return "", fmt.Errorf("failed to increase stock: %w", err)
	}

	return eventID, nil
}

func (c *Client) Ready(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/ready", nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("book service readiness returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) doRequest(ctx context.Context, url string, body StockChangeRequest) (string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.apiToken)

	if reqID, ok := ctx.Value(middleware.RequestIDKey).(string); ok && reqID != "" {
		req.Header.Set("X-Request-ID", reqID)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Success bool `json:"success"`
			Error   struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return "", fmt.Errorf("book service error: %s - %s", errResp.Error.Code, errResp.Error.Message)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			EventID string `json:"event_id"`
			Status  string `json:"status"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Data.EventID, nil
}
