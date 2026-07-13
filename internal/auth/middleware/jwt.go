package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	jwtsvc "kita-be/internal/auth/jwt"
	"kita-be/internal/platform/response"
)

func JWTAuth(jwtService *jwtsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return response.Unauthorized(c, "UNAUTHORIZED", "missing authorization header")
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			return response.Unauthorized(c, "UNAUTHORIZED", "invalid authorization header format")
		}

		tokenStr := parts[1]

		claims, err := jwtService.ValidateAccessToken(tokenStr)
		if err != nil {
			return response.Unauthorized(c, "UNAUTHORIZED", "invalid or expired token")
		}

		c.Locals("user_id", claims.UserID)
		c.Locals("email", claims.Email)
		c.Locals("role", claims.Role)

		return c.Next()
	}
}
