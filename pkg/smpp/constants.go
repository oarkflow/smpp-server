package smpp

// SMPP Protocol Version
const SMPPVersion = 0x34

// Command IDs
const (
	CommandBindReceiver      uint32 = 0x00000001
	CommandBindTransmitter   uint32 = 0x00000002
	CommandQuerySM           uint32 = 0x00000003
	CommandSubmitSM          uint32 = 0x00000004
	CommandDeliverSM         uint32 = 0x00000005
	CommandUnbind            uint32 = 0x00000006
	CommandReplaceSM         uint32 = 0x00000007
	CommandCancelSM          uint32 = 0x00000008
	CommandBindTransceiver   uint32 = 0x00000009
	CommandOutbind           uint32 = 0x0000000B
	CommandEnquireLink       uint32 = 0x00000015
	CommandSubmitMulti       uint32 = 0x00000021
	CommandAlertNotification uint32 = 0x00000102
	CommandDataSM            uint32 = 0x00000103
	CommandBroadcastSM       uint32 = 0x00000111
	CommandQueryBroadcastSM  uint32 = 0x00000112
	CommandCancelBroadcastSM uint32 = 0x00000113

	// Response command IDs (original command ID | 0x80000000)
	CommandBindReceiverResp      uint32 = 0x80000001
	CommandBindTransmitterResp   uint32 = 0x80000002
	CommandQuerySMResp           uint32 = 0x80000003
	CommandSubmitSMResp          uint32 = 0x80000004
	CommandDeliverSMResp         uint32 = 0x80000005
	CommandUnbindResp            uint32 = 0x80000006
	CommandReplaceSMResp         uint32 = 0x80000007
	CommandCancelSMResp          uint32 = 0x80000008
	CommandBindTransceiverResp   uint32 = 0x80000009
	CommandEnquireLinkResp       uint32 = 0x80000015
	CommandSubmitMultiResp       uint32 = 0x80000021
	CommandDataSMResp            uint32 = 0x80000103
	CommandBroadcastSMResp       uint32 = 0x80000111
	CommandQueryBroadcastSMResp  uint32 = 0x80000112
	CommandCancelBroadcastSMResp uint32 = 0x80000113
)

// Command Status
const (
	StatusOK              uint32 = 0x00000000
	StatusInvMsgLen       uint32 = 0x00000001
	StatusInvCmdLen       uint32 = 0x00000002
	StatusInvCmdID        uint32 = 0x00000003
	StatusInvBnd          uint32 = 0x00000004
	StatusAlreadyBnd      uint32 = 0x00000005
	StatusInvPrtFlg       uint32 = 0x00000006
	StatusInvRegDlvFlg    uint32 = 0x00000007
	StatusSysErr          uint32 = 0x00000008
	StatusInvSrcAdr       uint32 = 0x0000000A
	StatusInvDstAdr       uint32 = 0x0000000B
	StatusInvMsgID        uint32 = 0x0000000C
	StatusBindFail        uint32 = 0x0000000D
	StatusInvPaswd        uint32 = 0x0000000E
	StatusInvSysID        uint32 = 0x0000000F
	StatusCancelFail      uint32 = 0x00000011
	StatusReplaceFail     uint32 = 0x00000013
	StatusMsgQFul         uint32 = 0x00000014
	StatusInvSerTyp       uint32 = 0x00000015
	StatusInvNumDests     uint32 = 0x00000033
	StatusInvDLName       uint32 = 0x00000034
	StatusInvDestFlag     uint32 = 0x00000040
	StatusInvSubRep       uint32 = 0x00000042
	StatusInvEsmClass     uint32 = 0x00000043
	StatusCntSubDL        uint32 = 0x00000044
	StatusSubmitFail      uint32 = 0x00000045
	StatusInvSrcTON       uint32 = 0x00000048
	StatusInvSrcNPI       uint32 = 0x00000049
	StatusInvDstTON       uint32 = 0x00000050
	StatusInvDstNPI       uint32 = 0x00000051
	StatusInvSysTyp       uint32 = 0x00000053
	StatusInvRepFlag      uint32 = 0x00000054
	StatusInvNumMsgs      uint32 = 0x00000055
	StatusThrottled       uint32 = 0x00000058
	StatusInvSched        uint32 = 0x00000061
	StatusInvExpiry       uint32 = 0x00000062
	StatusInvDftMsgID     uint32 = 0x00000063
	StatusXTAppn          uint32 = 0x00000064
	StatusXPAppn          uint32 = 0x00000065
	StatusXRAppn          uint32 = 0x00000066
	StatusQueryFail       uint32 = 0x00000067
	StatusInvOptParStream uint32 = 0x000000C0
	StatusOptParNotAllwd  uint32 = 0x000000C1
	StatusInvParLen       uint32 = 0x000000C2
	StatusMissingOptParam uint32 = 0x000000C3
	StatusInvOptParamVal  uint32 = 0x000000C4
	StatusDeliveryFailure uint32 = 0x000000FE
	StatusUnknownErr      uint32 = 0x000000FF
)

// ESM Class values
const (
	EsmClassDefault      = 0x00
	EsmClassDatagramMode = 0x01
	EsmClassForwardMode  = 0x02
	EsmClassStoreForward = 0x03
	EsmClassUDHI         = 0x40
	EsmClassReplyPath    = 0x80
)

// Data Coding Scheme
const (
	DataCodingDefault     = 0x00
	DataCodingIA5         = 0x01
	DataCodingBinary      = 0x02
	DataCodingISO88591    = 0x03
	DataCodingISO88595    = 0x06
	DataCodingISO88598    = 0x07
	DataCodingUCS2        = 0x08
	DataCodingPictogram   = 0x09
	DataCodingISO2022JP   = 0x0A
	DataCodingExtJISX0212 = 0x0D
	DataCodingKSC5601     = 0x0E
)

// TON (Type of Number)
const (
	TONUnknown          = 0x00
	TONInternational    = 0x01
	TONNational         = 0x02
	TONNetworkSpecific  = 0x03
	TONSubscriberNumber = 0x04
	TONAlphanumeric     = 0x05
	TONAbbreviated      = 0x06
)

// NPI (Numbering Plan Indicator)
const (
	NPIUnknown    = 0x00
	NPIISDN       = 0x01
	NPIData       = 0x03
	NPITelex      = 0x04
	NPILandMobile = 0x06
	NPINational   = 0x08
	NPIPrivate    = 0x09
	NPIERMES      = 0x0A
	NPIIP         = 0x0E
	NPIWAP        = 0x12
)

// Registered Delivery
const (
	RegisteredDeliveryNone           = 0x00
	RegisteredDeliverySuccessFailure = 0x01
	RegisteredDeliveryFailure        = 0x02
	RegisteredDeliverySuccess        = 0x03
	RegisteredDeliveryIntermediate   = 0x10
)

// Priority Flag
const (
	PriorityLevel0 = 0x00 // Normal
	PriorityLevel1 = 0x01 // High
	PriorityLevel2 = 0x02 // Very High
	PriorityLevel3 = 0x03 // Highest
)

// Message State (for delivery receipts)
const (
	MessageStateEnroute       = 0x01
	MessageStateDelivered     = 0x02
	MessageStateExpired       = 0x03
	MessageStateDeleted       = 0x04
	MessageStateUndeliverable = 0x05
	MessageStateAccepted      = 0x06
	MessageStateUnknown       = 0x07
	MessageStateRejected      = 0x08
	MessageStateSkipped       = 0x09
)

// Optional Parameter Tags
const (
	TagDestAddrSubunit          = 0x0005
	TagDestNetworkType          = 0x0006
	TagDestBearerType           = 0x0007
	TagDestTelematicsID         = 0x0008
	TagSourceAddrSubunit        = 0x000D
	TagSourceNetworkType        = 0x000E
	TagSourceBearerType         = 0x000F
	TagSourceTelematicsID       = 0x0010
	TagQOSTimeToLive            = 0x0017
	TagPayloadType              = 0x0019
	TagAdditionalStatusInfoText = 0x001D
	TagReceiptedMessageID       = 0x001E
	TagMsMsgWaitFacilities      = 0x0030
	TagPrivacyIndicator         = 0x0201
	TagSourceSubaddress         = 0x0202
	TagDestSubaddress           = 0x0203
	TagUserMessageReference     = 0x0204
	TagUserResponseCode         = 0x0205
	TagSourcePort               = 0x020A
	TagDestinationPort          = 0x020B
	TagSarMsgRefNum             = 0x020C
	TagLanguageIndicator        = 0x020D
	TagSarTotalSegments         = 0x020E
	TagSarSegmentSeqnum         = 0x020F
	TagSCInterfaceVersion       = 0x0210
	TagCallbackNumPresInd       = 0x0302
	TagCallbackNumAtag          = 0x0303
	TagNumberOfMessages         = 0x0304
	TagCallbackNum              = 0x0381
	TagDpfResult                = 0x0420
	TagSetDpf                   = 0x0421
	TagMsAvailabilityStatus     = 0x0422
	TagNetworkErrorCode         = 0x0423
	TagMessagePayload           = 0x0424
	TagDeliveryFailureReason    = 0x0425
	TagMoreMessagesToSend       = 0x0426
	TagMessageStateOption       = 0x0427
	TagUssdServiceOp            = 0x0501
	TagDisplayTime              = 0x1201
	TagSmsSignal                = 0x1203
	TagMsValidity               = 0x1204
	TagAlertOnMessageDelivery   = 0x130C
	TagItsReplyType             = 0x1380
	TagItsSessionInfo           = 0x1383
)

// Maximum field lengths
const (
	MaxSystemIDLength     = 16
	MaxPasswordLength     = 9
	MaxSystemTypeLength   = 13
	MaxServiceTypeLength  = 6
	MaxAddressLength      = 21
	MaxShortMessageLength = 254
	MaxMessageIDLength    = 64
)
