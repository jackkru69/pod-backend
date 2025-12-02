package v1

import (
	"github.com/go-playground/validator/v10"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
)

// V1 -.
type V1 struct {
	t usecase.Translation
	l logger.Interface
	v *validator.Validate
}
