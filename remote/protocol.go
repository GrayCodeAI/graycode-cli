package remote

import "encoding/json"

// Extended bridge message types for IDE integration.
const (
	BridgeMsgCapacityWake  = "capacity_wake"
	BridgeMsgPermissionReq = "permission_request"
	BridgeMsgPermissionRes = "permission_response"
	BridgeMsgCompactStatus = "compact_status"
	BridgeMsgAttachment    = "attachment"
	BridgeMsgHeartbeat     = "heartbeat"
	BridgeMsgHistory       = "history"
	BridgeMsgControl       = "control"
)

// PermissionRequest is sent from the bridge to the IDE for tool approval.
type PermissionRequest struct {
	RequestID string          `json:"request_id"`
	ToolName  string          `json:"tool_name"`
	ToolID    string          `json:"tool_id"`
	Summary   string          `json:"summary"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// PermissionResponse is the IDE's response to a permission request.
type PermissionResponse struct {
	RequestID      string          `json:"request_id"`
	Allowed        bool            `json:"allowed"`
	ModifiedInputs json.RawMessage `json:"modified_inputs,omitempty"`
}

// CompactStatus notifies the IDE that compaction occurred.
type CompactStatus struct {
	Strategy     string `json:"strategy"`
	TokensBefore int    `json:"tokens_before"`
	TokensAfter  int    `json:"tokens_after"`
}

// Attachment carries file content or images through the bridge.
type Attachment struct {
	Type     string `json:"type"` // "file", "image", "pdf"
	Name     string `json:"name"`
	MimeType string `json:"mime_type,omitempty"`
	Content  string `json:"content"` // base64 for binary
	Size     int    `json:"size"`
}

// ControlRequest is a command sent from the IDE to control the session.
type ControlRequest struct {
	Action  string          `json:"action"` // "interrupt", "compact", "switch_model", "slash_command"
	Payload json.RawMessage `json:"payload,omitempty"`
}

// HeartbeatPayload carries health info in heartbeat messages.
type HeartbeatPayload struct {
	SessionID    string `json:"session_id"`
	MessageCount int    `json:"message_count"`
	TokenCount   int    `json:"token_count"`
	Model        string `json:"model"`
	Uptime       int64  `json:"uptime_seconds"`
}

// BridgeProtocol wraps a transport with message routing.
type BridgeProtocol struct {
	transport Transport
	handlers  map[string]MessageHandler
}

// MessageHandler processes a specific bridge message type.
type MessageHandler func(msg BridgeMessage)

// NewBridgeProtocol creates a protocol handler wrapping a transport.
func NewBridgeProtocol(transport Transport) *BridgeProtocol {
	return &BridgeProtocol{
		transport: transport,
		handlers:  make(map[string]MessageHandler),
	}
}

// OnMessage registers a handler for a message type.
func (bp *BridgeProtocol) OnMessage(msgType string, handler MessageHandler) {
	bp.handlers[msgType] = handler
}

// Listen starts processing incoming messages and routing them to handlers.
func (bp *BridgeProtocol) Listen() {
	for msg := range bp.transport.Receive() {
		if handler, ok := bp.handlers[msg.Type]; ok {
			handler(msg)
		}
	}
}

// SendPermissionRequest sends a permission request to the IDE.
func (bp *BridgeProtocol) SendPermissionRequest(req PermissionRequest) error {
	payload, _ := json.Marshal(req)
	return bp.transport.Send(BridgeMessage{
		Type:    BridgeMsgPermissionReq,
		Payload: payload,
	})
}

// SendHeartbeat sends a heartbeat message.
func (bp *BridgeProtocol) SendHeartbeat(hb HeartbeatPayload) error {
	payload, _ := json.Marshal(hb)
	return bp.transport.Send(BridgeMessage{
		Type:    BridgeMsgHeartbeat,
		Payload: payload,
	})
}

// SendCompactStatus notifies the IDE about a compaction event.
func (bp *BridgeProtocol) SendCompactStatus(status CompactStatus) error {
	payload, _ := json.Marshal(status)
	return bp.transport.Send(BridgeMessage{
		Type:    BridgeMsgCompactStatus,
		Payload: payload,
	})
}
