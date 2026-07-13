package response

import (
	"github.com/gofiber/fiber/v2"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Meta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

func Success(c *fiber.Ctx, status int, data interface{}) error {
	return c.Status(status).JSON(APIResponse{
		Success: true,
		Data:    data,
	})
}

func Created(c *fiber.Ctx, data interface{}) error {
	return Success(c, fiber.StatusCreated, data)
}

func OK(c *fiber.Ctx, data interface{}) error {
	return Success(c, fiber.StatusOK, data)
}

func Error(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(APIResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
		},
	})
}

func BadRequest(c *fiber.Ctx, code, message string) error {
	return Error(c, fiber.StatusBadRequest, code, message)
}

func Unauthorized(c *fiber.Ctx, code, message string) error {
	return Error(c, fiber.StatusUnauthorized, code, message)
}

func Forbidden(c *fiber.Ctx, code, message string) error {
	return Error(c, fiber.StatusForbidden, code, message)
}

func NotFound(c *fiber.Ctx, code, message string) error {
	return Error(c, fiber.StatusNotFound, code, message)
}

func Conflict(c *fiber.Ctx, code, message string) error {
	return Error(c, fiber.StatusConflict, code, message)
}

func InternalError(c *fiber.Ctx, code, message string) error {
	return Error(c, fiber.StatusInternalServerError, code, message)
}

func Paginated(c *fiber.Ctx, data interface{}, meta Meta) error {
	return c.Status(fiber.StatusOK).JSON(APIResponse{
		Success: true,
		Data:    data,
		Meta:    &meta,
	})
}
