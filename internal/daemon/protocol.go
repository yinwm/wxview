package daemon

const (
	ActionHealth          = "health"
	ActionRefreshContacts = "refresh_contacts"
)

type Request struct {
	Action string `json:"action"`
}

type Response struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}
