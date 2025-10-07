package smpp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// PDUEncoder handles encoding of PDUs to binary format
type PDUEncoder struct{}

// NewPDUEncoder creates a new PDU encoder
func NewPDUEncoder() *PDUEncoder {
	return &PDUEncoder{}
}

// Encode encodes a PDU to binary format
func (e *PDUEncoder) Encode(pdu *PDU) ([]byte, error) {
	// Marshal the body first to get its length
	bodyData, err := pdu.Body.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal PDU body: %w", err)
	}

	// Calculate total length (header + body)
	pdu.Header.CommandLength = uint32(16 + len(bodyData)) // 16 bytes for header
	pdu.Header.CommandID = pdu.Body.CommandID()

	// Create buffer for the complete PDU
	buf := new(bytes.Buffer)

	// Write header
	if err := binary.Write(buf, binary.BigEndian, pdu.Header.CommandLength); err != nil {
		return nil, fmt.Errorf("failed to write command length: %w", err)
	}
	if err := binary.Write(buf, binary.BigEndian, pdu.Header.CommandID); err != nil {
		return nil, fmt.Errorf("failed to write command ID: %w", err)
	}
	if err := binary.Write(buf, binary.BigEndian, pdu.Header.CommandStatus); err != nil {
		return nil, fmt.Errorf("failed to write command status: %w", err)
	}
	if err := binary.Write(buf, binary.BigEndian, pdu.Header.SequenceNum); err != nil {
		return nil, fmt.Errorf("failed to write sequence number: %w", err)
	}

	// Write body
	if _, err := buf.Write(bodyData); err != nil {
		return nil, fmt.Errorf("failed to write PDU body: %w", err)
	}

	return buf.Bytes(), nil
}

// PDUDecoder handles decoding of PDUs from binary format
type PDUDecoder struct{}

// NewPDUDecoder creates a new PDU decoder
func NewPDUDecoder() *PDUDecoder {
	return &PDUDecoder{}
}

// Decode decodes a PDU from binary format
func (d *PDUDecoder) Decode(data []byte) (*PDU, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("insufficient data for PDU header: got %d bytes, need at least 16", len(data))
	}

	// Parse header
	header := PDUHeader{}
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.BigEndian, &header.CommandLength); err != nil {
		return nil, fmt.Errorf("failed to read command length: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &header.CommandID); err != nil {
		return nil, fmt.Errorf("failed to read command ID: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &header.CommandStatus); err != nil {
		return nil, fmt.Errorf("failed to read command status: %w", err)
	}
	if err := binary.Read(buf, binary.BigEndian, &header.SequenceNum); err != nil {
		return nil, fmt.Errorf("failed to read sequence number: %w", err)
	}

	// Validate command length
	if header.CommandLength < 16 {
		return nil, fmt.Errorf("invalid command length: %d", header.CommandLength)
	}
	if uint32(len(data)) < header.CommandLength {
		return nil, fmt.Errorf("insufficient data: expected %d bytes, got %d", header.CommandLength, len(data))
	}

	// Extract body data
	bodyLen := header.CommandLength - 16
	bodyData := make([]byte, bodyLen)
	if bodyLen > 0 {
		if _, err := buf.Read(bodyData); err != nil {
			return nil, fmt.Errorf("failed to read PDU body: %w", err)
		}
	}

	// Create appropriate PDU body based on command ID
	body, err := d.createPDUBody(header.CommandID)
	if err != nil {
		return nil, fmt.Errorf("failed to create PDU body: %w", err)
	}

	// Unmarshal body
	if len(bodyData) > 0 {
		if err := body.Unmarshal(bodyData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal PDU body: %w", err)
		}
	}

	return &PDU{
		Header: header,
		Body:   body,
	}, nil
}

// DecodeFromReader decodes a PDU from an io.Reader
func (d *PDUDecoder) DecodeFromReader(reader io.Reader) (*PDU, error) {
	// Read header first
	headerBuf := make([]byte, 16)
	if _, err := io.ReadFull(reader, headerBuf); err != nil {
		return nil, fmt.Errorf("failed to read PDU header: %w", err)
	}

	// Parse command length
	commandLength := binary.BigEndian.Uint32(headerBuf[0:4])
	if commandLength < 16 {
		return nil, fmt.Errorf("invalid command length: %d", commandLength)
	}

	// Read remaining data
	remainingLen := commandLength - 16
	totalData := make([]byte, commandLength)
	copy(totalData, headerBuf)

	if remainingLen > 0 {
		if _, err := io.ReadFull(reader, totalData[16:]); err != nil {
			return nil, fmt.Errorf("failed to read PDU body: %w", err)
		}
	}

	return d.Decode(totalData)
}

// createPDUBody creates the appropriate PDU body based on command ID
func (d *PDUDecoder) createPDUBody(commandID uint32) (PDUBody, error) {
	switch commandID {
	case CommandBindReceiver:
		return &BindRequest{}, nil
	case CommandBindTransmitter:
		return &BindRequest{}, nil
	case CommandBindTransceiver:
		return &BindRequest{}, nil
	case CommandBindReceiverResp:
		return &BindResponse{}, nil
	case CommandBindTransmitterResp:
		return &BindResponse{}, nil
	case CommandBindTransceiverResp:
		return &BindResponse{}, nil
	case CommandSubmitSM:
		return &SubmitSM{}, nil
	case CommandSubmitSMResp:
		return &SubmitSMResp{}, nil
	case CommandDeliverSM:
		return &DeliverSM{}, nil
	case CommandDeliverSMResp:
		return &DeliverSMResp{}, nil
	case CommandEnquireLink:
		return &EnquireLink{}, nil
	case CommandEnquireLinkResp:
		return &EnquireLinkResp{}, nil
	case CommandUnbind:
		return &Unbind{}, nil
	case CommandUnbindResp:
		return &UnbindResp{}, nil
	default:
		return nil, fmt.Errorf("unsupported command ID: 0x%08X", commandID)
	}
}

// PDUBuilder helps building PDUs
type PDUBuilder struct {
	sequenceNum uint32
}

// NewPDUBuilder creates a new PDU builder
func NewPDUBuilder() *PDUBuilder {
	return &PDUBuilder{
		sequenceNum: 1,
	}
}

// BuildBindTransceiver builds a bind_transceiver PDU
func (b *PDUBuilder) BuildBindTransceiver(systemID, password, systemType string) *PDU {
	body := &BindRequest{
		SystemID:         systemID,
		Password:         password,
		SystemType:       systemType,
		InterfaceVersion: SMPPVersion,
		AddrTON:          TONUnknown,
		AddrNPI:          NPIUnknown,
		AddressRange:     "",
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandBindTransceiver,
			CommandStatus: StatusOK,
			SequenceNum:   b.getNextSequence(),
		},
		Body: body,
	}
}

// BuildBindTransceiverResp builds a bind_transceiver_resp PDU
func (b *PDUBuilder) BuildBindTransceiverResp(systemID string, sequenceNum uint32, status uint32) *PDU {
	body := &BindResponse{
		SystemID: systemID,
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandBindTransceiverResp,
			CommandStatus: status,
			SequenceNum:   sequenceNum,
		},
		Body: body,
	}
}

// BuildSubmitSM builds a submit_sm PDU
func (b *PDUBuilder) BuildSubmitSM(sourceAddr, destAddr, message string) *PDU {
	messageBytes := []byte(message)
	var shortMessage []byte
	var optionalParams []OptionalParameter

	// If message is longer than 255 bytes, use message_payload optional parameter
	if len(messageBytes) > 255 {
		shortMessage = []byte{} // Empty short message
		optionalParams = []OptionalParameter{
			{
				Tag:    TagMessagePayload,
				Length: uint16(len(messageBytes)),
				Value:  messageBytes,
			},
		}
	} else {
		shortMessage = messageBytes
		optionalParams = []OptionalParameter{}
	}

	body := &SubmitSM{
		ServiceType:          "",
		SourceAddrTON:        TONInternational,
		SourceAddrNPI:        NPIISDN,
		SourceAddr:           sourceAddr,
		DestAddrTON:          TONInternational,
		DestAddrNPI:          NPIISDN,
		DestAddr:             destAddr,
		EsmClass:             EsmClassDefault,
		ProtocolID:           0,
		PriorityFlag:         PriorityLevel0,
		ScheduleDeliveryTime: "",
		ValidityPeriod:       "",
		RegisteredDelivery:   RegisteredDeliverySuccessFailure,
		ReplaceIfPresentFlag: 0,
		DataCoding:           DataCodingDefault,
		SMDefaultMsgID:       0,
		SMLength:             uint8(len(shortMessage)),
		ShortMessage:         shortMessage,
		OptionalParams:       optionalParams,
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandSubmitSM,
			CommandStatus: StatusOK,
			SequenceNum:   b.getNextSequence(),
		},
		Body: body,
	}
}

// BuildSubmitSMResp builds a submit_sm_resp PDU
func (b *PDUBuilder) BuildSubmitSMResp(messageID string, sequenceNum uint32, status uint32) *PDU {
	body := &SubmitSMResp{
		MessageID: messageID,
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandSubmitSMResp,
			CommandStatus: status,
			SequenceNum:   sequenceNum,
		},
		Body: body,
	}
}

// BuildQuerySMResp builds a query_sm_resp PDU
func (b *PDUBuilder) BuildQuerySMResp(messageID CString, finalDate string, messageState, errorCode uint8, sequenceNum uint32, status uint32) *PDU {
	finalDateCStr := NewCString(17)
	finalDateCStr.SetString(finalDate)

	body := &QuerySMResp{
		MessageID:    messageID,
		FinalDate:    *finalDateCStr,
		MessageState: messageState,
		ErrorCode:    errorCode,
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandQuerySMResp,
			CommandStatus: status,
			SequenceNum:   sequenceNum,
		},
		Body: body,
	}
}

// BuildReplaceSMResp builds a replace_sm_resp PDU
func (b *PDUBuilder) BuildReplaceSMResp(sequenceNum uint32, status uint32) *PDU {
	body := &ReplaceSMResp{}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandReplaceSMResp,
			CommandStatus: status,
			SequenceNum:   sequenceNum,
		},
		Body: body,
	}
}

// BuildCancelSMResp builds a cancel_sm_resp PDU
func (b *PDUBuilder) BuildCancelSMResp(sequenceNum uint32, status uint32) *PDU {
	body := &CancelSMResp{}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandCancelSMResp,
			CommandStatus: status,
			SequenceNum:   sequenceNum,
		},
		Body: body,
	}
}

// BuildSubmitMultiResp builds a submit_multi_resp PDU
func (b *PDUBuilder) BuildSubmitMultiResp(messageID string, unsuccessfulSMEs []UnsuccessfulSME, sequenceNum uint32, status uint32) *PDU {
	messageIDCStr := NewCString(65)
	messageIDCStr.SetString(messageID)

	body := &SubmitMultiResp{
		MessageID:    *messageIDCStr,
		NoUnsuccess:  uint8(len(unsuccessfulSMEs)),
		UnsuccessSME: unsuccessfulSMEs,
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandSubmitMultiResp,
			CommandStatus: status,
			SequenceNum:   sequenceNum,
		},
		Body: body,
	}
}

// BuildDataSMResp builds a data_sm_resp PDU
func (b *PDUBuilder) BuildDataSMResp(messageID string, sequenceNum uint32, status uint32) *PDU {
	messageIDCStr := NewCString(65)
	messageIDCStr.SetString(messageID)

	body := &DataSMResp{
		MessageID: *messageIDCStr,
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandDataSMResp,
			CommandStatus: status,
			SequenceNum:   sequenceNum,
		},
		Body: body,
	}
}

// BuildDeliverSM builds a deliver_sm PDU
func (b *PDUBuilder) BuildDeliverSM(sourceAddr, destAddr string, message []byte, dataCoding uint8) *PDU {
	body := &DeliverSM{
		ServiceType:          "",
		SourceAddrTON:        TONInternational,
		SourceAddrNPI:        NPIISDN,
		SourceAddr:           sourceAddr,
		DestAddrTON:          TONInternational,
		DestAddrNPI:          NPIISDN,
		DestAddr:             destAddr,
		EsmClass:             EsmClassDefault,
		ProtocolID:           0,
		PriorityFlag:         PriorityLevel0,
		ScheduleDeliveryTime: "",
		ValidityPeriod:       "",
		RegisteredDelivery:   0,
		ReplaceIfPresentFlag: 0,
		DataCoding:           dataCoding,
		SMDefaultMsgID:       0,
		SMLength:             uint8(len(message)),
		ShortMessage:         message,
		OptionalParams:       []OptionalParameter{},
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandDeliverSM,
			CommandStatus: StatusOK,
			SequenceNum:   b.getNextSequence(),
		},
		Body: body,
	}
}

// BuildDeliverSMResp builds a deliver_sm_resp PDU
func (b *PDUBuilder) BuildDeliverSMResp(messageID string, sequenceNum uint32, status uint32) *PDU {
	body := &DeliverSMResp{
		MessageID: messageID,
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandDeliverSMResp,
			CommandStatus: status,
			SequenceNum:   sequenceNum,
		},
		Body: body,
	}
}

// BuildEnquireLink builds an enquire_link PDU
func (b *PDUBuilder) BuildEnquireLink() *PDU {
	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandEnquireLink,
			CommandStatus: StatusOK,
			SequenceNum:   b.getNextSequence(),
		},
		Body: &EnquireLink{},
	}
}

// BuildEnquireLinkResp builds an enquire_link_resp PDU
func (b *PDUBuilder) BuildEnquireLinkResp(sequenceNum uint32) *PDU {
	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandEnquireLinkResp,
			CommandStatus: StatusOK,
			SequenceNum:   sequenceNum,
		},
		Body: &EnquireLinkResp{},
	}
}

// BuildUnbind builds an unbind PDU
func (b *PDUBuilder) BuildUnbind() *PDU {
	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandUnbind,
			CommandStatus: StatusOK,
			SequenceNum:   b.getNextSequence(),
		},
		Body: &Unbind{},
	}
}

// BuildUnbindResp builds an unbind_resp PDU
func (b *PDUBuilder) BuildUnbindResp(sequenceNum uint32) *PDU {
	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandUnbindResp,
			CommandStatus: StatusOK,
			SequenceNum:   sequenceNum,
		},
		Body: &UnbindResp{},
	}
}

// BuildGenericNack builds a generic_nack PDU
func (b *PDUBuilder) BuildGenericNack(sequenceNum uint32, status uint32) *PDU {
	return &PDU{
		Header: PDUHeader{
			CommandID:     0x80000000, // Generic NACK
			CommandStatus: status,
			SequenceNum:   sequenceNum,
		},
		Body: &GenericNack{},
	}
}

// GenericNack represents a generic_nack PDU
type GenericNack struct{}

func (g *GenericNack) Marshal() ([]byte, error) {
	return []byte{}, nil
}

func (g *GenericNack) Unmarshal(data []byte) error {
	return nil
}

func (g *GenericNack) CommandID() uint32 {
	return 0x80000000
}

// getNextSequence returns the next sequence number
func (b *PDUBuilder) getNextSequence() uint32 {
	seq := b.sequenceNum
	b.sequenceNum++
	if b.sequenceNum == 0 {
		b.sequenceNum = 1
	}
	return seq
}

// SetSequenceNum sets the current sequence number
func (b *PDUBuilder) SetSequenceNum(seq uint32) {
	if seq == 0 {
		seq = 1
	}
	b.sequenceNum = seq
}

// GetCurrentSequence returns the current sequence number without incrementing
func (b *PDUBuilder) GetCurrentSequence() uint32 {
	return b.sequenceNum
}
