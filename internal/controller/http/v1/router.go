package v1

import (
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"

	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
)

// ErrorResponse represents an error response for API v1
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// NewTranslationRoutes -.
func NewTranslationRoutes(apiV1Group fiber.Router, t usecase.Translation, l logger.Interface) {
	r := &V1{t: t, l: l, v: validator.New(validator.WithRequiredStructEnabled())}

	translationGroup := apiV1Group.Group("/translation")

	{
		translationGroup.Get("/history", r.history)
		translationGroup.Post("/do-translate", r.doTranslate)
	}
}
