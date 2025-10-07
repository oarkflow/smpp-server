package smpp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

// PDU represents the base Protocol Data Unit
type PDU struct {
	Header PDUHeader
	Body   PDUBody
}

// PDUHeader represents the SMPP PDU header
type PDUHeader struct {
	CommandLength uint32
	CommandID     uint32
	CommandStatus uint32
	SequenceNum   uint32
}

// PDUBody represents the PDU body interface
type PDUBody interface {
	Marshal() ([]byte, error)
	Unmarshal(data []byte) error
	CommandID() uint32
}

// CString represents a NULL-terminated C string
type CString struct {
	Data   []byte
	MaxLen int
}

// NewCString creates a new CString with the given maximum length
func NewCString(maxLen int) *CString {
	return &CString{
		Data:   make([]byte, 0),
		MaxLen: maxLen,
	}
}

// SetString sets the string value
func (cs *CString) SetString(value string) error {
	if len(value) > cs.MaxLen {
		return fmt.Errorf("string too long: %d > %d", len(value), cs.MaxLen)
	}
	cs.Data = []byte(value)
	return nil
}

// GetString returns the string value
func (cs *CString) GetString() string {
	return string(cs.Data)
}

// Marshal serializes CString to bytes
func (cs *CString) Marshal() ([]byte, error) {
	// C strings are null-terminated
	result := make([]byte, len(cs.Data)+1)
	copy(result, cs.Data)
	result[len(cs.Data)] = 0 // null terminator
	return result, nil
}

// Unmarshal deserializes bytes to CString
func (cs *CString) Unmarshal(data []byte) error {
	if len(data) == 0 {
		cs.Data = []byte{}
		return nil
	}

	// Find null terminator
	nullIndex := -1
	for i, b := range data {
		if b == 0 {
			nullIndex = i
			break
		}
	}

	if nullIndex == -1 {
		return fmt.Errorf("CString missing null terminator")
	}

	if nullIndex > cs.MaxLen {
		return fmt.Errorf("CString too long: %d > %d", nullIndex, cs.MaxLen)
	}

	cs.Data = make([]byte, nullIndex)
	copy(cs.Data, data[:nullIndex])
	return nil
}

// Len returns the length including null terminator
func (cs *CString) Len() int {
	return len(cs.Data) + 1
}

// QuerySM represents a query_sm PDU
type QuerySM struct {
	MessageID  CString `json:"message_id"`
	SourceAddr Address `json:"source_addr"`
}

// QuerySMResp represents a query_sm_resp PDU
type QuerySMResp struct {
	MessageID    CString `json:"message_id"`
	FinalDate    CString `json:"final_date"`
	MessageState uint8   `json:"message_state"`
	ErrorCode    uint8   `json:"error_code"`
}

// ReplaceSM represents a replace_sm PDU
type ReplaceSM struct {
	MessageID            CString `json:"message_id"`
	SourceAddr           Address `json:"source_addr"`
	ScheduleDeliveryTime CString `json:"schedule_delivery_time"`
	ValidityPeriod       CString `json:"validity_period"`
	RegisteredDelivery   uint8   `json:"registered_delivery"`
	SMDefaultMsgID       uint8   `json:"sm_default_msg_id"`
	SMLength             uint8   `json:"sm_length"`
	ShortMessage         []byte  `json:"short_message"`
}

// ReplaceSMResp represents a replace_sm_resp PDU
type ReplaceSMResp struct{}

// CancelSM represents a cancel_sm PDU
type CancelSM struct {
	ServiceType     CString `json:"service_type"`
	MessageID       CString `json:"message_id"`
	SourceAddrTON   uint8   `json:"source_addr_ton"`
	SourceAddrNPI   uint8   `json:"source_addr_npi"`
	SourceAddr      CString `json:"source_addr"`
	DestAddrTON     uint8   `json:"dest_addr_ton"`
	DestAddrNPI     uint8   `json:"dest_addr_npi"`
	DestinationAddr CString `json:"destination_addr"`
}

// CancelSMResp represents a cancel_sm_resp PDU
type CancelSMResp struct{}

// SubmitMulti represents a submit_multi PDU
type SubmitMulti struct {
	ServiceType          CString              `json:"service_type"`
	SourceAddr           Address              `json:"source_addr"`
	NumberOfDests        uint8                `json:"number_of_dests"`
	DestAddresses        []DestinationAddress `json:"dest_addresses"`
	ESMClass             uint8                `json:"esm_class"`
	ProtocolID           uint8                `json:"protocol_id"`
	PriorityFlag         uint8                `json:"priority_flag"`
	ScheduleDeliveryTime CString              `json:"schedule_delivery_time"`
	ValidityPeriod       CString              `json:"validity_period"`
	RegisteredDelivery   uint8                `json:"registered_delivery"`
	ReplaceIfPresentFlag uint8                `json:"replace_if_present_flag"`
	DataCoding           uint8                `json:"data_coding"`
	SMDefaultMsgID       uint8                `json:"sm_default_msg_id"`
	SMLength             uint8                `json:"sm_length"`
	ShortMessage         []byte               `json:"short_message"`
	OptionalParameters   []OptionalParameter  `json:"optional_parameters"`
}

// DestinationAddress represents a destination address for submit_multi
type DestinationAddress struct {
	DestFlag        uint8   `json:"dest_flag"`
	DestAddrTON     uint8   `json:"dest_addr_ton"`    // Only if DestFlag == 1
	DestAddrNPI     uint8   `json:"dest_addr_npi"`    // Only if DestFlag == 1
	DestinationAddr CString `json:"destination_addr"` // Only if DestFlag == 1
	DLName          CString `json:"dl_name"`          // Only if DestFlag == 2
}

// SubmitMultiResp represents a submit_multi_resp PDU
type SubmitMultiResp struct {
	MessageID    CString           `json:"message_id"`
	NoUnsuccess  uint8             `json:"no_unsuccess"`
	UnsuccessSME []UnsuccessfulSME `json:"unsuccess_sme"`
}

// UnsuccessfulSME represents an unsuccessful SME in submit_multi_resp
type UnsuccessfulSME struct {
	DestAddrTON     uint8   `json:"dest_addr_ton"`
	DestAddrNPI     uint8   `json:"dest_addr_npi"`
	DestinationAddr CString `json:"destination_addr"`
	ErrorStatusCode uint32  `json:"error_status_code"`
}

// AlertNotification represents an alert_notification PDU
type AlertNotification struct {
	SourceAddr         Address             `json:"source_addr"`
	ESMEAddr           Address             `json:"esme_addr"`
	OptionalParameters []OptionalParameter `json:"optional_parameters"`
}

// DataSM represents a data_sm PDU
type DataSM struct {
	ServiceType        CString             `json:"service_type"`
	SourceAddr         Address             `json:"source_addr"`
	DestAddr           Address             `json:"dest_addr"`
	ESMClass           uint8               `json:"esm_class"`
	RegisteredDelivery uint8               `json:"registered_delivery"`
	DataCoding         uint8               `json:"data_coding"`
	OptionalParameters []OptionalParameter `json:"optional_parameters"`
}

// DataSMResp represents a data_sm_resp PDU
type DataSMResp struct {
	MessageID          CString             `json:"message_id"`
	OptionalParameters []OptionalParameter `json:"optional_parameters"`
}

// Outbind represents an outbind PDU
type Outbind struct {
	SystemID CString `json:"system_id"`
	Password CString `json:"password"`
}

// SMPP v5.0 PDUs

// BroadcastSM represents a broadcast_sm PDU (SMPP v5.0)
type BroadcastSM struct {
	ServiceType          CString             `json:"service_type"`
	SourceAddr           Address             `json:"source_addr"`
	MessageID            CString             `json:"message_id"`
	PriorityFlag         uint8               `json:"priority_flag"`
	ScheduleDeliveryTime CString             `json:"schedule_delivery_time"`
	ValidityPeriod       CString             `json:"validity_period"`
	ReplaceIfPresentFlag uint8               `json:"replace_if_present_flag"`
	DataCoding           uint8               `json:"data_coding"`
	SMDefaultMsgID       uint8               `json:"sm_default_msg_id"`
	OptionalParameters   []OptionalParameter `json:"optional_parameters"`
}

// BroadcastSMResp represents a broadcast_sm_resp PDU (SMPP v5.0)
type BroadcastSMResp struct {
	MessageID          CString             `json:"message_id"`
	OptionalParameters []OptionalParameter `json:"optional_parameters"`
}

// QueryBroadcastSM represents a query_broadcast_sm PDU (SMPP v5.0)
type QueryBroadcastSM struct {
	MessageID          CString             `json:"message_id"`
	SourceAddr         Address             `json:"source_addr"`
	OptionalParameters []OptionalParameter `json:"optional_parameters"`
}

// QueryBroadcastSMResp represents a query_broadcast_sm_resp PDU (SMPP v5.0)
type QueryBroadcastSMResp struct {
	MessageID          CString             `json:"message_id"`
	MessageState       uint8               `json:"message_state"`
	BroadcastAreaCount uint8               `json:"broadcast_area_count"`
	BroadcastAreas     []BroadcastArea     `json:"broadcast_areas"`
	OptionalParameters []OptionalParameter `json:"optional_parameters"`
}

// BroadcastArea represents a broadcast area (SMPP v5.0)
type BroadcastArea struct {
	AreaIdentifier uint16  `json:"area_identifier"`
	AreaName       CString `json:"area_name"`
}

// CancelBroadcastSM represents a cancel_broadcast_sm PDU (SMPP v5.0)
type CancelBroadcastSM struct {
	ServiceType        CString             `json:"service_type"`
	MessageID          CString             `json:"message_id"`
	SourceAddr         Address             `json:"source_addr"`
	OptionalParameters []OptionalParameter `json:"optional_parameters"`
}

// CancelBroadcastSMResp represents a cancel_broadcast_sm_resp PDU (SMPP v5.0)
type CancelBroadcastSMResp struct {
	MessageID          CString             `json:"message_id"`
	OptionalParameters []OptionalParameter `json:"optional_parameters"`
}

// Marshal methods for new PDUs

// Marshal serializes QuerySM to bytes
func (q *QuerySM) Marshal() ([]byte, error) {
	var buf []byte

	msgIDBytes, err := q.MessageID.Marshal()
	if err != nil {
		return nil, err
	}
	buf = append(buf, msgIDBytes...)

	addrBytes, err := q.SourceAddr.Marshal()
	if err != nil {
		return nil, err
	}
	buf = append(buf, addrBytes...)

	return buf, nil
}

// Unmarshal deserializes bytes to QuerySM
func (q *QuerySM) Unmarshal(data []byte) error {
	offset := 0

	if err := q.MessageID.Unmarshal(data[offset:]); err != nil {
		return err
	}
	offset += q.MessageID.Len()

	if err := q.SourceAddr.Unmarshal(data[offset:]); err != nil {
		return err
	}

	return nil
}

// Marshal serializes QuerySMResp to bytes
func (q *QuerySMResp) Marshal() ([]byte, error) {
	var buf []byte

	msgIDBytes, err := q.MessageID.Marshal()
	if err != nil {
		return nil, err
	}
	buf = append(buf, msgIDBytes...)

	finalDateBytes, err := q.FinalDate.Marshal()
	if err != nil {
		return nil, err
	}
	buf = append(buf, finalDateBytes...)

	buf = append(buf, q.MessageState)
	buf = append(buf, q.ErrorCode)

	return buf, nil
}

// Unmarshal deserializes bytes to QuerySMResp
func (q *QuerySMResp) Unmarshal(data []byte) error {
	offset := 0

	if err := q.MessageID.Unmarshal(data[offset:]); err != nil {
		return err
	}
	offset += q.MessageID.Len()

	if err := q.FinalDate.Unmarshal(data[offset:]); err != nil {
		return err
	}
	offset += q.FinalDate.Len()

	if offset >= len(data) {
		return fmt.Errorf("insufficient data for message state")
	}
	q.MessageState = data[offset]
	offset++

	if offset >= len(data) {
		return fmt.Errorf("insufficient data for error code")
	}
	q.ErrorCode = data[offset]

	return nil
}

// Marshal serializes ReplaceSM to bytes
func (r *ReplaceSM) Marshal() ([]byte, error) {
	var buf []byte

	msgIDBytes, err := r.MessageID.Marshal()
	if err != nil {
		return nil, err
	}
	buf = append(buf, msgIDBytes...)

	addrBytes, err := r.SourceAddr.Marshal()
	if err != nil {
		return nil, err
	}
	buf = append(buf, addrBytes...)

	scheduleBytes, err := r.ScheduleDeliveryTime.Marshal()
	if err != nil {
		return nil, err
	}
	buf = append(buf, scheduleBytes...)

	validityBytes, err := r.ValidityPeriod.Marshal()
	if err != nil {
		return nil, err
	}
	buf = append(buf, validityBytes...)

	buf = append(buf, r.RegisteredDelivery)
	buf = append(buf, r.SMDefaultMsgID)
	buf = append(buf, r.SMLength)
	buf = append(buf, r.ShortMessage...)

	return buf, nil
}

// Unmarshal deserializes bytes to ReplaceSM
func (r *ReplaceSM) Unmarshal(data []byte) error {
	offset := 0

	if err := r.MessageID.Unmarshal(data[offset:]); err != nil {
		return err
	}
	offset += r.MessageID.Len()

	if err := r.SourceAddr.Unmarshal(data[offset:]); err != nil {
		return err
	}
	offset += r.SourceAddr.Len()

	if err := r.ScheduleDeliveryTime.Unmarshal(data[offset:]); err != nil {
		return err
	}
	offset += r.ScheduleDeliveryTime.Len()

	if err := r.ValidityPeriod.Unmarshal(data[offset:]); err != nil {
		return err
	}
	offset += r.ValidityPeriod.Len()

	if offset >= len(data) {
		return fmt.Errorf("insufficient data for registered delivery")
	}
	r.RegisteredDelivery = data[offset]
	offset++

	if offset >= len(data) {
		return fmt.Errorf("insufficient data for sm default msg id")
	}
	r.SMDefaultMsgID = data[offset]
	offset++

	if offset >= len(data) {
		return fmt.Errorf("insufficient data for sm length")
	}
	r.SMLength = data[offset]
	offset++

	if offset+int(r.SMLength) > len(data) {
		return fmt.Errorf("insufficient data for short message")
	}
	r.ShortMessage = make([]byte, r.SMLength)
	copy(r.ShortMessage, data[offset:offset+int(r.SMLength)])

	return nil
}

// ReplaceSMResp Marshal and Unmarshal (empty PDU)
func (r *ReplaceSMResp) Marshal() ([]byte, error) {
	return []byte{}, nil
}

func (r *ReplaceSMResp) Unmarshal(data []byte) error {
	return nil
}

// CancelSM Marshal and Unmarshal
func (c *CancelSM) Marshal() ([]byte, error) {
	var buf []byte

	serviceBytes, err := c.ServiceType.Marshal()
	if err != nil {
		return nil, err
	}
	buf = append(buf, serviceBytes...)

	msgIDBytes, err := c.MessageID.Marshal()
	if err != nil {
		return nil, err
	}
	buf = append(buf, msgIDBytes...)

	buf = append(buf, c.SourceAddrTON)
	buf = append(buf, c.SourceAddrNPI)

	srcAddrBytes, err := c.SourceAddr.Marshal()
	if err != nil {
		return nil, err
	}
	buf = append(buf, srcAddrBytes...)

	buf = append(buf, c.DestAddrTON)
	buf = append(buf, c.DestAddrNPI)

	destAddrBytes, err := c.DestinationAddr.Marshal()
	if err != nil {
		return nil, err
	}
	buf = append(buf, destAddrBytes...)

	return buf, nil
}

func (c *CancelSM) Unmarshal(data []byte) error {
	offset := 0

	if err := c.ServiceType.Unmarshal(data[offset:]); err != nil {
		return err
	}
	offset += c.ServiceType.Len()

	if err := c.MessageID.Unmarshal(data[offset:]); err != nil {
		return err
	}
	offset += c.MessageID.Len()

	if offset >= len(data) {
		return fmt.Errorf("insufficient data for source addr ton")
	}
	c.SourceAddrTON = data[offset]
	offset++

	if offset >= len(data) {
		return fmt.Errorf("insufficient data for source addr npi")
	}
	c.SourceAddrNPI = data[offset]
	offset++

	if err := c.SourceAddr.Unmarshal(data[offset:]); err != nil {
		return err
	}
	offset += c.SourceAddr.Len()

	if offset >= len(data) {
		return fmt.Errorf("insufficient data for dest addr ton")
	}
	c.DestAddrTON = data[offset]
	offset++

	if offset >= len(data) {
		return fmt.Errorf("insufficient data for dest addr npi")
	}
	c.DestAddrNPI = data[offset]
	offset++

	if err := c.DestinationAddr.Unmarshal(data[offset:]); err != nil {
		return err
	}

	return nil
}

// CancelSMResp Marshal and Unmarshal (empty PDU)
func (c *CancelSMResp) Marshal() ([]byte, error) {
	return []byte{}, nil
}

func (c *CancelSMResp) Unmarshal(data []byte) error {
	return nil
}

// String returns the string value
func (cs *CString) String() string {
	if len(cs.Data) == 0 {
		return ""
	}
	// Remove NULL terminator if present
	if cs.Data[len(cs.Data)-1] == 0 {
		return string(cs.Data[:len(cs.Data)-1])
	}
	return string(cs.Data)
}

// OptionalParameter represents an optional parameter
type OptionalParameter struct {
	Tag    uint16
	Length uint16
	Value  []byte
}

// Address represents an SMPP address
type Address struct {
	TON  uint8
	NPI  uint8
	Addr string
}

// Marshal serializes Address to bytes
func (a *Address) Marshal() ([]byte, error) {
	addrBytes := []byte(a.Addr)
	result := make([]byte, 2+len(addrBytes)+1)
	result[0] = a.TON
	result[1] = a.NPI
	copy(result[2:], addrBytes)
	result[len(result)-1] = 0 // null terminator
	return result, nil
}

// Unmarshal deserializes bytes to Address
func (a *Address) Unmarshal(data []byte) error {
	if len(data) < 3 {
		return fmt.Errorf("Address data too short")
	}

	a.TON = data[0]
	a.NPI = data[1]

	// Find null terminator for address
	nullIndex := -1
	for i := 2; i < len(data); i++ {
		if data[i] == 0 {
			nullIndex = i
			break
		}
	}

	if nullIndex == -1 {
		return fmt.Errorf("Address missing null terminator")
	}

	a.Addr = string(data[2:nullIndex])
	return nil
}

// Len returns the marshaled length
func (a *Address) Len() int {
	return 2 + len(a.Addr) + 1
}

// BindRequest represents bind request PDUs
type BindRequest struct {
	SystemID         string
	Password         string
	SystemType       string
	InterfaceVersion uint8
	AddrTON          uint8
	AddrNPI          uint8
	AddressRange     string
}

func (b *BindRequest) Marshal() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write system_id (max 16 bytes, NULL terminated)
	systemID := make([]byte, MaxSystemIDLength)
	copy(systemID, []byte(b.SystemID))
	buf.Write(systemID)

	// Write password (max 9 bytes, NULL terminated)
	password := make([]byte, MaxPasswordLength)
	copy(password, []byte(b.Password))
	buf.Write(password)

	// Write system_type (max 13 bytes, NULL terminated)
	systemType := make([]byte, MaxSystemTypeLength)
	copy(systemType, []byte(b.SystemType))
	buf.Write(systemType)

	// Write interface_version
	buf.WriteByte(b.InterfaceVersion)

	// Write addr_ton
	buf.WriteByte(b.AddrTON)

	// Write addr_npi
	buf.WriteByte(b.AddrNPI)

	// Write address_range (max 41 bytes, NULL terminated)
	addressRange := make([]byte, 41)
	copy(addressRange, []byte(b.AddressRange))
	buf.Write(addressRange)

	return buf.Bytes(), nil
}

func (b *BindRequest) Unmarshal(data []byte) error {
	if len(data) < MaxSystemIDLength+MaxPasswordLength+MaxSystemTypeLength+3+41 {
		return fmt.Errorf("insufficient data for bind request")
	}

	offset := 0

	// Read system_id
	systemIDBytes := data[offset : offset+MaxSystemIDLength]
	b.SystemID = string(bytes.TrimRight(systemIDBytes, "\x00"))
	offset += MaxSystemIDLength

	// Read password
	passwordBytes := data[offset : offset+MaxPasswordLength]
	b.Password = string(bytes.TrimRight(passwordBytes, "\x00"))
	offset += MaxPasswordLength

	// Read system_type
	systemTypeBytes := data[offset : offset+MaxSystemTypeLength]
	b.SystemType = string(bytes.TrimRight(systemTypeBytes, "\x00"))
	offset += MaxSystemTypeLength

	// Read interface_version
	b.InterfaceVersion = data[offset]
	offset++

	// Read addr_ton
	b.AddrTON = data[offset]
	offset++

	// Read addr_npi
	b.AddrNPI = data[offset]
	offset++

	// Read address_range
	addressRangeBytes := data[offset : offset+41]
	b.AddressRange = string(bytes.TrimRight(addressRangeBytes, "\x00"))

	return nil
}

func (b *BindRequest) CommandID() uint32 {
	return CommandBindTransceiver // Default, should be overridden
}

// BindResponse represents bind response PDUs
type BindResponse struct {
	SystemID string
}

func (b *BindResponse) Marshal() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write system_id (NULL terminated)
	buf.WriteString(b.SystemID)
	buf.WriteByte(0)

	return buf.Bytes(), nil
}

func (b *BindResponse) Unmarshal(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("insufficient data for bind response")
	}

	// Find NULL terminator or use entire data
	nullIndex := bytes.IndexByte(data, 0)
	if nullIndex == -1 {
		b.SystemID = string(data)
	} else {
		b.SystemID = string(data[:nullIndex])
	}

	return nil
}

func (b *BindResponse) CommandID() uint32 {
	return CommandBindTransceiverResp // Default, should be overridden
}

// SubmitSM represents submit_sm PDU
type SubmitSM struct {
	ServiceType          string
	SourceAddrTON        uint8
	SourceAddrNPI        uint8
	SourceAddr           string
	DestAddrTON          uint8
	DestAddrNPI          uint8
	DestAddr             string
	EsmClass             uint8
	ProtocolID           uint8
	PriorityFlag         uint8
	ScheduleDeliveryTime string
	ValidityPeriod       string
	RegisteredDelivery   uint8
	ReplaceIfPresentFlag uint8
	DataCoding           uint8
	SMDefaultMsgID       uint8
	SMLength             uint8
	ShortMessage         []byte
	OptionalParams       []OptionalParameter
}

func (s *SubmitSM) Marshal() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Service type (max 6 bytes, NULL terminated)
	serviceType := make([]byte, MaxServiceTypeLength)
	copy(serviceType, []byte(s.ServiceType))
	buf.Write(serviceType)

	// Source address
	buf.WriteByte(s.SourceAddrTON)
	buf.WriteByte(s.SourceAddrNPI)
	sourceAddr := make([]byte, MaxAddressLength)
	copy(sourceAddr, []byte(s.SourceAddr))
	buf.Write(sourceAddr)

	// Destination address
	buf.WriteByte(s.DestAddrTON)
	buf.WriteByte(s.DestAddrNPI)
	destAddr := make([]byte, MaxAddressLength)
	copy(destAddr, []byte(s.DestAddr))
	buf.Write(destAddr)

	// Other fields
	buf.WriteByte(s.EsmClass)
	buf.WriteByte(s.ProtocolID)
	buf.WriteByte(s.PriorityFlag)

	// Schedule delivery time (max 17 bytes, NULL terminated)
	scheduleTime := make([]byte, 17)
	copy(scheduleTime, []byte(s.ScheduleDeliveryTime))
	buf.Write(scheduleTime)

	// Validity period (max 17 bytes, NULL terminated)
	validityPeriod := make([]byte, 17)
	copy(validityPeriod, []byte(s.ValidityPeriod))
	buf.Write(validityPeriod)

	buf.WriteByte(s.RegisteredDelivery)
	buf.WriteByte(s.ReplaceIfPresentFlag)
	buf.WriteByte(s.DataCoding)
	buf.WriteByte(s.SMDefaultMsgID)

	// Short message length and data
	buf.WriteByte(s.SMLength)
	buf.Write(s.ShortMessage)

	// Optional parameters
	for _, param := range s.OptionalParams {
		binary.Write(buf, binary.BigEndian, param.Tag)
		binary.Write(buf, binary.BigEndian, param.Length)
		buf.Write(param.Value)
	}

	return buf.Bytes(), nil
}

func (s *SubmitSM) Unmarshal(data []byte) error {
	minLength := MaxServiceTypeLength + 2 + MaxAddressLength + 2 + MaxAddressLength + 9 + 17 + 17
	if len(data) < minLength {
		return fmt.Errorf("insufficient data for submit_sm")
	}

	offset := 0

	// Service type
	serviceTypeBytes := data[offset : offset+MaxServiceTypeLength]
	s.ServiceType = string(bytes.TrimRight(serviceTypeBytes, "\x00"))
	offset += MaxServiceTypeLength

	// Source address
	s.SourceAddrTON = data[offset]
	offset++
	s.SourceAddrNPI = data[offset]
	offset++
	sourceAddrBytes := data[offset : offset+MaxAddressLength]
	s.SourceAddr = string(bytes.TrimRight(sourceAddrBytes, "\x00"))
	offset += MaxAddressLength

	// Destination address
	s.DestAddrTON = data[offset]
	offset++
	s.DestAddrNPI = data[offset]
	offset++
	destAddrBytes := data[offset : offset+MaxAddressLength]
	s.DestAddr = string(bytes.TrimRight(destAddrBytes, "\x00"))
	offset += MaxAddressLength

	// Other fields
	s.EsmClass = data[offset]
	offset++
	s.ProtocolID = data[offset]
	offset++
	s.PriorityFlag = data[offset]
	offset++

	// Schedule delivery time
	scheduleTimeBytes := data[offset : offset+17]
	s.ScheduleDeliveryTime = string(bytes.TrimRight(scheduleTimeBytes, "\x00"))
	offset += 17

	// Validity period
	validityPeriodBytes := data[offset : offset+17]
	s.ValidityPeriod = string(bytes.TrimRight(validityPeriodBytes, "\x00"))
	offset += 17

	s.RegisteredDelivery = data[offset]
	offset++
	s.ReplaceIfPresentFlag = data[offset]
	offset++
	s.DataCoding = data[offset]
	offset++
	s.SMDefaultMsgID = data[offset]
	offset++

	// Short message
	s.SMLength = data[offset]
	offset++

	if len(data) < offset+int(s.SMLength) {
		return fmt.Errorf("insufficient data for short message")
	}

	s.ShortMessage = make([]byte, s.SMLength)
	copy(s.ShortMessage, data[offset:offset+int(s.SMLength)])
	offset += int(s.SMLength)

	// Parse optional parameters
	s.OptionalParams = []OptionalParameter{}
	for offset < len(data) {
		if len(data) < offset+4 {
			break
		}

		param := OptionalParameter{}
		param.Tag = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
		param.Length = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2

		if len(data) < offset+int(param.Length) {
			break
		}

		param.Value = make([]byte, param.Length)
		copy(param.Value, data[offset:offset+int(param.Length)])
		offset += int(param.Length)

		s.OptionalParams = append(s.OptionalParams, param)
	}

	return nil
}

func (s *SubmitSM) CommandID() uint32 {
	return CommandSubmitSM
}

// SubmitSMResp represents submit_sm_resp PDU
type SubmitSMResp struct {
	MessageID string
}

func (s *SubmitSMResp) Marshal() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Message ID (NULL terminated)
	buf.WriteString(s.MessageID)
	buf.WriteByte(0)

	return buf.Bytes(), nil
}

func (s *SubmitSMResp) Unmarshal(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("insufficient data for submit_sm_resp")
	}

	// Find NULL terminator or use entire data
	nullIndex := bytes.IndexByte(data, 0)
	if nullIndex == -1 {
		s.MessageID = string(data)
	} else {
		s.MessageID = string(data[:nullIndex])
	}

	return nil
}

func (s *SubmitSMResp) CommandID() uint32 {
	return CommandSubmitSMResp
}

// DeliverSM represents deliver_sm PDU
type DeliverSM struct {
	ServiceType          string
	SourceAddrTON        uint8
	SourceAddrNPI        uint8
	SourceAddr           string
	DestAddrTON          uint8
	DestAddrNPI          uint8
	DestAddr             string
	EsmClass             uint8
	ProtocolID           uint8
	PriorityFlag         uint8
	ScheduleDeliveryTime string
	ValidityPeriod       string
	RegisteredDelivery   uint8
	ReplaceIfPresentFlag uint8
	DataCoding           uint8
	SMDefaultMsgID       uint8
	SMLength             uint8
	ShortMessage         []byte
	OptionalParams       []OptionalParameter
}

func (d *DeliverSM) Marshal() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Service type (max 6 bytes, NULL terminated)
	serviceType := make([]byte, MaxServiceTypeLength)
	copy(serviceType, []byte(d.ServiceType))
	buf.Write(serviceType)

	// Source address
	buf.WriteByte(d.SourceAddrTON)
	buf.WriteByte(d.SourceAddrNPI)
	sourceAddr := make([]byte, MaxAddressLength)
	copy(sourceAddr, []byte(d.SourceAddr))
	buf.Write(sourceAddr)

	// Destination address
	buf.WriteByte(d.DestAddrTON)
	buf.WriteByte(d.DestAddrNPI)
	destAddr := make([]byte, MaxAddressLength)
	copy(destAddr, []byte(d.DestAddr))
	buf.Write(destAddr)

	// Other fields
	buf.WriteByte(d.EsmClass)
	buf.WriteByte(d.ProtocolID)
	buf.WriteByte(d.PriorityFlag)

	// Schedule delivery time (max 17 bytes, NULL terminated)
	scheduleTime := make([]byte, 17)
	copy(scheduleTime, []byte(d.ScheduleDeliveryTime))
	buf.Write(scheduleTime)

	// Validity period (max 17 bytes, NULL terminated)
	validityPeriod := make([]byte, 17)
	copy(validityPeriod, []byte(d.ValidityPeriod))
	buf.Write(validityPeriod)

	buf.WriteByte(d.RegisteredDelivery)
	buf.WriteByte(d.ReplaceIfPresentFlag)
	buf.WriteByte(d.DataCoding)
	buf.WriteByte(d.SMDefaultMsgID)

	// Short message length and data
	buf.WriteByte(d.SMLength)
	buf.Write(d.ShortMessage)

	// Optional parameters
	for _, param := range d.OptionalParams {
		binary.Write(buf, binary.BigEndian, param.Tag)
		binary.Write(buf, binary.BigEndian, param.Length)
		buf.Write(param.Value)
	}

	return buf.Bytes(), nil
}

func (d *DeliverSM) Unmarshal(data []byte) error {
	minLength := MaxServiceTypeLength + 2 + MaxAddressLength + 2 + MaxAddressLength + 9 + 17 + 17
	if len(data) < minLength {
		return fmt.Errorf("insufficient data for deliver_sm")
	}

	offset := 0

	// Service type
	serviceTypeBytes := data[offset : offset+MaxServiceTypeLength]
	d.ServiceType = string(bytes.TrimRight(serviceTypeBytes, "\x00"))
	offset += MaxServiceTypeLength

	// Source address
	d.SourceAddrTON = data[offset]
	offset++
	d.SourceAddrNPI = data[offset]
	offset++
	sourceAddrBytes := data[offset : offset+MaxAddressLength]
	d.SourceAddr = string(bytes.TrimRight(sourceAddrBytes, "\x00"))
	offset += MaxAddressLength

	// Destination address
	d.DestAddrTON = data[offset]
	offset++
	d.DestAddrNPI = data[offset]
	offset++
	destAddrBytes := data[offset : offset+MaxAddressLength]
	d.DestAddr = string(bytes.TrimRight(destAddrBytes, "\x00"))
	offset += MaxAddressLength

	// Other fields
	d.EsmClass = data[offset]
	offset++
	d.ProtocolID = data[offset]
	offset++
	d.PriorityFlag = data[offset]
	offset++

	// Schedule delivery time
	scheduleTimeBytes := data[offset : offset+17]
	d.ScheduleDeliveryTime = string(bytes.TrimRight(scheduleTimeBytes, "\x00"))
	offset += 17

	// Validity period
	validityPeriodBytes := data[offset : offset+17]
	d.ValidityPeriod = string(bytes.TrimRight(validityPeriodBytes, "\x00"))
	offset += 17

	d.RegisteredDelivery = data[offset]
	offset++
	d.ReplaceIfPresentFlag = data[offset]
	offset++
	d.DataCoding = data[offset]
	offset++
	d.SMDefaultMsgID = data[offset]
	offset++

	// Short message
	d.SMLength = data[offset]
	offset++

	if len(data) < offset+int(d.SMLength) {
		return fmt.Errorf("insufficient data for short message")
	}

	d.ShortMessage = make([]byte, d.SMLength)
	copy(d.ShortMessage, data[offset:offset+int(d.SMLength)])
	offset += int(d.SMLength)

	// Parse optional parameters
	d.OptionalParams = []OptionalParameter{}
	for offset < len(data) {
		if len(data) < offset+4 {
			break
		}

		param := OptionalParameter{}
		param.Tag = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
		param.Length = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2

		if len(data) < offset+int(param.Length) {
			break
		}

		param.Value = make([]byte, param.Length)
		copy(param.Value, data[offset:offset+int(param.Length)])
		offset += int(param.Length)

		d.OptionalParams = append(d.OptionalParams, param)
	}

	return nil
}

func (d *DeliverSM) CommandID() uint32 {
	return CommandDeliverSM
}

// DeliverSMResp represents deliver_sm_resp PDU
type DeliverSMResp struct {
	MessageID string
}

func (d *DeliverSMResp) Marshal() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Message ID (NULL terminated)
	buf.WriteString(d.MessageID)
	buf.WriteByte(0)

	return buf.Bytes(), nil
}

func (d *DeliverSMResp) Unmarshal(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("insufficient data for deliver_sm_resp")
	}

	// Find NULL terminator or use entire data
	nullIndex := bytes.IndexByte(data, 0)
	if nullIndex == -1 {
		d.MessageID = string(data)
	} else {
		d.MessageID = string(data[:nullIndex])
	}

	return nil
}

func (d *DeliverSMResp) CommandID() uint32 {
	return CommandDeliverSMResp
}

// EnquireLink represents enquire_link PDU
type EnquireLink struct{}

func (e *EnquireLink) Marshal() ([]byte, error) {
	return []byte{}, nil
}

func (e *EnquireLink) Unmarshal(data []byte) error {
	return nil
}

func (e *EnquireLink) CommandID() uint32 {
	return CommandEnquireLink
}

// EnquireLinkResp represents enquire_link_resp PDU
type EnquireLinkResp struct{}

func (e *EnquireLinkResp) Marshal() ([]byte, error) {
	return []byte{}, nil
}

func (e *EnquireLinkResp) Unmarshal(data []byte) error {
	return nil
}

func (e *EnquireLinkResp) CommandID() uint32 {
	return CommandEnquireLinkResp
}

// Unbind represents unbind PDU
type Unbind struct{}

func (u *Unbind) Marshal() ([]byte, error) {
	return []byte{}, nil
}

func (u *Unbind) Unmarshal(data []byte) error {
	return nil
}

func (u *Unbind) CommandID() uint32 {
	return CommandUnbind
}

// UnbindResp represents unbind_resp PDU
type UnbindResp struct{}

func (u *UnbindResp) Marshal() ([]byte, error) {
	return []byte{}, nil
}

func (u *UnbindResp) Unmarshal(data []byte) error {
	return nil
}

func (u *UnbindResp) CommandID() uint32 {
	return CommandUnbindResp
}

// Session represents an SMPP session
type Session struct {
	ID           string
	SystemID     string
	Password     string
	SystemType   string
	BindType     uint32
	State        SessionState
	LastActivity time.Time
	SequenceNum  uint32
}

// SessionState represents the state of an SMPP session
type SessionState int

const (
	SessionStateOpen SessionState = iota
	SessionStateBoundTX
	SessionStateBoundRX
	SessionStateBoundTRX
	SessionStateUnbound
	SessionStateClosed
)

func (s SessionState) String() string {
	switch s {
	case SessionStateOpen:
		return "OPEN"
	case SessionStateBoundTX:
		return "BOUND_TX"
	case SessionStateBoundRX:
		return "BOUND_RX"
	case SessionStateBoundTRX:
		return "BOUND_TRX"
	case SessionStateUnbound:
		return "UNBOUND"
	case SessionStateClosed:
		return "CLOSED"
	default:
		return "UNKNOWN"
	}
}

// Message represents an SMS message
type Message struct {
	ID                   string
	SessionID            string // Session that submitted this message
	SourceAddr           Address
	DestAddr             Address
	ShortMessage         []byte
	DataCoding           uint8
	EsmClass             uint8
	RegisteredDelivery   uint8
	ValidityPeriod       string
	ScheduleDeliveryTime string
	ServiceType          string
	ProtocolID           uint8
	PriorityFlag         uint8
	Status               MessageStatus
	SubmitTime           time.Time
	DoneTime             *time.Time
	SubDate              string
	DoneDate             string
	Stat                 string
	Err                  string
	Text                 string
}

// MessageStatus represents the status of a message
type MessageStatus int

const (
	MessageStatusSubmitted MessageStatus = iota
	MessageStatusEnroute
	MessageStatusDelivered
	MessageStatusExpired
	MessageStatusDeleted
	MessageStatusUndeliverable
	MessageStatusAccepted
	MessageStatusUnknown
	MessageStatusRejected
	MessageStatusSplit
)

func (m MessageStatus) String() string {
	switch m {
	case MessageStatusSubmitted:
		return "SUBMITTED"
	case MessageStatusEnroute:
		return "ENROUTE"
	case MessageStatusDelivered:
		return "DELIVERED"
	case MessageStatusExpired:
		return "EXPIRED"
	case MessageStatusDeleted:
		return "DELETED"
	case MessageStatusUndeliverable:
		return "UNDELIVERABLE"
	case MessageStatusAccepted:
		return "ACCEPTED"
	case MessageStatusUnknown:
		return "UNKNOWN"
	case MessageStatusRejected:
		return "REJECTED"
	case MessageStatusSplit:
		return "SPLIT"
	default:
		return "UNKNOWN"
	}
}

// DeliveryReceipt represents a delivery receipt
type DeliveryReceipt struct {
	MessageID      string
	SubmitDate     string
	DoneDate       string
	Status         string
	Error          string
	Text           string
	SubmittedParts int
	DeliveredParts int
}
