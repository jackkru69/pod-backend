package response

// Error represents an error response
type Error struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
