package toncenter

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xssnick/tonutils-go/tvm/cell"
)

func TestRuntimeMessageParser_ParseInMsg_LegacyJSON(t *testing.T) {
	t.Parallel()

	parser := NewRuntimeMessageParser()

	tests := []struct {
		name      string
		inMsg     json.RawMessage
		assertMsg func(t *testing.T, msg *ParsedMessage)
	}{
		{
			name:  "parses GameInitializedNotify from legacy JSON",
			inMsg: json.RawMessage(`{"event_type":"GameInitializedNotify","game_id":123,"player_one":"EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2","bet_amount":"1000000000"}`),
			assertMsg: func(t *testing.T, msg *ParsedMessage) {
				t.Helper()
				assert.Equal(t, "GameInitializedNotify", msg.EventType)
				assert.Equal(t, int64(123), msg.GameID)
				assert.Equal(t, "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2", msg.PlayerOne)
				assert.NotNil(t, msg.BidValue)
				assert.Equal(t, "1000000000", msg.BidValue.String())
			},
		},
		{
			name:  "parses GameStartedNotify from legacy JSON",
			inMsg: json.RawMessage(`{"event_type":"GameStartedNotify","game_id":456,"player_one":"EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2","player_two":"EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X"}`),
			assertMsg: func(t *testing.T, msg *ParsedMessage) {
				t.Helper()
				assert.Equal(t, "GameStartedNotify", msg.EventType)
				assert.Equal(t, int64(456), msg.GameID)
				assert.Equal(t, "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2", msg.PlayerOne)
				assert.Equal(t, "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X", msg.PlayerTwo)
			},
		},
		{
			name:  "parses GameFinishedNotify from legacy JSON",
			inMsg: json.RawMessage(`{"event_type":"GameFinishedNotify","game_id":789,"winner":"EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2","looser":"EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X"}`),
			assertMsg: func(t *testing.T, msg *ParsedMessage) {
				t.Helper()
				assert.Equal(t, "GameFinishedNotify", msg.EventType)
				assert.Equal(t, int64(789), msg.GameID)
				assert.Equal(t, "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2", msg.Winner)
				assert.Equal(t, "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X", msg.Looser)
			},
		},
		{
			name:  "parses DrawNotify from legacy JSON",
			inMsg: json.RawMessage(`{"event_type":"DrawNotify","game_id":999}`),
			assertMsg: func(t *testing.T, msg *ParsedMessage) {
				t.Helper()
				assert.Equal(t, "DrawNotify", msg.EventType)
				assert.Equal(t, int64(999), msg.GameID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			msg, err := parser.ParseInMsg(tt.inMsg)
			require.NoError(t, err)
			tt.assertMsg(t, msg)
		})
	}
}

func TestRuntimeMessageParser_ParseInMsg_LegacyJSONErrors(t *testing.T) {
	t.Parallel()

	parser := NewRuntimeMessageParser()

	t.Run("returns error for empty in_msg", func(t *testing.T) {
		t.Parallel()

		inMsg := json.RawMessage(`null`)
		_, err := parser.ParseInMsg(inMsg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty in_msg")
	})

	t.Run("returns error for missing event_type", func(t *testing.T) {
		t.Parallel()

		inMsg := json.RawMessage(`{"game_id": 123}`)
		_, err := parser.ParseInMsg(inMsg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "event_type")
	})

	t.Run("returns error for missing game_id", func(t *testing.T) {
		t.Parallel()

		inMsg := json.RawMessage(`{"event_type": "GameInitializedNotify"}`)
		_, err := parser.ParseInMsg(inMsg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "game_id")
	})
}

func TestRuntimeMessageParser_ParseInMsg_BOC(t *testing.T) {
	t.Parallel()

	parser := NewRuntimeMessageParser()

	// Test with real TON Center API v3 format
	t.Run("parses message with TON Center API structure", func(t *testing.T) {
		t.Parallel()

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

func TestRuntimeMessageParser_ParseInMsg_UsesMsgDataBodyFallback(t *testing.T) {
	t.Parallel()

	parser := NewRuntimeMessageParser()
	builder := NewTestMessageBuilder()

	messageBase64 := builder.BuildGameStartedNotify(
		456,
		"EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
		"EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X",
		2000000000,
	)

	inMsg := json.RawMessage(`{
		"@type": "raw.message",
		"source": "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
		"destination": "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X",
		"value": "2000000000",
		"msg_data": {"@type": "msg.dataRaw", "body": "` + messageBase64 + `"}
	}`)

	msg, err := parser.ParseInMsg(inMsg)
	require.NoError(t, err)
	assert.Equal(t, EventTypeGameStarted, msg.EventType)
	assert.Equal(t, int64(456), msg.GameID)
	assert.NotNil(t, msg.TotalGainings)
	assert.Equal(t, "2000000000", msg.TotalGainings.String())
}

func TestRuntimeMessageParser_ParseInMsg_RejectsOverflowingUint256GameID(t *testing.T) {
	t.Parallel()

	parser := NewRuntimeMessageParser()

	builder := cell.BeginCell()
	builder.MustStoreUInt(uint64(OpcodeDrawNotifyV2), 32)
	overflowGameID := new(big.Int).Add(big.NewInt(math.MaxInt64), big.NewInt(1))
	builder.MustStoreBigUInt(overflowGameID, 256)

	messageBase64 := base64.StdEncoding.EncodeToString(builder.EndCell().ToBOC())
	inMsg := json.RawMessage(`{
		"@type": "raw.message",
		"message": "` + messageBase64 + `"
	}`)

	_, err := parser.ParseInMsg(inMsg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds int64 max")
}

func TestRuntimeMessageParser_ParseInMsg_RejectsUnsupportedOpcode(t *testing.T) {
	t.Parallel()

	parser := NewRuntimeMessageParser()
	builder := NewTestMessageBuilder()

	inMsg := json.RawMessage(`{
		"@type": "raw.message",
		"message": "` + builder.BuildOpcodeOnlyMessage(0xDEADBEEF) + `"
	}`)

	_, err := parser.ParseInMsg(inMsg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown opcode")
}

func TestRuntimeMessageParser_ParseInMsg_RejectsMalformedShortPayload(t *testing.T) {
	t.Parallel()

	parser := NewRuntimeMessageParser()
	malformedBody := base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03})

	inMsg := json.RawMessage(`{
		"@type": "raw.message",
		"message": "` + malformedBody + `"
	}`)

	_, err := parser.ParseInMsg(inMsg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message too short")
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

	t.Run("builds opcode-only message", func(t *testing.T) {
		msg := builder.BuildOpcodeOnlyMessage(0xDEADBEEF)
		assert.NotEmpty(t, msg)
	})
}

func TestGetEventTypeForOpcode(t *testing.T) {
	t.Parallel()

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

			eventTypeV2, existsV2 := GetEventTypeForOpcodeV2(tt.opcode)
			assert.Equal(t, tt.shouldExist, existsV2)

			if existsV2 {
				assert.Equal(t, tt.expectedType, eventTypeV2)
			}
		})
	}
}

// TestMessageParserV2_RoundTrip tests the complete build -> parse cycle using proper TON Cell format.
// This ensures that messages built with TestMessageBuilder can be correctly parsed by MessageParserV2.
func TestMessageParserV2_RoundTrip(t *testing.T) {
	t.Parallel()

	builder := NewTestMessageBuilder()
	parser := NewRuntimeMessageParser()

	// Test addresses in user-friendly format (EQ prefix for mainnet)
	testAddr1 := "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2"
	testAddr2 := "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X"

	t.Run("GameInitializedNotify round-trip", func(t *testing.T) {
		// Build message
		gameID := int64(12345)
		bidValue := int64(1000000000) // 1 TON
		messageBase64 := builder.BuildGameInitializedNotify(gameID, testAddr1, bidValue)

		// Create in_msg JSON like from TON Center API
		inMsgJSON := json.RawMessage(`{
			"@type": "raw.message",
			"source": "` + testAddr1 + `",
			"destination": "` + testAddr2 + `",
			"value": "1000000000",
			"message": "` + messageBase64 + `",
			"msg_data": {"@type": "msg.dataRaw", "body": "` + messageBase64 + `"}
		}`)

		// Parse message
		msg, err := parser.ParseInMsg(inMsgJSON)
		require.NoError(t, err, "ParseInMsg should not return error")
		assert.Equal(t, EventTypeGameInitialized, msg.EventType)
		assert.Equal(t, gameID, msg.GameID)
		// Address format from parser uses EQ prefix
		assert.Contains(t, msg.PlayerOne, "EQ")
		assert.NotNil(t, msg.BidValue)
		assert.Equal(t, "1000000000", msg.BidValue.String())
	})

	t.Run("GameStartedNotify round-trip", func(t *testing.T) {
		gameID := int64(67890)
		totalGainings := int64(2000000000) // 2 TON
		messageBase64 := builder.BuildGameStartedNotify(gameID, testAddr1, testAddr2, totalGainings)

		inMsgJSON := json.RawMessage(`{
			"@type": "raw.message",
			"source": "` + testAddr1 + `",
			"destination": "` + testAddr2 + `",
			"value": "2000000000",
			"message": "` + messageBase64 + `"
		}`)

		msg, err := parser.ParseInMsg(inMsgJSON)
		require.NoError(t, err)
		assert.Equal(t, EventTypeGameStarted, msg.EventType)
		assert.Equal(t, gameID, msg.GameID)
		assert.Contains(t, msg.PlayerOne, "EQ")
		assert.Contains(t, msg.PlayerTwo, "EQ")
		assert.NotNil(t, msg.TotalGainings)
		assert.Equal(t, "2000000000", msg.TotalGainings.String())
	})

	t.Run("GameFinishedNotify round-trip", func(t *testing.T) {
		gameID := int64(99999)
		totalGainings := int64(1900000000)
		messageBase64 := builder.BuildGameFinishedNotify(gameID, testAddr1, testAddr2, totalGainings)

		inMsgJSON := json.RawMessage(`{
			"@type": "raw.message",
			"source": "` + testAddr1 + `",
			"destination": "` + testAddr2 + `",
			"value": "1900000000",
			"message": "` + messageBase64 + `"
		}`)

		msg, err := parser.ParseInMsg(inMsgJSON)
		require.NoError(t, err)
		assert.Equal(t, EventTypeGameFinished, msg.EventType)
		assert.Equal(t, gameID, msg.GameID)
		assert.Contains(t, msg.Winner, "EQ")
		assert.Contains(t, msg.Looser, "EQ")
		assert.NotNil(t, msg.TotalGainings)
	})

	t.Run("GameCancelledNotify round-trip", func(t *testing.T) {
		gameID := int64(55555)
		messageBase64 := builder.BuildGameCancelledNotify(gameID, testAddr1)

		inMsgJSON := json.RawMessage(`{
			"@type": "raw.message",
			"source": "` + testAddr1 + `",
			"destination": "` + testAddr2 + `",
			"value": "100000",
			"message": "` + messageBase64 + `"
		}`)

		msg, err := parser.ParseInMsg(inMsgJSON)
		require.NoError(t, err)
		assert.Equal(t, EventTypeGameCancelled, msg.EventType)
		assert.Equal(t, gameID, msg.GameID)
		assert.Contains(t, msg.PlayerOne, "EQ")
	})

	t.Run("DrawNotify round-trip", func(t *testing.T) {
		gameID := int64(77777)
		messageBase64 := builder.BuildDrawNotify(gameID)

		inMsgJSON := json.RawMessage(`{
			"@type": "raw.message",
			"source": "` + testAddr1 + `",
			"destination": "` + testAddr2 + `",
			"value": "0",
			"message": "` + messageBase64 + `"
		}`)

		msg, err := parser.ParseInMsg(inMsgJSON)
		require.NoError(t, err)
		assert.Equal(t, EventTypeDraw, msg.EventType)
		assert.Equal(t, gameID, msg.GameID)
	})

	t.Run("SecretOpenedNotify round-trip", func(t *testing.T) {
		gameID := int64(33333)
		coinSide := uint8(1) // Heads
		messageBase64 := builder.BuildSecretOpenedNotify(gameID, testAddr1, coinSide)

		inMsgJSON := json.RawMessage(`{
			"@type": "raw.message",
			"source": "` + testAddr1 + `",
			"destination": "` + testAddr2 + `",
			"value": "0",
			"message": "` + messageBase64 + `"
		}`)

		msg, err := parser.ParseInMsg(inMsgJSON)
		require.NoError(t, err)
		assert.Equal(t, EventTypeSecretOpened, msg.EventType)
		assert.Equal(t, gameID, msg.GameID)
		assert.Contains(t, msg.Player, "EQ")
		assert.Equal(t, coinSide, msg.CoinSide)
	})

	t.Run("InsufficientBalanceNotify round-trip", func(t *testing.T) {
		gameID := int64(44444)
		required := int64(2000000000)
		actual := int64(1500000000)
		messageBase64 := builder.BuildInsufficientBalanceNotify(gameID, required, actual)

		inMsgJSON := json.RawMessage(`{
			"@type": "raw.message",
			"source": "` + testAddr1 + `",
			"destination": "` + testAddr2 + `",
			"value": "0",
			"message": "` + messageBase64 + `"
		}`)

		msg, err := parser.ParseInMsg(inMsgJSON)
		require.NoError(t, err)
		assert.Equal(t, EventTypeInsufficientBalance, msg.EventType)
		assert.Equal(t, gameID, msg.GameID)
		assert.NotNil(t, msg.Required)
		assert.NotNil(t, msg.Actual)
		assert.Equal(t, "2000000000", msg.Required.String())
		assert.Equal(t, "1500000000", msg.Actual.String())
	})
}
