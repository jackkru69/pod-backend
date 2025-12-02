package toncenter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageParser_ParseInMsg_LegacyJSON(t *testing.T) {
	parser := NewMessageParser()

	t.Run("parses GameInitializedNotify from legacy JSON", func(t *testing.T) {
		inMsg := json.RawMessage(`{
			"event_type": "GameInitializedNotify",
			"game_id": 123,
			"player_one": "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			"bet_amount": "1000000000"
		}`)
		msg, err := parser.ParseInMsg(inMsg)
		require.NoError(t, err)
		assert.Equal(t, "GameInitializedNotify", msg.EventType)
		assert.Equal(t, int64(123), msg.GameID)
		assert.Equal(t, "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2", msg.PlayerOne)
		assert.NotNil(t, msg.BidValue)
		assert.Equal(t, "1000000000", msg.BidValue.String())
	})

	t.Run("parses GameStartedNotify from legacy JSON", func(t *testing.T) {
		inMsg := json.RawMessage(`{
			"event_type": "GameStartedNotify",
			"game_id": 456,
			"player_one": "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			"player_two": "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X"
		}`)
		msg, err := parser.ParseInMsg(inMsg)
		require.NoError(t, err)
		assert.Equal(t, "GameStartedNotify", msg.EventType)
		assert.Equal(t, int64(456), msg.GameID)
		assert.Equal(t, "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2", msg.PlayerOne)
		assert.Equal(t, "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X", msg.PlayerTwo)
	})

	t.Run("parses GameFinishedNotify from legacy JSON", func(t *testing.T) {
		inMsg := json.RawMessage(`{
			"event_type": "GameFinishedNotify",
			"game_id": 789,
			"winner": "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			"looser": "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X"
		}`)
		msg, err := parser.ParseInMsg(inMsg)
		require.NoError(t, err)
		assert.Equal(t, "GameFinishedNotify", msg.EventType)
		assert.Equal(t, int64(789), msg.GameID)
		assert.Equal(t, "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2", msg.Winner)
		assert.Equal(t, "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X", msg.Looser)
	})

	t.Run("parses DrawNotify from legacy JSON", func(t *testing.T) {
		inMsg := json.RawMessage(`{
			"event_type": "DrawNotify",
			"game_id": 999
		}`)
		msg, err := parser.ParseInMsg(inMsg)
		require.NoError(t, err)
		assert.Equal(t, "DrawNotify", msg.EventType)
		assert.Equal(t, int64(999), msg.GameID)
	})

	t.Run("returns error for empty in_msg", func(t *testing.T) {
		inMsg := json.RawMessage(`null`)
		_, err := parser.ParseInMsg(inMsg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty in_msg")
	})

	t.Run("returns error for missing event_type", func(t *testing.T) {
		inMsg := json.RawMessage(`{"game_id": 123}`)
		_, err := parser.ParseInMsg(inMsg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "event_type")
	})

	t.Run("returns error for missing game_id", func(t *testing.T) {
		inMsg := json.RawMessage(`{"event_type": "GameInitializedNotify"}`)
		_, err := parser.ParseInMsg(inMsg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "game_id")
	})
}

func TestMessageParser_ParseInMsg_BOC(t *testing.T) {
	parser := NewMessageParser()

	// Test with real TON Center API v3 format
	t.Run("parses message with TON Center API structure", func(t *testing.T) {
		// This is a simplified test - in production you'd have real base64 BOC data
		// For now, we test that the parser correctly detects and falls back to legacy JSON
		// when the message field doesn't contain valid BOC
		inMsg := json.RawMessage(`{
			"@type": "raw.message",
			"source": "EQD6F-tB_9ey8lZaAn9NsL3uy-0i4sNh50gpBaQGHujGITnD",
			"destination": "EQBpP1onIv-k4bZW8cCXd4bxmLLBkMCVcq8l4vKPNYU5U8aF",
			"value": "8309000",
			"event_type": "GameInitializedNotify",
			"game_id": 201,
			"player_one": "EQD6F-tB_9ey8lZaAn9NsL3uy-0i4sNh50gpBaQGHujGITnD"
		}`)
		msg, err := parser.ParseInMsg(inMsg)
		require.NoError(t, err)
		assert.Equal(t, "GameInitializedNotify", msg.EventType)
		assert.Equal(t, int64(201), msg.GameID)
	})
}

func TestTestMessageBuilder_BuildMethods(t *testing.T) {
	builder := NewTestMessageBuilder()

	t.Run("builds GameInitializedNotify message", func(t *testing.T) {
		msg := builder.BuildGameInitializedNotify(123, "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2", 1000000000)
		assert.NotEmpty(t, msg)
		// Verify it's valid base64
		assert.NotContains(t, msg, " ")
	})

	t.Run("builds GameStartedNotify message", func(t *testing.T) {
		msg := builder.BuildGameStartedNotify(
			456,
			"EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			"EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X",
			2000000000,
		)
		assert.NotEmpty(t, msg)
	})

	t.Run("builds GameFinishedNotify message", func(t *testing.T) {
		msg := builder.BuildGameFinishedNotify(
			789,
			"EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			"EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X",
			1900000000,
		)
		assert.NotEmpty(t, msg)
	})

	t.Run("builds DrawNotify message", func(t *testing.T) {
		msg := builder.BuildDrawNotify(999)
		assert.NotEmpty(t, msg)
	})

	t.Run("builds GameCancelledNotify message", func(t *testing.T) {
		msg := builder.BuildGameCancelledNotify(111, "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2")
		assert.NotEmpty(t, msg)
	})
}

func TestGetEventTypeForOpcode(t *testing.T) {
	tests := []struct {
		opcode       uint32
		expectedType string
		shouldExist  bool
	}{
		{OpcodeGameInitializedNotify, EventTypeGameInitialized, true},
		{OpcodeGameStartedNotify, EventTypeGameStarted, true},
		{OpcodeGameFinishedNotify, EventTypeGameFinished, true},
		{OpcodeGameCancelledNotify, EventTypeGameCancelled, true},
		{OpcodeSecretOpenedNotify, EventTypeSecretOpened, true},
		{OpcodeDrawNotify, EventTypeDraw, true},
		{OpcodeInsufficientBalanceNotify, EventTypeInsufficientBalance, true},
		{0x00000000, "", false}, // Unknown opcode
		{0xFFFFFFFF, "", false}, // Unknown opcode
	}

	for _, tt := range tests {
		t.Run(tt.expectedType, func(t *testing.T) {
			eventType, exists := GetEventTypeForOpcode(tt.opcode)
			assert.Equal(t, tt.shouldExist, exists)
			if exists {
				assert.Equal(t, tt.expectedType, eventType)
			}
		})
	}
}
