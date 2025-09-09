package a2s

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"time"
)


const (
	Header        = 0xFFFFFFFF
	SPLIT_FLAG    = 0xFFFFFFFE
	A2S_INFO      = 0x54
	A2S_PLAYER    = 0x55
	A2S_RULES     = 0x56
	A2S_CHALLENGE = 0x41
	S2C_CHALLENGE = 0x41
	S2A_INFO_SRC  = 0x49
	S2A_INFO_GOLD = 0x6D
	S2A_PLAYER    = 0x44
	S2A_RULES     = 0x45
)


type Client struct {
	conn      *net.UDPConn
	challenge int32
	timeout   time.Duration
	connected bool
}

func NewClient(timeout time.Duration) *Client {
	return &Client{
		timeout:   timeout,
		challenge: -1,
	}
}


func (c *Client) Connect(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return err
	}

	c.conn = conn
	c.connected = true
	return conn.SetDeadline(time.Now().Add(c.timeout))
}

func (c *Client) Close() error {
	if c.conn != nil {
		c.connected = false
		return c.conn.Close()
	}
	return nil
}

func (c *Client) IsConnected() bool {
	return c.connected && c.conn != nil
}


// GetInfo gets the server info. It sends an A2S_INFO request to the server and
// parses the response. If the response is from a GoldSource server, it uses
// parseGoldSourceInfo to parse the response. Otherwise, it uses parseSourceInfo.
// If the server is not connected, it returns an ErrNotConnected error.
func (c *Client) GetInfo() (*ServerInfo, error) {
	if !c.IsConnected() {
		return nil, ErrNotConnected
	}

	payload := []byte{0x53, 0x6F, 0x75, 0x72, 0x63, 0x65, 0x20, 0x45, 0x6E, 0x67, 0x69, 0x6E, 0x65, 0x20, 0x51, 0x75, 0x65, 0x72, 0x79, 0x00}
	
	response, err := c.sendRequest(A2S_INFO, payload, S2A_INFO_SRC)
	if err != nil {
		response, err = c.sendRequest(A2S_INFO, payload, S2A_INFO_GOLD)
		if err != nil {
			return nil, err
		}
		return c.parseGoldSourceInfo(response)
	}
	return c.parseSourceInfo(response)
}


func (c *Client) GetPlayers() ([]PlayerInfo, error) {
	if !c.IsConnected() {
		return nil, ErrNotConnected
	}

	challengeReq := []byte{0xFF, 0xFF, 0xFF, 0xFF}
	_, err := c.sendRequest(A2S_PLAYER, challengeReq, S2C_CHALLENGE)
	if err != nil && !errors.Is(err, ErrChallengeRequired) {
		return nil, err
	}
	
	if c.challenge == -1 {
		return nil, errors.New("challenge not received")
	}

	challengeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(challengeBytes, uint32(c.challenge))
	
	response, err := c.sendRequest(A2S_PLAYER, challengeBytes, S2A_PLAYER)
	if err != nil {
		return nil, err
	}

	return c.parsePlayersResponse(response)
}


func (c *Client) GetRules() ([]Rule, error) {
	if !c.IsConnected() {
		return nil, ErrNotConnected
	}

	challengeReq := []byte{0xFF, 0xFF, 0xFF, 0xFF}
	_, err := c.sendRequest(A2S_RULES, challengeReq, S2C_CHALLENGE)
	if err != nil && !errors.Is(err, ErrChallengeRequired) {
		return nil, err
	}
	
	if c.challenge == -1 {
		return nil, errors.New("challenge not received")
	}

	challengeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(challengeBytes, uint32(c.challenge))
	
	response, err := c.sendRequest(A2S_RULES, challengeBytes, S2A_RULES)
	if err != nil {
		return nil, err
	}

	return c.parseRulesResponse(response)
}


// CheckFeatures returns the features supported by the server. It checks if the server
// supports the A2S_PLAYER and A2S_RULES requests and returns a ServerFeatures struct
// with the appropriate fields set to true or false. The Info field is always set to
// true, as the A2S_INFO request is always supported.
func (c *Client) CheckFeatures() ServerFeatures {
	features := ServerFeatures{
		Info: true,
	}

	_, err := c.GetPlayers()
	features.Players = err == nil


	_, err = c.GetRules()
	features.Rules = err == nil

	return features
}


// sendRequest sends a request to the server and waits for a response.
// It retries up to 3 times if the response is a challenge.
// If the response is not what was expected, it returns an error.
func (c *Client) sendRequest(packetType byte, payload []byte, expectResponse byte) ([]byte, error) {
	for retry := 0; retry < 3; retry++ {
		response, err := c.sendRequestRaw(packetType, payload, expectResponse)
		if err != nil {
			if errors.Is(err, ErrChallengeRequired) {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return nil, err
		}
		return response, nil
	}
	return nil, ErrTooManyRetries
}


// sendRequestRaw sends a request to the server and waits for a response.
// It returns an error if the response is not what was expected.
// It does not retry if the response is a challenge.
func (c *Client) sendRequestRaw(packetType byte, payload []byte, expectResponse byte) ([]byte, error) {
	packet := c.buildPacket(packetType, payload)
	
	c.conn.SetDeadline(time.Now().Add(c.timeout))
	
	if _, err := c.conn.Write(packet); err != nil {
		return nil, fmt.Errorf("write error: %w", err)
	}

	buffer := make([]byte, 4096)
	n, err := c.conn.Read(buffer)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, ErrTimeout
		}
		return nil, fmt.Errorf("read error: %w", err)
	}

	return c.processResponse(buffer[:n], expectResponse)
}

// buildPacket builds a packet for sending to the server.
// The packet consists of:
// * a 4-byte header of 0xFFFFFFFF
// * a single byte of the packet type
// * the payload, if any
// * the challenge number, if required
// The challenge number is required for A2S_PLAYER and A2S_RULES packets,
// and is sent at the beginning of the packet. For A2S_INFO packets, the
// challenge number is sent at the end of the packet if it is not -1.
// The packet is built in a single allocation, and the returned []byte
// is suitable for sending directly over the wire.
func (c *Client) buildPacket(packetType byte, payload []byte) []byte {
	challengeAtBeginning := packetType == A2S_PLAYER || packetType == A2S_RULES
	challengeAtEnd := packetType == A2S_INFO && c.challenge != -1

	size := 4 + 1
	if challengeAtBeginning {
		size += 4
	}
	if payload != nil {
		size += len(payload)
	}
	if challengeAtEnd {
		size += 4
	}

	packet := make([]byte, size)
	offset := 0

	binary.LittleEndian.PutUint32(packet[offset:], uint32(Header))
	offset += 4

	packet[offset] = packetType
	offset++

	if challengeAtBeginning {
		binary.LittleEndian.PutUint32(packet[offset:], uint32(c.challenge))
		offset += 4
	}

	if payload != nil {
		copy(packet[offset:], payload)
		offset += len(payload)
	}

	if challengeAtEnd {
		binary.LittleEndian.PutUint32(packet[offset:], uint32(c.challenge))
	}

	return packet
}


// processResponse processes a response from the server, handling split packets and challenges.
// It returns the response data (without the header) and an error. If the response is a challenge,
// the error is ErrChallengeRequired. If the response type is not what was expected, the error is
// a ProtocolError.
func (c *Client) processResponse(data []byte, expect byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, ErrShortResponse
	}

	header := binary.LittleEndian.Uint32(data[:4])
	
	switch header {
	case uint32(Header):
		return c.processSinglePacket(data[4:], expect)
	case uint32(SPLIT_FLAG):
		return c.processSplitPacket(data[4:], expect)
	default:
		return nil, fmt.Errorf("unknown header: 0x%X", header)
	}
}
// processSinglePacket processes a single packet from the server, handling challenges and checking for the expected response type.
// It returns the response data (without the first byte) and an error. If the response is a challenge, the error is ErrChallengeRequired.
// If the response type is not what was expected, the error is a ProtocolError.

func (c *Client) processSinglePacket(data []byte, expect byte) ([]byte, error) {
	if len(data) < 1 {
		return nil, ErrShortResponse
	}

	responseType := data[0]
	
	if responseType == S2C_CHALLENGE {
		if len(data) < 5 {
			return nil, ErrShortResponse
		}
		c.challenge = int32(binary.LittleEndian.Uint32(data[1:5]))
		return nil, ErrChallengeRequired
	}

	if responseType != expect {
		return nil, &ProtocolError{Expected: expect, Actual: responseType}
	}

	return data[1:], nil
}


func (c *Client) processSplitPacket(data []byte, expect byte) ([]byte, error) {
	if len(data) < 9 {
		return nil, ErrShortResponse
	}
	
	payloadStart := 4 + 1 + 1
	if len(data) > 8 {
		payloadStart += 2
	}
	
	if payloadStart >= len(data) {
		return nil, ErrInvalidResponse
	}
	
	return c.processSinglePacket(data[payloadStart:], expect)
}
// parseSourceInfo parses the response to A2S_INFO request from Source (HL2) servers and returns a ServerInfo object.
// It returns an error if the response is too short.

func (c *Client) parseSourceInfo(data []byte) (*ServerInfo, error) {
	if len(data) < 20 {
		return nil, ErrShortResponse
	}

	info := &ServerInfo{}
	offset := 0

	info.Protocol = data[offset]
	offset++

	info.Name = readString(data, &offset)
	info.Map = readString(data, &offset)
	info.Folder = readString(data, &offset)
	info.Game = readString(data, &offset)

	if offset+2 <= len(data) {
		info.AppID = binary.LittleEndian.Uint16(data[offset:])
		offset += 2
	}

	if offset+3 > len(data) {
		return nil, ErrShortResponse
	}
	info.Players = data[offset]
	offset++
	info.MaxPlayers = data[offset]
	offset++
	info.Bots = data[offset]
	offset++

	if offset+4 > len(data) {
		return nil, ErrShortResponse
	}
	info.ServerType = data[offset]
	offset++
	info.Environment = data[offset]
	offset++
	info.Visibility = data[offset]
	offset++
	info.VAC = data[offset]
	offset++

	info.Version = readString(data, &offset)

	if offset < len(data) {
		info.EDF = data[offset]
		offset++

		if info.EDF&0x80 != 0 && offset+2 <= len(data) {
			info.GamePort = binary.LittleEndian.Uint16(data[offset:])
			offset += 2
		}

		if info.EDF&0x10 != 0 && offset+8 <= len(data) {
			info.SteamID = binary.LittleEndian.Uint64(data[offset:])
			offset += 8
		}

		if info.EDF&0x40 != 0 && offset+2 <= len(data) {
			info.SourceTV.Port = binary.LittleEndian.Uint16(data[offset:])
			offset += 2
			info.SourceTV.Name = readString(data, &offset)
		}

		if info.EDF&0x20 != 0 {
			tags := readString(data, &offset)
			if tags != "" {
				
			}
		}

		if info.EDF&0x01 != 0 && offset+8 <= len(data) {
			info.GameID = binary.LittleEndian.Uint64(data[offset:])
		}
	}

	return info, nil
}

// parseGoldSourceInfo parses the response to A2S_INFO request from GoldSource (HL1) servers and returns a ServerInfo object.
// It returns an error if the response is too short.
func (c *Client) parseGoldSourceInfo(data []byte) (*ServerInfo, error) {
	info := &ServerInfo{}
	offset := 0

	_ = readString(data, &offset)
	info.Name = readString(data, &offset)
	info.Map = readString(data, &offset)
	info.Folder = readString(data, &offset)
	info.Game = readString(data, &offset)

	if offset+2 > len(data) {
		return nil, ErrShortResponse
	}
	info.Players = data[offset]
	offset++
	info.MaxPlayers = data[offset]
	offset++

	info.Protocol = data[offset]
	offset++
	info.ServerType = data[offset]
	offset++
	info.Environment = data[offset]
	offset++
	info.Visibility = data[offset]
	offset++

	modFlag := data[offset]
	offset++

	if modFlag == 1 {
		_ = readString(data, &offset)
		_ = readString(data, &offset)
		offset++
		offset += 4
		offset += 4
		offset++
		offset++
	}

	info.VAC = data[offset]
	offset++

	if offset < len(data) {
		info.Bots = data[offset]
	}

	return info, nil
}


// parsePlayersResponse parses the response to A2S_PLAYER request and returns a slice of PlayerInfo.
// The function returns an error if the response is too short.
// The players are returned in the order they were received from the server.
func (c *Client) parsePlayersResponse(data []byte) ([]PlayerInfo, error) {
	if len(data) < 1 {
		return nil, ErrShortResponse
	}

	offset := 0
	numPlayers := int(data[offset])
	offset++

	players := make([]PlayerInfo, 0, numPlayers)
	
	for i := 0; i < numPlayers && offset < len(data); i++ {
		var player PlayerInfo
		
		player.Index = data[offset]
		offset++
		
		player.Name = readString(data, &offset)
		
		if offset+4 > len(data) {
			return nil, ErrShortResponse
		}
		player.Score = int32(binary.LittleEndian.Uint32(data[offset:]))
		offset += 4
		
		if offset+4 > len(data) {
			return nil, ErrShortResponse
		}
		player.Duration = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
		offset += 4

		players = append(players, player)
	}

	return players, nil
}

// parseRulesResponse parses the response to A2S_RULES request and returns a slice of server rules.
// Each rule is a key-value pair, where key and value are strings.
// The function returns an error if the response is too short.
// The rules are returned in the order they were received from the server.
func (c *Client) parseRulesResponse(data []byte) ([]Rule, error) {
	if len(data) < 2 {
		return nil, ErrShortResponse
	}

	offset := 0
	numRules := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2

	rules := make([]Rule, 0, numRules)
	
	for i := 0; i < numRules && offset < len(data); i++ {
		var rule Rule
		rule.Name = readString(data, &offset)
		rule.Value = readString(data, &offset)
		rules = append(rules, rule)
	}

	return rules, nil
}
// readString reads a null-terminated string from the given byte slice, starting from the given offset. It returns the string and updates the offset to point after the null byte. If the offset points to the end of the slice, it returns an empty string.

func readString(data []byte, offset *int) string {
	if *offset >= len(data) {
		return ""
	}
	
	start := *offset
	for *offset < len(data) && data[*offset] != 0 {
		*offset++
	}
	
	str := string(data[start:*offset])
	if *offset < len(data) {
		*offset++
	}
	return str
}