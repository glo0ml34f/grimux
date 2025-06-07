package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

var apiURL = "https://api.openai.com/v1/chat/completions"

// Client interacts with the OpenAI API.
type Client struct {
	APIKey     string
	HTTPClient *http.Client
}

// NewClient creates a client using the OPENAI_API_KEY environment variable.
func NewClient() (*Client, error) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}
	return &Client{APIKey: key, HTTPClient: http.DefaultClient}, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// SendPrompt sends the given text as a user message and returns the assistant's reply.
func (c *Client) SendPrompt(prompt string) (string, error) {
	if env := os.Getenv("OPENAI_API_URL"); env != "" {
		apiURL = env
	}
	reqBody := chatRequest{
		Model:    "gpt-4o",
		Messages: []chatMessage{{Role: "user", Content: prompt}},
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai: unexpected status %s", resp.Status)
	}
	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", err
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("openai: no choices in response")
	}
	return cr.Choices[0].Message.Content, nil
}
