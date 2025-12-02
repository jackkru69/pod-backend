package toncenter

import (
	"encoding/base64"
	"encoding/binary"
	"math/big"
)

// TestMessageBuilder builds BOC-encoded messages for testing.
// It creates messages in the same format as real TON blockchain transactions.
type TestMessageBuilder struct{}

// NewTestMessageBuilder creates a new message builder for tests.
func NewTestMessageBuilder() *TestMessageBuilder {
	return &TestMessageBuilder{}
}

// BuildGameInitializedNotify creates a BOC-encoded GameInitializedNotify message.
func (b *TestMessageBuilder) BuildGameInitializedNotify(gameID int64, playerOne string, bidValue int64) string {
	var data []byte
	
	// Opcode (4 bytes, big-endian)
	opcode := make([]byte, 4)
	binary.BigEndian.PutUint32(opcode, OpcodeGameInitializedNotify)
	data = append(data, opcode...)
	
	// gameId: uint256 (32 bytes, big-endian)
	gameIDBytes := make([]byte, 32)
	big.NewInt(gameID).FillBytes(gameIDBytes)
	data = append(data, gameIDBytes...)
	
	// playerOne: Address (workchain + hash = 33 bytes)
	addrBytes := b.encodeAddress(playerOne)
	data = append(data, addrBytes...)
	
	// bidValue: coins (VarUInt16 format)
	coinsBytes := b.encodeCoins(big.NewInt(bidValue))
	data = append(data, coinsBytes...)
	
	return base64.StdEncoding.EncodeToString(data)
}

// BuildGameStartedNotify creates a BOC-encoded GameStartedNotify message.
func (b *TestMessageBuilder) BuildGameStartedNotify(gameID int64, playerOne, playerTwo string, totalGainings int64) string {
	var data []byte
	
	// Opcode
	opcode := make([]byte, 4)
	binary.BigEndian.PutUint32(opcode, OpcodeGameStartedNotify)
	data = append(data, opcode...)
	
	// gameId: uint256
	gameIDBytes := make([]byte, 32)
	big.NewInt(gameID).FillBytes(gameIDBytes)
	data = append(data, gameIDBytes...)
	
	// playerOne: Address
	data = append(data, b.encodeAddress(playerOne)...)
	
	// playerTwo: Address
	data = append(data, b.encodeAddress(playerTwo)...)
	
	// totalGainings: coins
	data = append(data, b.encodeCoins(big.NewInt(totalGainings))...)
	
	return base64.StdEncoding.EncodeToString(data)
}

// BuildGameFinishedNotify creates a BOC-encoded GameFinishedNotify message.
func (b *TestMessageBuilder) BuildGameFinishedNotify(gameID int64, winner, looser string, totalGainings int64) string {
	var data []byte
	
	// Opcode
	opcode := make([]byte, 4)
	binary.BigEndian.PutUint32(opcode, OpcodeGameFinishedNotify)
	data = append(data, opcode...)
	
	// gameId: uint256
	gameIDBytes := make([]byte, 32)
	big.NewInt(gameID).FillBytes(gameIDBytes)
	data = append(data, gameIDBytes...)
	
	// winner: Address
	data = append(data, b.encodeAddress(winner)...)
	
	// looser: Address
	data = append(data, b.encodeAddress(looser)...)
	
	// totalGainings: coins
	data = append(data, b.encodeCoins(big.NewInt(totalGainings))...)
	
	return base64.StdEncoding.EncodeToString(data)
}

// BuildGameCancelledNotify creates a BOC-encoded GameCancelledNotify message.
func (b *TestMessageBuilder) BuildGameCancelledNotify(gameID int64, playerOne string) string {
	var data []byte
	
	// Opcode
	opcode := make([]byte, 4)
	binary.BigEndian.PutUint32(opcode, OpcodeGameCancelledNotify)
	data = append(data, opcode...)
	
	// gameId: uint256
	gameIDBytes := make([]byte, 32)
	big.NewInt(gameID).FillBytes(gameIDBytes)
	data = append(data, gameIDBytes...)
	
	// playerOne: Address
	data = append(data, b.encodeAddress(playerOne)...)
	
	return base64.StdEncoding.EncodeToString(data)
}

// BuildDrawNotify creates a BOC-encoded DrawNotify message.
func (b *TestMessageBuilder) BuildDrawNotify(gameID int64) string {
	var data []byte
	
	// Opcode
	opcode := make([]byte, 4)
	binary.BigEndian.PutUint32(opcode, OpcodeDrawNotify)
	data = append(data, opcode...)
	
	// gameId: uint256
	gameIDBytes := make([]byte, 32)
	big.NewInt(gameID).FillBytes(gameIDBytes)
	data = append(data, gameIDBytes...)
	
	return base64.StdEncoding.EncodeToString(data)
}

// BuildSecretOpenedNotify creates a BOC-encoded SecretOpenedNotify message.
func (b *TestMessageBuilder) BuildSecretOpenedNotify(gameID int64, player string, coinSide uint8) string {
	var data []byte
	
	// Opcode
	opcode := make([]byte, 4)
	binary.BigEndian.PutUint32(opcode, OpcodeSecretOpenedNotify)
	data = append(data, opcode...)
	
	// gameId: uint256
	gameIDBytes := make([]byte, 32)
	big.NewInt(gameID).FillBytes(gameIDBytes)
	data = append(data, gameIDBytes...)
	
	// player: Address
	data = append(data, b.encodeAddress(player)...)
	
	// coinSide: uint8
	data = append(data, coinSide)
	
	return base64.StdEncoding.EncodeToString(data)
}

// BuildInsufficientBalanceNotify creates a BOC-encoded InsufficientBalanceNotify message.
func (b *TestMessageBuilder) BuildInsufficientBalanceNotify(gameID int64, required, actual int64) string {
	var data []byte
	
	// Opcode
	opcode := make([]byte, 4)
	binary.BigEndian.PutUint32(opcode, OpcodeInsufficientBalanceNotify)
	data = append(data, opcode...)
	
	// gameId: uint256
	gameIDBytes := make([]byte, 32)
	big.NewInt(gameID).FillBytes(gameIDBytes)
	data = append(data, gameIDBytes...)
	
	// required: coins
	data = append(data, b.encodeCoins(big.NewInt(required))...)
	
	// actual: coins
	data = append(data, b.encodeCoins(big.NewInt(actual))...)
	
	return base64.StdEncoding.EncodeToString(data)
}

// encodeAddress encodes a TON address string to raw bytes.
// For testing, we use a simplified encoding: workchain (1 byte) + 32 bytes of hash.
// Real TON addresses are base64url encoded and include checksum.
func (b *TestMessageBuilder) encodeAddress(addr string) []byte {
	result := make([]byte, 33)
	
	// Simplified: just use 0 workchain and hash from address string
	result[0] = 0 // workchain 0 for basechain
	
	// For testing, fill hash with address string bytes (simplified)
	// In production, you'd decode the real address
	addrBytes := []byte(addr)
	copy(result[1:], addrBytes)
	
	return result
}

// encodeCoins encodes a big.Int value to VarUInt16 format.
// VarUInt16: first byte is length (0-15), followed by value bytes.
func (b *TestMessageBuilder) encodeCoins(value *big.Int) []byte {
	if value == nil || value.Sign() == 0 {
		return []byte{0}
	}
	
	valueBytes := value.Bytes()
	length := len(valueBytes)
	if length > 15 {
		length = 15
		valueBytes = valueBytes[len(valueBytes)-15:]
	}
	
	result := make([]byte, 1+length)
	result[0] = byte(length)
	copy(result[1:], valueBytes)
	
	return result
}

// CreateInMsgJSON creates a JSON structure matching TON Center API format.
// This is used to build test transactions with proper in_msg structure.
func (b *TestMessageBuilder) CreateInMsgJSON(source, destination, value, messageBase64 string) string {
	return `{
		"@type": "raw.message",
		"source": "` + source + `",
		"destination": "` + destination + `",
		"value": "` + value + `",
		"message": "` + messageBase64 + `"
	}`
}
