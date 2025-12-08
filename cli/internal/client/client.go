package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jonasbg/paste/cli/internal/types"
	"github.com/jonasbg/paste/crypto"
)

// Client represents a paste API client
type Client struct {
	baseURL string
}

// New creates a new paste client
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
	}
}

// BaseURL returns the base URL of the client
func (c *Client) BaseURL() string {
	return c.baseURL
}

// GetConfig fetches server configuration
func (c *Client) GetConfig() (*types.Config, error) {
	resp, err := http.Get(c.baseURL + "/api/config")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var config types.Config
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// FetchMetadata retrieves and decrypts file metadata
func (c *Client) FetchMetadata(fileID string, key []byte) (*types.Metadata, error) {
	token, err := crypto.GenerateHMACToken(fileID, key)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", c.baseURL+"/api/metadata/"+fileID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-HMAC-Token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	decrypted, err := crypto.DecryptMetadata(key, data)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	var metadata types.Metadata
	if err := json.Unmarshal(decrypted, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}
