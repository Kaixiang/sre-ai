package providers

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

const (
    geminiAPIBaseURL     = "https://generativelanguage.googleapis.com/v1beta/models"
    defaultGeminiModelID = "gemini-1.5-flash-latest"
)

// DefaultGeminiModel returns the model id used when no explicit preference is supplied.
func DefaultGeminiModel() string {
    return defaultGeminiModelID
}

type geminiClient struct {
    apiKey     string
    model      string
    httpClient *http.Client
}

// NewGeminiClient creates a client capable of calling the Gemini API.
func NewGeminiClient(apiKey, model string) *geminiClient {
    if model == "" {
        model = defaultGeminiModelID
    }
    return &geminiClient{
        apiKey: apiKey,
        model:  model,
        httpClient: &http.Client{
            Timeout: 60 * time.Second,
        },
    }
}

type geminiRequest struct {
    Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
    Role  string        `json:"role,omitempty"`
    Parts []geminiParts `json:"parts"`
}

type geminiParts struct {
    Text string `json:"text,omitempty"`
}

type geminiResponse struct {
    Candidates []struct {
        Content struct {
            Parts []struct {
                Text string `json:"text,omitempty"`
            } `json:"parts"`
        } `json:"content"`
    } `json:"candidates"`
    PromptFeedback any `json:"promptFeedback,omitempty"`
}

// Generate runs a single prompt against the Gemini generateContent API.
func (c *geminiClient) Generate(ctx context.Context, prompt string) (string, error) {
    payload := geminiRequest{
        Contents: []geminiContent{
            {
                Role: "user",
                Parts: []geminiParts{{Text: prompt}},
            },
        },
    }

    body, err := json.Marshal(payload)
    if err != nil {
        return "", err
    }

    url := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiAPIBaseURL, c.model, c.apiKey)
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
    if err != nil {
        return "", err
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    data, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", err
    }

    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return "", fmt.Errorf("gemini api error: %s", bytes.TrimSpace(data))
    }

    var decoded geminiResponse
    if err := json.Unmarshal(data, &decoded); err != nil {
        return "", err
    }

    if len(decoded.Candidates) == 0 || len(decoded.Candidates[0].Content.Parts) == 0 {
        return "", fmt.Errorf("gemini api returned no candidates")
    }

    return decoded.Candidates[0].Content.Parts[0].Text, nil
}
