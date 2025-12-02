package v1

import (
	"github.com/go-playground/validator/v10"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
	"pod-backend/pkg/nats/nats_rpc/server"
)

// NewTranslationRoutes -.
func NewTranslationRoutes(routes map[string]server.CallHandler, t usecase.Translation, l logger.Interface) {
	r := &V1{t: t, l: l, v: validator.New(validator.WithRequiredStructEnabled())}

	{
		routes["v1.getHistory"] = r.getHistory()
	}
}
