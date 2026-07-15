package book

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"kita-be/internal/platform/middleware"
	domain "kita-be/internal/transaction/domain"
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
			ISBN           string `json:"isbn"`
			Title          string `json:"title"`
			Author         string `json:"author"`
			AvailableStock int    `json:"available_stock"`
			CanBorrow      bool   `json:"can_borrow"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &domain.BookSnapshot{
		ISBN:           result.Data.ISBN,
		Title:          result.Data.Title,
		Author:         result.Data.Author,
		AvailableStock: result.Data.AvailableStock,
		CanBorrow:      result.Data.CanBorrow,
	}, nil
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
