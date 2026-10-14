package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrReadOnly = errors.New("router writes are disabled by read_only")

// TransportError reports a fixed transport category without the request URL,
// which contains the authentication token on this firmware.
type TransportError struct{ Kind string }

func (e *TransportError) Error() string {
	return "router request failed (" + e.Kind + "); check connectivity and TLS options; a submitted write may have been applied"
}

// APIError intentionally exposes only numeric firmware errors, never response data.
type APIError struct {
	Code     int
	SubCodes []int
}

func (e *APIError) Error() string {
	if len(e.SubCodes) != 0 {
		return fmt.Sprintf("router rejected the request (error_code=%d, sub_error_codes=%v)", e.Code, e.SubCodes)
	}
	return fmt.Sprintf("router rejected the request (error_code=%d)", e.Code)
}

func (e *APIError) AuthenticationExpired() bool {
	return e.Code == -40401 || e.Code == -40402 || e.Code == -40403
}

// Client serializes its session independently of resource-level operations.
// Use WithLock around an entire read-modify-write-save resource operation.
type Client struct {
	config      Config
	endpoint    *url.URL
	http        *http.Client
	sessionLock chan struct{}
	operation   chan struct{}
	stok        string
	role        string
}

func New(config Config) (*Client, error) {
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || endpoint == nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Path != "" && endpoint.Path != "/") {
		return nil, errors.New("endpoint must be an HTTP or HTTPS origin without credentials, path, query, or fragment")
	}
	if config.Username == "" || config.Password == "" {
		return nil, errors.New("router username and password are required")
	}
	if config.Timeout < 0 {
		return nil, errors.New("request timeout must be positive")
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	endpoint.Path = ""
	endpoint.RawPath = ""
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		// Insecure is an explicit provider opt-in for a router's self-signed certificate.
		InsecureSkipVerify: config.Insecure, //nolint:gosec
	}
	if config.AllowLegacyTLS {
		// This firmware requires static RSA TLS 1.2 suites disabled by modern Go.
		// Keep the offer limited to these suites: the device selects an offered
		// ECDHE suite first, then aborts the handshake even though RSA would work.
		tlsConfig.MaxVersion = tls.VersionTLS12
		tlsConfig.CipherSuites = []uint16{tls.TLS_RSA_WITH_AES_256_GCM_SHA384, tls.TLS_RSA_WITH_AES_128_GCM_SHA256}
	}
	transport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		TLSHandshakeTimeout:   config.Timeout,
		ResponseHeaderTimeout: config.Timeout,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       90 * time.Second,
	}
	jar, _ := cookiejar.New(nil)
	client := &Client{config: config, endpoint: endpoint, sessionLock: make(chan struct{}, 1), operation: make(chan struct{}, 1)}
	client.http = &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
		Jar:       jar,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if request.URL.Scheme != endpoint.Scheme || !strings.EqualFold(request.URL.Host, endpoint.Host) {
				return errors.New("router redirect to a different origin was rejected")
			}
			if len(via) >= 3 {
				return errors.New("too many router redirects")
			}
			return nil
		},
	}
	return client, nil
}

func (c *Client) Endpoint() string { return c.endpoint.String() }

// Credentials returns the current credentials for an authenticated password-change
// payload. Callers must never log these values or store them in Terraform state.
func (c *Client) Credentials() (username, password string) {
	c.sessionLock <- struct{}{}
	defer func() { <-c.sessionLock }()
	return c.config.Username, c.config.Password
}

// Close releases idle transports without making a network request.
func (c *Client) Close() { c.http.CloseIdleConnections() }

func acquire(ctx context.Context, semaphore chan struct{}) (func(), error) {
	select {
	case semaphore <- struct{}{}:
		return func() { <-semaphore }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// WithLock serializes a complete resource transaction; callbacks must not nest it.
func (c *Client) WithLock(ctx context.Context, operation func() error) error {
	release, err := acquire(ctx, c.operation)
	if err != nil {
		return err
	}
	defer release()
	return operation()
}

func (c *Client) Login(ctx context.Context) error {
	release, err := acquire(ctx, c.sessionLock)
	if err != nil {
		return err
	}
	defer release()
	if c.stok != "" {
		return nil
	}
	return c.login(ctx, c.config.Username, c.config.Password)
}

func (c *Client) login(ctx context.Context, username, password string) error {
	response, err := c.post(ctx, "/", map[string]any{"method": "do", "login": map[string]any{"username": username, "password": EncodePassword(password)}})
	if err != nil {
		return fmt.Errorf("router login failed: %w", err)
	}
	token, ok := response["stok"].(string)
	if !ok || token == "" || strings.ContainsAny(token, "/?#%\\\r\n") {
		return errors.New("router login did not return a valid session token")
	}
	c.stok = token
	c.role, _ = response["role"].(string)
	return nil
}

// RotateCredentials verifies already-applied credentials and replaces the live session.
// The caller performs the credential change first under WithLock; this method does
// not write router settings or modify a credentials file.
func (c *Client) RotateCredentials(ctx context.Context, username, password string) error {
	if username == "" || password == "" {
		return errors.New("new router credentials must not be empty")
	}
	release, err := acquire(ctx, c.sessionLock)
	if err != nil {
		return err
	}
	defer release()
	if err := c.login(ctx, username, password); err != nil {
		return err
	}
	c.config.Username, c.config.Password = username, password
	return nil
}

// Logout ends only this client's session. It is allowed in read-only mode.
func (c *Client) Logout(ctx context.Context) error {
	release, err := acquire(ctx, c.sessionLock)
	if err != nil {
		return err
	}
	defer release()
	if c.stok == "" {
		return nil
	}
	_, err = c.post(ctx, "/stok="+c.stok+"/ds", map[string]any{"method": "do", "system": map[string]any{"logout": nil}})
	c.stok, c.role = "", ""
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.AuthenticationExpired() {
		return nil
	}
	return err
}

// Query accepts only SLP get requests, adding method=get when omitted.
func (c *Client) Query(ctx context.Context, payload map[string]any) (map[string]any, error) {
	request := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		request[key] = value
	}
	if method, exists := request["method"]; exists && method != "get" {
		return nil, errors.New("Query accepts only method=get")
	}
	request["method"] = "get"
	return c.request(ctx, request)
}

// Mutate does not retry ambiguous network failures. Only explicit pre-execution
// authentication rejection permits one login and replay.
func (c *Client) Mutate(ctx context.Context, payload map[string]any) (map[string]any, error) {
	if c.config.ReadOnly {
		return nil, ErrReadOnly
	}
	method, _ := payload["method"].(string)
	if method != "set" && method != "add" && method != "delete" && method != "do" {
		return nil, errors.New("Mutate requires method=set, add, delete, or do")
	}
	return c.request(ctx, payload)
}

func (c *Client) request(ctx context.Context, payload map[string]any) (map[string]any, error) {
	release, err := acquire(ctx, c.sessionLock)
	if err != nil {
		return nil, err
	}
	defer release()
	if c.stok == "" {
		if err := c.login(ctx, c.config.Username, c.config.Password); err != nil {
			return nil, err
		}
	}
	result, err := c.post(ctx, "/stok="+c.stok+"/ds", payload)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.AuthenticationExpired() {
		c.stok = ""
		if err := c.login(ctx, c.config.Username, c.config.Password); err != nil {
			return nil, err
		}
		return c.post(ctx, "/stok="+c.stok+"/ds", payload)
	}
	return result, err
}

func (c *Client) post(ctx context.Context, path string, payload map[string]any) (map[string]any, error) {
	// Round-trip first so typed slices/maps receive the same recursive treatment.
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("unable to encode the router request")
	}
	var normalized any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&normalized); err != nil {
		return nil, errors.New("unable to normalize the router request")
	}
	body, err := json.Marshal(transformStrings(normalized, true))
	if err != nil {
		return nil, errors.New("unable to encode the router request")
	}
	target := *c.endpoint
	target.Path = path
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("unable to construct the router request")
	}
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Referer", c.endpoint.String()+"/")
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		kind := "network"
		var networkError net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkError) && networkError.Timeout() {
			kind = "timeout"
		} else if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			kind = "unexpected_eof"
		}
		// net/http errors may contain /stok=<secret>/ds or certificate details.
		return nil, &TransportError{Kind: kind}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("router returned HTTP status %d", response.StatusCode)
	}
	const maximumResponseBytes = 16 << 20
	content, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		kind := "response_read"
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			kind = "unexpected_eof"
		}
		return nil, &TransportError{Kind: kind}
	}
	if len(content) > maximumResponseBytes {
		return nil, errors.New("router response exceeds the supported 16 MiB limit")
	}
	var result map[string]any
	decoder = json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, errors.New("router returned an invalid JSON response")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("router returned trailing content after its JSON response")
	}
	result, _ = transformStrings(result, false).(map[string]any)
	code, ok := integer(result["error_code"])
	if !ok {
		return nil, errors.New("router response is missing a valid error_code")
	}
	if code != 0 {
		apiErr := &APIError{Code: code}
		if details, ok := result["error_msg"].(map[string]any); ok {
			if codes, ok := details["sub_err_code"].([]any); ok {
				for _, value := range codes {
					if subCode, valid := integer(value); valid {
						apiErr.SubCodes = append(apiErr.SubCodes, subCode)
					}
				}
			} else if subCode, valid := integer(details["sub_err_code"]); valid {
				apiErr.SubCodes = []int{subCode}
			}
		}
		return nil, apiErr
	}
	return result, nil
}

func integer(value any) (int, bool) {
	switch typed := value.(type) {
	case json.Number:
		result, err := strconv.Atoi(string(typed))
		return result, err == nil
	case string:
		result, err := strconv.Atoi(typed)
		return result, err == nil
	case int:
		return typed, true
	case float64:
		return int(typed), typed == float64(int(typed))
	default:
		return 0, false
	}
}
