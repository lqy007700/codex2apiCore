package admin

type unlockPromptConversationRequest struct {
	Reason string `json:"reason"`
	Scope  string `json:"scope"`
}
