package daemon

const (
	ActionHealth          = "health"
	ActionRefreshContacts = "refresh_contacts"
	ActionRefreshMessages = "refresh_messages"
	ActionStop            = "stop"
)

type Request struct {
	Action string `json:"action"`
}

type Response struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}
