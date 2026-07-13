package tests_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"kita-be/internal/platform/httpserver"
	"kita-be/internal/platform/response"
)

func TestHealthEndpoint(t *testing.T) {
	app := httpserver.New()
	app.Get("/api/v1/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service": "test-service",
			"status":  "healthy",
		})
	})

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to test health endpoint: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["status"] != "healthy" {
		t.Errorf("expected status healthy, got %v", body["status"])
	}
}

func TestReadyEndpointWhenDependencyOK(t *testing.T) {
	app := httpserver.New()
	app.Get("/api/v1/ready", func(c *fiber.Ctx) error {
		return response.OK(c, fiber.Map{
			"service": "test-service",
			"status":  "ready",
		})
	})

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/ready", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to test ready endpoint: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	app := httpserver.New()
	app.Get("/api/v1/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/health", nil)
	req.Header.Set("X-Request-ID", "test-req-123")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to test request ID middleware: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	respReqID := resp.Header.Get("X-Request-ID")
	if respReqID != "test-req-123" {
		t.Errorf("expected request ID test-req-123, got %s", respReqID)
	}
}

func TestResponseFormatConsistency(t *testing.T) {
	app := httpserver.New()
	app.Get("/api/v1/health", func(c *fiber.Ctx) error {
		return response.OK(c, fiber.Map{
			"service": "test",
			"status":  "healthy",
		})
	})

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}

	var body response.APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !body.Success {
		t.Error("expected success true")
	}
	if body.Data == nil {
		t.Error("expected data not nil")
	}
}

func TestErrorResponseFormatConsistency(t *testing.T) {
	app := httpserver.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return response.BadRequest(c, "VALIDATION_ERROR", "invalid input")
	})

	req := httptest.NewRequest(fiber.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}

	var body response.APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body.Success {
		t.Error("expected success false")
	}
	if body.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("expected error code VALIDATION_ERROR, got %s", body.Error.Code)
	}
}
