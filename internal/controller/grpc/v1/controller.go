package v1

import (
	"github.com/go-playground/validator/v10"
	v1 "pod-backend/docs/proto/v1"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
)

// V1 -.
type V1 struct {
	v1.TranslationServer

	t usecase.Translation
	l logger.Interface
	v *validator.Validate
}
