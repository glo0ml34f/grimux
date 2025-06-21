package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/glo0ml34f/grimux/internal/input"
	"github.com/glo0ml34f/grimux/internal/plugin"
)

const defaultAPIURL = "https://api.openai.com/v1/chat/completions"

// ModelName controls which OpenAI model is used for requests.
var ModelName = "gpt-4o"

var sessionAPIURL string

var sessionAPIKey string

// SetSessionAPIKey stores the API key loaded from the session file.
func SetSessionAPIKey(k string) { sessionAPIKey = k }

// SetSessionAPIURL stores the API URL loaded from the session file.
func SetSessionAPIURL(u string) { sessionAPIURL = u }

// GetSessionAPIKey returns the API key saved in the current session.
func GetSessionAPIKey() string { return sessionAPIKey }

// GetSessionAPIURL returns the API URL saved in the current session.
func GetSessionAPIURL() string { return sessionAPIURL }

// SetModelName sets the OpenAI model name used by SendPrompt.
func SetModelName(n string) { ModelName = n }

// GetModelName returns the current OpenAI model name.
func GetModelName() string { return ModelName }

// Client interacts with the OpenAI API.
type Client struct {
	APIKey     string
	APIURL     string
	HTTPClient *http.Client
}

// NewClient creates a client using the OPENAI_API_KEY environment variable.
func NewClient() (*Client, error) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		key = sessionAPIKey
	}
	if key == "" {
		line, err := input.ReadPasswordPrompt("OpenAI API key: ")
		if err != nil {
			return nil, err
		}
		key = strings.TrimSpace(line)
		sessionAPIKey = key
	}
	if key == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}

	url := os.Getenv("OPENAI_API_URL")
	if url == "" {
		url = sessionAPIURL
	}
	if url == "" {
		line, err := input.ReadLinePrompt(fmt.Sprintf("OpenAI API URL [%s]: ", defaultAPIURL))
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			url = defaultAPIURL
		} else {
			url = line
		}
		sessionAPIURL = url
	}
	if url == "" {
		url = defaultAPIURL
	}

	return &Client{APIKey: key, APIURL: url, HTTPClient: http.DefaultClient}, nil
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
	prompt = plugin.GetManager().RunHook("before_openai", "", prompt)
	reqBody := chatRequest{
		Model:    ModelName,
		Messages: []chatMessage{{Role: "user", Content: prompt}},
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	url := c.APIURL
	if url == "" {
		url = defaultAPIURL
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
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
	reply := cr.Choices[0].Message.Content
	reply = plugin.GetManager().RunHook("after_openai", "", reply)
	return reply, nil
}
