package admin

const qualityTestPromptNameLimit = 100

type qualityTestPromptPayload struct {
	Name   *string `json:"name"`
	Prompt *string `json:"prompt"`
}

// Presets keep the studio's validation: a non-empty prompt within the request
// limit. A blank name falls back to the prompt's opening characters.
