package daemon

import "weview/internal/contacts"

const (
	ActionHealth          = "health"
	ActionRefreshContacts = "refresh_contacts"
	ActionListContacts    = "list_contacts"
)

type Request struct {
	Action string `json:"action"`
}

type Response struct {
	OK       bool               `json:"ok"`
	Message  string             `json:"message,omitempty"`
	Contacts []contacts.Contact `json:"contacts,omitempty"`
}
