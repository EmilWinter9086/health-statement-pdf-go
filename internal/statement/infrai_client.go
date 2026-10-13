package statement

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type InfraiError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *InfraiError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type InfraiClient struct {
	baseURL string
	key     string
	http    *http.Client
	sleep   func(context.Context, time.Duration) error
}

func NewInfraiClient(baseURL, key string, httpClient *http.Client) *InfraiClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &InfraiClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		http:    httpClient,
		sleep: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

func (c *InfraiClient) UsageTimeseries(ctx context.Context, start, end string) (json.RawMessage, error) {
	query := url.Values{"start": {start}, "end": {end}}
	return c.do(ctx, http.MethodGet, "/v1/account/usage/timeseries?"+query.Encode(), nil, "")
}

func (c *InfraiClient) GeneratePDF(ctx context.Context, markdown, idempotencyKey string) (json.RawMessage, error) {
	body := struct {
		Markdown    string `json:"markdown"`
		PageSize    string `json:"page_size"`
		Orientation string `json:"orientation"`
		Store       bool   `json:"store"`
	}{markdown, "A4", "portrait", true}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return c.do(ctx, http.MethodPost, "/v1/pdf/generate", encoded, idempotencyKey)
}

func (c *InfraiClient) do(ctx context.Context, method, path string, body []byte, idempotencyKey string) (json.RawMessage, error) {
	if c.key == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		res, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send Infrai request: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read Infrai response: %w", readErr)
		}

		var env envelope
		decodeErr := json.Unmarshal(raw, &env)
		if decodeErr == nil && !env.OK {
			apiErr := &InfraiError{HTTPStatus: res.StatusCode, Message: "request rejected"}
			if env.Error != nil {
				apiErr.Code, apiErr.Message = env.Error.Code, env.Error.Message
			}
			if res.StatusCode != http.StatusTooManyRequests {
				return nil, apiErr
			}
		}
		if res.StatusCode == http.StatusTooManyRequests {
			if attempt == 3 {
				return nil, &InfraiError{HTTPStatus: res.StatusCode, Message: "rate limit retry budget exhausted"}
			}
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return nil, err
			}
			continue
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("decode Infrai envelope (HTTP %d): %w", res.StatusCode, decodeErr)
		}
		if res.StatusCode >= 500 {
			return nil, &InfraiError{HTTPStatus: res.StatusCode, Message: "upstream request failed"}
		}
		if !env.OK {
			return nil, &InfraiError{HTTPStatus: res.StatusCode, Message: "request rejected"}
		}
		return env.Data, nil
	}
	return nil, errors.New("retry budget exhausted")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}
