package gemini

import (
	"context"
	"os"
	"testing"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

func TestGeminiAPI(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API")
	if apiKey == "" {
		t.Skip("GEMINI_API environment variable not set, skipping test")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		t.Fatalf("Failed to create genai client: %v", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-1.5-flash")
	prompt := "Where is Kyoto located?"
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		t.Fatalf("Failed to generate content: %v", err)
	}

	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				t.Logf("Response: %v\n", part)
			}
		}
	}
}
