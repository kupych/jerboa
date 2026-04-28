// Package replicate is a minimal HTTP client for the Replicate API.
// Covers what we need: upload a file, run a model, poll to completion, download outputs.
package replicate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

const apiBase = "https://api.replicate.com/v1"

type Client struct {
	token string
	http  *http.Client
}

func New(token string) *Client {
	return &Client{
		token: token,
		http:  &http.Client{Timeout: 0}, // long-lived; per-request timeouts come from ctx
	}
}

type Prediction struct {
	ID     string `json:"id"`
	Status string `json:"status"` // starting, processing, succeeded, failed, canceled
	Output any    `json:"output"`
	Error  string `json:"error,omitempty"`
	URLs   struct {
		Get string `json:"get"`
	} `json:"urls"`
}

type fileUploadResp struct {
	ID   string `json:"id"`
	URLs struct {
		Get string `json:"get"`
	} `json:"urls"`
}

// UploadFile sends a file to Replicate's Files API and returns a URL the
// prediction worker can fetch. The returned URL is signed and short-lived.
func (c *Client) UploadFile(ctx context.Context, filename, contentType string, body io.Reader) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="content"; filename=%q`, filename)}
	if contentType != "" {
		h["Content-Type"] = []string{contentType}
	}
	part, err := mw.CreatePart(h)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, body); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiBase+"/files", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("replicate upload: %s: %s", resp.Status, string(b))
	}
	var out fileUploadResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.URLs.Get == "" {
		return "", fmt.Errorf("replicate upload: empty URL in response")
	}
	return out.URLs.Get, nil
}

// RunModel creates a prediction against the given model (e.g. "owner/name")
// and polls until it reaches a terminal state. Returns the final Prediction.
func (c *Client) RunModel(ctx context.Context, model string, input map[string]any) (*Prediction, error) {
	body, err := json.Marshal(map[string]any{"input": input})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", apiBase+"/models/"+model+"/predictions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "wait=5") // ask server to hold the connection a few seconds before returning

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("replicate run: %s: %s", resp.Status, string(b))
	}

	var p Prediction
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}

	for !terminal(p.Status) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
		next, err := c.getPrediction(ctx, p.URLs.Get)
		if err != nil {
			return nil, err
		}
		p = *next
	}
	if p.Status != "succeeded" {
		return &p, fmt.Errorf("prediction %s: %s", p.Status, p.Error)
	}
	return &p, nil
}

func (c *Client) getPrediction(ctx context.Context, url string) (*Prediction, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("replicate get: %s: %s", resp.Status, string(b))
	}
	var p Prediction
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Download fetches a prediction output URL into dst.
func (c *Client) Download(ctx context.Context, url string, dst io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("replicate download: %s: %s", resp.Status, string(b))
	}
	_, err = io.Copy(dst, resp.Body)
	return err
}

func terminal(s string) bool {
	return s == "succeeded" || s == "failed" || s == "canceled"
}
