package session

type InputProvenance string

const (
	ProvenanceExternalUser   InputProvenance = "external_user"
	ProvenanceInterSession   InputProvenance = "inter_session"
	ProvenanceInternalSystem InputProvenance = "internal_system"
	ProvenanceCron           InputProvenance = "cron"
	ProvenanceWebhook        InputProvenance = "webhook"
)

type ProvenanceTag struct {
	Source    InputProvenance `json:"source"`
	SessionID string         `json:"session_id,omitempty"`
	Channel   string         `json:"channel,omitempty"`
	Trusted   bool           `json:"trusted"`
}

func NewUserProvenance() ProvenanceTag {
	return ProvenanceTag{Source: ProvenanceExternalUser, Trusted: true}
}

func NewSystemProvenance() ProvenanceTag {
	return ProvenanceTag{Source: ProvenanceInternalSystem, Trusted: true}
}

func NewInterSessionProvenance(fromSession string) ProvenanceTag {
	return ProvenanceTag{Source: ProvenanceInterSession, SessionID: fromSession, Trusted: true}
}

func NewCronProvenance() ProvenanceTag {
	return ProvenanceTag{Source: ProvenanceCron, Trusted: true}
}

func NewWebhookProvenance(channel string) ProvenanceTag {
	return ProvenanceTag{Source: ProvenanceWebhook, Channel: channel, Trusted: false}
}

func (p ProvenanceTag) RequiresSecurityWrap() bool {
	return !p.Trusted
}
