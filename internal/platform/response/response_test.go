package response_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"kita-be/internal/platform/response"
)

func TestSuccessResponse(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return response.OK(c, fiber.Map{"message": "hello"})
	})

	req := httptest.NewRequest(fiber.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var body response.APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !body.Success {
		t.Error("expected success true")
	}
}

func TestCreatedResponse(t *testing.T) {
	app := fiber.New()
	app.Post("/test", func(c *fiber.Ctx) error {
		return response.Created(c, fiber.Map{"id": "123"})
	})

	req := httptest.NewRequest(fiber.MethodPost, "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if resp.StatusCode != fiber.StatusCreated {
		t.Errorf("expected status 201, got %d", resp.StatusCode)
	}
}

func TestErrorResponse(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return response.BadRequest(c, "INVALID", "something is wrong")
	})

	req := httptest.NewRequest(fiber.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}

	var body response.APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body.Success {
		t.Error("expected success false")
	}
	if body.Error == nil {
		t.Fatal("expected error not nil")
	}
	if body.Error.Code != "INVALID" {
		t.Errorf("expected error code INVALID, got %s", body.Error.Code)
	}
}

func TestUnauthorizedResponse(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return response.Unauthorized(c, "UNAUTHORIZED", "invalid token")
	})

	req := httptest.NewRequest(fiber.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

func TestNotFoundResponse(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return response.NotFound(c, "NOT_FOUND", "resource not found")
	})

	req := httptest.NewRequest(fiber.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestPaginatedResponse(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return response.Paginated(c, []string{"a", "b"}, response.Meta{
			Page:       1,
			PerPage:    10,
			Total:      2,
			TotalPages: 1,
		})
	})

	req := httptest.NewRequest(fiber.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var body response.APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !body.Success {
		t.Error("expected success true")
	}
	if body.Meta == nil {
		t.Fatal("expected meta not nil")
	}
	if body.Meta.Page != 1 {
		t.Errorf("expected page 1, got %d", body.Meta.Page)
	}
}
