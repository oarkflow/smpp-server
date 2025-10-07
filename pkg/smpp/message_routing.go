package smpp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DefaultMessageRouter provides basic message routing
type DefaultMessageRouter struct {
	routes map[string]string // prefix -> gateway mapping
	mutex  sync.RWMutex
}

// NewDefaultMessageRouter creates a new default router
func NewDefaultMessageRouter() *DefaultMessageRouter {
	return &DefaultMessageRouter{
		routes: make(map[string]string),
	}
}

// AddRoute adds a routing rule
func (r *DefaultMessageRouter) AddRoute(prefix, gateway string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.routes[prefix] = gateway
}

// RouteMessage determines the best route for a message
func (r *DefaultMessageRouter) RouteMessage(ctx context.Context, message *Message) (*RouteInfo, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	destination := message.DestAddr.Addr
	gateway := "default"

	// Find the longest matching prefix
	longestMatch := ""
	for prefix, gw := range r.routes {
		if strings.HasPrefix(destination, prefix) && len(prefix) > len(longestMatch) {
			longestMatch = prefix
			gateway = gw
		}
	}

	// Determine priority based on message class
	priority := 1
	if message.PriorityFlag > 0 {
		priority = int(message.PriorityFlag)
	}

	return &RouteInfo{
		Gateway:     gateway,
		Priority:    priority,
		RetryCount:  3,
		Destination: destination,
		Route:       longestMatch,
	}, nil
}

// InMemoryMessageQueue provides in-memory message queuing
type InMemoryMessageQueue struct {
	messages chan *QueuedMessage
	status   map[string]MessageStatus
	mutex    sync.RWMutex
}

// NewInMemoryMessageQueue creates a new in-memory queue
func NewInMemoryMessageQueue(capacity int) *InMemoryMessageQueue {
	return &InMemoryMessageQueue{
		messages: make(chan *QueuedMessage, capacity),
		status:   make(map[string]MessageStatus),
	}
}

// EnqueueMessage adds a message to the queue
func (q *InMemoryMessageQueue) EnqueueMessage(ctx context.Context, message *Message, route *RouteInfo) error {
	queuedMsg := &QueuedMessage{
		Message:   message,
		Route:     route,
		Attempts:  0,
		NextRetry: time.Now(),
		Created:   time.Now(),
	}

	select {
	case q.messages <- queuedMsg:
		q.mutex.Lock()
		q.status[message.ID] = MessageStatusEnroute
		q.mutex.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("queue is full")
	}
}

// DequeueMessage retrieves a message from the queue
func (q *InMemoryMessageQueue) DequeueMessage(ctx context.Context) (*QueuedMessage, error) {
	select {
	case msg := <-q.messages:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// UpdateMessageStatus updates the status of a message
func (q *InMemoryMessageQueue) UpdateMessageStatus(ctx context.Context, messageID string, status MessageStatus) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.status[messageID] = status
	return nil
}

// GetMessageStatus gets the status of a message
func (q *InMemoryMessageQueue) GetMessageStatus(messageID string) (MessageStatus, bool) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	status, exists := q.status[messageID]
	return status, exists
}

// DefaultMessageSplitter provides message splitting for long SMS
type DefaultMessageSplitter struct {
	singleSMSLimits map[uint8]int // data coding -> max length
	concatSMSLimits map[uint8]int // data coding -> max length per part
}

// NewDefaultMessageSplitter creates a new message splitter
func NewDefaultMessageSplitter() *DefaultMessageSplitter {
	return &DefaultMessageSplitter{
		singleSMSLimits: map[uint8]int{
			0: 160, // GSM 7-bit
			3: 160, // Latin-1
			8: 70,  // UCS2
			4: 140, // 8-bit binary
		},
		concatSMSLimits: map[uint8]int{
			0: 153, // GSM 7-bit (160 - 7 for UDH)
			3: 153, // Latin-1 (160 - 7 for UDH)
			8: 67,  // UCS2 (70 - 3 for UDH, accounting for 2-byte chars)
			4: 133, // 8-bit binary (140 - 7 for UDH)
		},
	}
}

// SplitMessage splits a long message into multiple parts
func (s *DefaultMessageSplitter) SplitMessage(message *Message) ([]*Message, error) {
	// Get limits for this data coding
	singleLimit, exists := s.singleSMSLimits[message.DataCoding]
	if !exists {
		singleLimit = 160 // Default to GSM 7-bit
	}

	concatLimit, exists := s.concatSMSLimits[message.DataCoding]
	if !exists {
		concatLimit = 153 // Default to GSM 7-bit with UDH
	}

	// Calculate effective length based on encoding
	var textLength int
	if message.DataCoding == 8 { // UCS2
		textLength = len([]rune(message.Text))
	} else {
		textLength = len(message.Text)
	}

	// If message fits in single SMS, return as-is
	if textLength <= singleLimit {
		return []*Message{message}, nil
	}

	// Split message text based on character boundaries
	var parts []string
	if message.DataCoding == 8 { // UCS2 - split by runes
		parts = s.splitTextByRunes(message.Text, concatLimit)
	} else {
		parts = s.splitTextByBytes(message.Text, concatLimit)
	}

	if len(parts) > 255 {
		return nil, fmt.Errorf("message too long: would require %d parts (max 255)", len(parts))
	}

	// Create message parts
	messages := make([]*Message, len(parts))
	refNum := s.generateReferenceNumber(message.ID)

	for i, part := range parts {
		msgCopy := *message
		msgCopy.ID = fmt.Sprintf("%s_%d", message.ID, i+1)
		msgCopy.Text = part
		msgCopy.EsmClass |= EsmClassUDHI // Set UDHI flag

		// Create UDH for concatenated SMS
		udh := []byte{
			0x00, 0x03, // IEI: Concatenated SMS, IEDL: 3
			uint8(refNum >> 8),   // Reference number high byte
			uint8(refNum & 0xFF), // Reference number low byte
			uint8(len(parts)),    // Total parts
			uint8(i + 1),         // Current part
		}

		// Encode part with UDH
		encoded, err := s.encodeWithUDH(part, udh, message.DataCoding)
		if err != nil {
			return nil, fmt.Errorf("failed to encode message part %d: %w", i+1, err)
		}

		msgCopy.ShortMessage = encoded
		messages[i] = &msgCopy
	}

	return messages, nil
}

// generateReferenceNumber generates a reference number based on message ID
func (s *DefaultMessageSplitter) generateReferenceNumber(messageID string) uint16 {
	// Create a hash of the message ID to ensure uniqueness
	hash := uint32(0)
	for _, b := range []byte(messageID) {
		hash = hash*31 + uint32(b)
	}
	return uint16(hash & 0xFFFF)
}

// splitTextByBytes splits text by byte length for single-byte encodings
func (s *DefaultMessageSplitter) splitTextByBytes(text string, maxLength int) []string {
	if len(text) <= maxLength {
		return []string{text}
	}

	var parts []string
	for len(text) > maxLength {
		parts = append(parts, text[:maxLength])
		text = text[maxLength:]
	}
	if len(text) > 0 {
		parts = append(parts, text)
	}

	return parts
}

// splitTextByRunes splits text by rune count for multi-byte encodings
func (s *DefaultMessageSplitter) splitTextByRunes(text string, maxLength int) []string {
	runes := []rune(text)
	if len(runes) <= maxLength {
		return []string{text}
	}

	var parts []string
	for len(runes) > maxLength {
		parts = append(parts, string(runes[:maxLength]))
		runes = runes[maxLength:]
	}
	if len(runes) > 0 {
		parts = append(parts, string(runes))
	}

	return parts
}

// encodeWithUDH encodes a message part with UDH
func (s *DefaultMessageSplitter) encodeWithUDH(text string, udh []byte, dataCoding uint8) ([]byte, error) {
	var textBytes []byte
	var err error

	// Encode text based on data coding
	switch dataCoding {
	case 0, 3: // GSM 7-bit or Latin-1
		textBytes = []byte(text)
	case 8: // UCS2
		// Convert to UCS2/UTF-16BE
		runes := []rune(text)
		textBytes = make([]byte, len(runes)*2)
		for i, r := range runes {
			textBytes[i*2] = byte(r >> 8)
			textBytes[i*2+1] = byte(r & 0xFF)
		}
	case 4: // 8-bit binary
		textBytes = []byte(text)
	default:
		// Default to treating as Latin-1
		textBytes = []byte(text)
	}

	// Combine UDH length + UDH + text
	result := make([]byte, 1+len(udh)+len(textBytes))
	result[0] = byte(len(udh)) // UDH length
	copy(result[1:], udh)
	copy(result[1+len(udh):], textBytes)

	return result, err
}
