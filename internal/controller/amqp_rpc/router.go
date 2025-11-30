package v1

import (
	v1 "pod-backend/internal/controller/amqp_rpc/v1"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
	"pod-backend/pkg/rabbitmq/rmq_rpc/server"
)

// NewRouter -.
func NewRouter(t usecase.Translation, l logger.Interface) map[string]server.CallHandler {
	routes := make(map[string]server.CallHandler)

	{
		v1.NewTranslationRoutes(routes, t, l)
	}

	return routes
}
