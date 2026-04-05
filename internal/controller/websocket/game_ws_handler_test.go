package websocket

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pod-backend/internal/entity"
)

func TestParseClientMessage(t *testing.T) {
	t.Parallel()

	msg, err := parseClientMessage([]byte(`{"type":"sync_request","last_event_timestamp":"2026-04-05T00:00:00Z"}`))
	require.NoError(t, err)
	assert.Equal(t, "sync_request", msg.Type)
	assert.Equal(t, "2026-04-05T00:00:00Z", msg.LastEventTimestamp)
}

func TestParseClientMessage_InvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := parseClientMessage([]byte(`{"type"`))
	require.Error(t, err)
}

func TestNewSyncResponseMessage(t *testing.T) {
	t.Parallel()

	game := &entity.Game{
		GameID:           77,
		Status:           entity.GameStatusWaitingForOpenBids,
		PlayerOneAddress: "EQD4FPq-PRDieyQKkizFTRtSDyucUIqrj0v_zXJmqaDp6_0t",
		PlayerOneChoice:  entity.CoinSideClosed,
		BetAmount:        1000000000,
		InitTxHash:       "init-hash",
		CreatedAt:        time.Now(),
	}

	response := newSyncResponseMessage(game)
	assert.Equal(t, "sync_response", response.Type)
	assert.Equal(t, game, response.Game)
	_, err := time.Parse(time.RFC3339Nano, response.Timestamp)
	assert.NoError(t, err)
}

func TestNewErrorMessage(t *testing.T) {
	t.Parallel()

	response := newErrorMessage("unsupported_message_type", "unsupported websocket message type")
	assert.Equal(t, "error", response.Type)
	assert.Equal(t, "unsupported_message_type", response.Code)
	assert.Equal(t, "unsupported websocket message type", response.Message)
	_, err := time.Parse(time.RFC3339Nano, response.Timestamp)
	assert.NoError(t, err)
}
