package domain

type LLMRequest struct {
	Prompt string `json:"prompt" binding:"required"`
}

type LLMResponse struct {
	Text string `json:"text"`
}
