package v1

import (
	"github.com/go-playground/validator/v10"
	pbgrpc "google.golang.org/grpc"
	v1 "pod-backend/docs/proto/v1"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
)

// NewTranslationRoutes -.
func NewTranslationRoutes(app *pbgrpc.Server, t usecase.Translation, l logger.Interface) {
	r := &V1{t: t, l: l, v: validator.New(validator.WithRequiredStructEnabled())}

	{
		v1.RegisterTranslationServer(app, r)
	}
}
