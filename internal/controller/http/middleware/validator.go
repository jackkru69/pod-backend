package middleware

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
)

// ValidatorInstance is the global validator instance.
var ValidatorInstance = validator.New()

// ErrorResponse represents a validation error response (FR-011, T108).
type ErrorResponse struct {
	Error   string            `json:"error"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// ValidateStruct validates a struct and returns formatted error response.
// Implements FR-011: validate all incoming data.
func ValidateStruct(data interface{}) error {
	if err := ValidatorInstance.Struct(data); err != nil {
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			fields := make(map[string]string)
			for _, fieldError := range validationErrors {
				fields[strings.ToLower(fieldError.Field())] = formatValidationError(fieldError)
			}
			return &fiber.Error{
				Code:    fiber.StatusBadRequest,
				Message: formatErrorResponse("validation_failed", "Request validation failed", fields),
			}
		}
		return &fiber.Error{
			Code:    fiber.StatusBadRequest,
			Message: fmt.Sprintf(`{"error":"validation_failed","message":"%s"}`, err.Error()),
		}
	}
	return nil
}

// formatValidationError converts validator.FieldError to human-readable message.
func formatValidationError(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", fe.Field())
	case "email":
		return fmt.Sprintf("%s must be a valid email address", fe.Field())
	case "min":
		return fmt.Sprintf("%s must be at least %s", fe.Field(), fe.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s", fe.Field(), fe.Param())
	case "len":
		return fmt.Sprintf("%s must be exactly %s characters", fe.Field(), fe.Param())
	case "tonaddress":
		return fmt.Sprintf("%s must be a valid TON wallet address", fe.Field())
	default:
		return fmt.Sprintf("%s failed validation (%s)", fe.Field(), fe.Tag())
	}
}

// formatErrorResponse creates JSON error response string.
func formatErrorResponse(errorCode, message string, fields map[string]string) string {
	if len(fields) == 0 {
		return fmt.Sprintf(`{"error":"%s","message":"%s"}`, errorCode, message)
	}

	fieldsJSON := "{"
	i := 0
	for k, v := range fields {
		if i > 0 {
			fieldsJSON += ","
		}
		fieldsJSON += fmt.Sprintf(`"%s":"%s"`, k, v)
		i++
	}
	fieldsJSON += "}"

	return fmt.Sprintf(`{"error":"%s","message":"%s","fields":%s}`, errorCode, message, fieldsJSON)
}

// RegisterCustomValidators registers custom validation functions.
// Implements FR-011, T107: validate TON wallet address format.
func RegisterCustomValidators() {
	// TON wallet address validator (T107)
	// Format: EQ[A-Za-z0-9_-]{46} (base64url encoding)
	ValidatorInstance.RegisterValidation("tonaddress", func(fl validator.FieldLevel) bool {
		address := fl.Field().String()
		if len(address) != 48 {
			return false
		}
		if !strings.HasPrefix(address, "EQ") && !strings.HasPrefix(address, "UQ") {
			return false
		}
		// Check base64url characters
		validChars := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"
		for _, char := range address[2:] {
			if !strings.ContainsRune(validChars, char) {
				return false
			}
		}
		return true
	})
}
