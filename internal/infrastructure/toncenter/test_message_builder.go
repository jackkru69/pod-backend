package toncenter

import (
	"encoding/base64"
	"math/big"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tvm/cell"
)

// TestMessageBuilder builds BOC-encoded messages for testing.
// It creates messages in the same format as real TON blockchain transactions.
// Uses tonutils-go Cell builder to create proper TON Cell format.
type TestMessageBuilder struct{}

// NewTestMessageBuilder creates a new message builder for tests.
func NewTestMessageBuilder() *TestMessageBuilder {
	return &TestMessageBuilder{}
}

// parseOrDefaultAddress parses a TON address or returns a default zero address.
func (b *TestMessageBuilder) parseOrDefaultAddress(addrStr string) *address.Address {
	addr, err := address.ParseAddr(addrStr)
	if err != nil {
		// For testing with invalid addresses, use a default zero address
		// We create it manually instead of using MustParseAddr to avoid panic
		defaultAddr := address.NewAddress(0, 0, make([]byte, 32))
		return defaultAddr
	}
	return addr
}

// BuildGameInitializedNotify creates a BOC-encoded GameInitializedNotify message.
// Message format matches contract ABI:
// - opcode: uint32 (32 bits)
// - gameId: uint256 (256 bits)
// - playerOne: address
// - bidValue: coins (VarUInt16)
func (b *TestMessageBuilder) BuildGameInitializedNotify(gameID int64, playerOne string, bidValue int64) string {
	builder := cell.BeginCell()

	// Opcode (32 bits)
	builder.MustStoreUInt(uint64(OpcodeGameInitializedNotifyV2), 32)

	// gameId: uint256 (256 bits)
	builder.MustStoreBigUInt(big.NewInt(gameID), 256)

	// playerOne: Address
	builder.MustStoreAddr(b.parseOrDefaultAddress(playerOne))

	// bidValue: coins (VarUInt16)
	builder.MustStoreBigCoins(big.NewInt(bidValue))

	c := builder.EndCell()
	boc := c.ToBOC()
	return base64.StdEncoding.EncodeToString(boc)
}

// BuildGameStartedNotify creates a BOC-encoded GameStartedNotify message.
// Message format:
// - opcode: uint32
// - gameId: uint256
// - playerOne: address
// - playerTwo: address
// - totalGainings: coins
func (b *TestMessageBuilder) BuildGameStartedNotify(gameID int64, playerOne, playerTwo string, totalGainings int64) string {
	builder := cell.BeginCell()

	// Opcode (32 bits)
	builder.MustStoreUInt(uint64(OpcodeGameStartedNotifyV2), 32)

	// gameId: uint256 (256 bits)
	builder.MustStoreBigUInt(big.NewInt(gameID), 256)

	// playerOne: Address
	builder.MustStoreAddr(b.parseOrDefaultAddress(playerOne))

	// playerTwo: Address
	builder.MustStoreAddr(b.parseOrDefaultAddress(playerTwo))

	// totalGainings: coins (VarUInt16)
	builder.MustStoreBigCoins(big.NewInt(totalGainings))

	c := builder.EndCell()
	boc := c.ToBOC()
	return base64.StdEncoding.EncodeToString(boc)
}

// BuildGameFinishedNotify creates a BOC-encoded GameFinishedNotify message.
// Message format:
// - opcode: uint32
// - gameId: uint256
// - winner: address
// - looser: address
// - totalGainings: coins
func (b *TestMessageBuilder) BuildGameFinishedNotify(gameID int64, winner, looser string, totalGainings int64) string {
	builder := cell.BeginCell()

	// Opcode (32 bits)
	builder.MustStoreUInt(uint64(OpcodeGameFinishedNotifyV2), 32)

	// gameId: uint256 (256 bits)
	builder.MustStoreBigUInt(big.NewInt(gameID), 256)

	// winner: Address
	builder.MustStoreAddr(b.parseOrDefaultAddress(winner))

	// looser: Address
	builder.MustStoreAddr(b.parseOrDefaultAddress(looser))

	// totalGainings: coins (VarUInt16)
	builder.MustStoreBigCoins(big.NewInt(totalGainings))

	c := builder.EndCell()
	boc := c.ToBOC()
	return base64.StdEncoding.EncodeToString(boc)
}

// BuildGameCancelledNotify creates a BOC-encoded GameCancelledNotify message.
// Message format:
// - opcode: uint32
// - gameId: uint256
// - playerOne: address
func (b *TestMessageBuilder) BuildGameCancelledNotify(gameID int64, playerOne string) string {
	builder := cell.BeginCell()

	// Opcode (32 bits)
	builder.MustStoreUInt(uint64(OpcodeGameCancelledNotifyV2), 32)

	// gameId: uint256 (256 bits)
	builder.MustStoreBigUInt(big.NewInt(gameID), 256)

	// playerOne: Address
	builder.MustStoreAddr(b.parseOrDefaultAddress(playerOne))

	c := builder.EndCell()
	boc := c.ToBOC()
	return base64.StdEncoding.EncodeToString(boc)
}

// BuildDrawNotify creates a BOC-encoded DrawNotify message.
// Message format:
// - opcode: uint32
// - gameId: uint256
func (b *TestMessageBuilder) BuildDrawNotify(gameID int64) string {
	builder := cell.BeginCell()

	// Opcode (32 bits)
	builder.MustStoreUInt(uint64(OpcodeDrawNotifyV2), 32)

	// gameId: uint256 (256 bits)
	builder.MustStoreBigUInt(big.NewInt(gameID), 256)

	c := builder.EndCell()
	boc := c.ToBOC()
	return base64.StdEncoding.EncodeToString(boc)
}

// BuildSecretOpenedNotify creates a BOC-encoded SecretOpenedNotify message.
// Message format:
// - opcode: uint32
// - gameId: uint256
// - player: address
// - coinSide: uint8
func (b *TestMessageBuilder) BuildSecretOpenedNotify(gameID int64, player string, coinSide uint8) string {
	builder := cell.BeginCell()

	// Opcode (32 bits)
	builder.MustStoreUInt(uint64(OpcodeSecretOpenedNotifyV2), 32)

	// gameId: uint256 (256 bits)
	builder.MustStoreBigUInt(big.NewInt(gameID), 256)

	// player: Address
	builder.MustStoreAddr(b.parseOrDefaultAddress(player))

	// coinSide: uint8 (8 bits)
	builder.MustStoreUInt(uint64(coinSide), 8)

	c := builder.EndCell()
	boc := c.ToBOC()
	return base64.StdEncoding.EncodeToString(boc)
}

// BuildInsufficientBalanceNotify creates a BOC-encoded InsufficientBalanceNotify message.
// Message format:
// - opcode: uint32
// - gameId: uint256
// - required: coins
// - actual: coins
func (b *TestMessageBuilder) BuildInsufficientBalanceNotify(gameID int64, required, actual int64) string {
	builder := cell.BeginCell()

	// Opcode (32 bits)
	builder.MustStoreUInt(uint64(OpcodeInsufficientBalanceNotifyV2), 32)

	// gameId: uint256 (256 bits)
	builder.MustStoreBigUInt(big.NewInt(gameID), 256)

	// required: coins (VarUInt16)
	builder.MustStoreBigCoins(big.NewInt(required))

	// actual: coins (VarUInt16)
	builder.MustStoreBigCoins(big.NewInt(actual))

	c := builder.EndCell()
	boc := c.ToBOC()
	return base64.StdEncoding.EncodeToString(boc)
}

// BuildGameInitializedEvent creates a BOC-encoded factory-emitted GameInitializedEvent message.
func (b *TestMessageBuilder) BuildGameInitializedEvent(gameID int64, playerOne string, bidValue int64, timestamp uint32) string {
	builder := cell.BeginCell()
	builder.MustStoreUInt(uint64(OpcodeGameInitializedEvent), 32)
	builder.MustStoreBigUInt(big.NewInt(gameID), 256)
	builder.MustStoreAddr(b.parseOrDefaultAddress(playerOne))
	builder.MustStoreBigCoins(big.NewInt(bidValue))
	builder.MustStoreUInt(uint64(timestamp), 32)

	c := builder.EndCell()
	boc := c.ToBOC()
	return base64.StdEncoding.EncodeToString(boc)
}

// BuildGameStartedEvent creates a BOC-encoded factory-emitted GameStartedEvent message.
func (b *TestMessageBuilder) BuildGameStartedEvent(gameID int64, playerOne, playerTwo string, totalGainings int64, timestamp uint32) string {
	builder := cell.BeginCell()
	builder.MustStoreUInt(uint64(OpcodeGameStartedEvent), 32)
	builder.MustStoreBigUInt(big.NewInt(gameID), 256)
	builder.MustStoreAddr(b.parseOrDefaultAddress(playerOne))
	builder.MustStoreAddr(b.parseOrDefaultAddress(playerTwo))
	builder.MustStoreBigCoins(big.NewInt(totalGainings))
	builder.MustStoreUInt(uint64(timestamp), 32)

	c := builder.EndCell()
	boc := c.ToBOC()
	return base64.StdEncoding.EncodeToString(boc)
}

// BuildGameFinishedEvent creates a BOC-encoded factory-emitted GameFinishedEvent message.
func (b *TestMessageBuilder) BuildGameFinishedEvent(gameID int64, winner, looser string, totalGainings int64, timestamp uint32) string {
	builder := cell.BeginCell()
	builder.MustStoreUInt(uint64(OpcodeGameFinishedEvent), 32)
	builder.MustStoreBigUInt(big.NewInt(gameID), 256)
	builder.MustStoreAddr(b.parseOrDefaultAddress(winner))
	builder.MustStoreAddr(b.parseOrDefaultAddress(looser))
	builder.MustStoreBigCoins(big.NewInt(totalGainings))
	builder.MustStoreUInt(uint64(timestamp), 32)

	c := builder.EndCell()
	boc := c.ToBOC()
	return base64.StdEncoding.EncodeToString(boc)
}

// BuildGameCancelledEvent creates a BOC-encoded factory-emitted GameCancelledEvent message.
func (b *TestMessageBuilder) BuildGameCancelledEvent(gameID int64, playerOne string, timestamp uint32) string {
	builder := cell.BeginCell()
	builder.MustStoreUInt(uint64(OpcodeGameCancelledEvent), 32)
	builder.MustStoreBigUInt(big.NewInt(gameID), 256)
	builder.MustStoreAddr(b.parseOrDefaultAddress(playerOne))
	builder.MustStoreUInt(uint64(timestamp), 32)

	c := builder.EndCell()
	boc := c.ToBOC()
	return base64.StdEncoding.EncodeToString(boc)
}

// BuildDrawEvent creates a BOC-encoded factory-emitted DrawEvent message.
func (b *TestMessageBuilder) BuildDrawEvent(gameID int64, timestamp uint32) string {
	builder := cell.BeginCell()
	builder.MustStoreUInt(uint64(OpcodeDrawEvent), 32)
	builder.MustStoreBigUInt(big.NewInt(gameID), 256)
	builder.MustStoreUInt(uint64(timestamp), 32)

	c := builder.EndCell()
	boc := c.ToBOC()
	return base64.StdEncoding.EncodeToString(boc)
}

// BuildOpcodeOnlyMessage creates a BOC-encoded message containing only an opcode.
// Useful for testing unsupported or truncated payload handling.
func (b *TestMessageBuilder) BuildOpcodeOnlyMessage(opcode uint32) string {
	builder := cell.BeginCell()
	builder.MustStoreUInt(uint64(opcode), 32)

	c := builder.EndCell()
	boc := c.ToBOC()
	return base64.StdEncoding.EncodeToString(boc)
}

// CreateInMsgJSON creates a JSON structure matching TON Center API format.
// This is used to build test transactions with proper in_msg structure.
func (b *TestMessageBuilder) CreateInMsgJSON(source, destination, value, messageBase64 string) string {
	return `{
		"@type": "raw.message",
		"source": "` + source + `",
		"destination": "` + destination + `",
		"value": "` + value + `",
		"message": "` + messageBase64 + `",
		"msg_data": {
			"@type": "msg.dataRaw",
			"body": "` + messageBase64 + `"
		}
	}`
}

// BuildTestTransaction creates a complete test transaction with in_msg.
// This helper method simplifies creating test transactions for integration tests.
func (b *TestMessageBuilder) BuildTestTransaction(txHash, lt, source, destination, value, messageBase64 string) string {
	return `{
		"@type": "raw.transaction",
		"transaction_id": {
			"@type": "internal.transactionId",
			"lt": "` + lt + `",
			"hash": "` + txHash + `"
		},
		"utime": 1234567890,
		"data": "",
		"in_msg": ` + b.CreateInMsgJSON(source, destination, value, messageBase64) + `,
		"out_msgs": [],
		"fee": "1000000",
		"storage_fee": "100000",
		"other_fee": "50000"
	}`
}
