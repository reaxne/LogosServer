package freedompay

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"logosserver/internal/config"
)

type Client struct {
	merchantID     string
	secretKey      string
	initURL        string
	initScript     string
	callbackScript string
	httpClient     *http.Client
}

type InitPaymentRequest struct {
	OrderID     int64
	AmountCents int64
	Currency    string
	Description string
	Email       string
	ResultURL   string
	SuccessURL  string
	FailureURL  string
	Lifetime    time.Duration
}

type initPaymentResponse struct {
	XMLName     xml.Name `xml:"response"`
	Status      string   `xml:"pg_status"`
	PaymentID   string   `xml:"pg_payment_id"`
	RedirectURL string   `xml:"pg_redirect_url"`
	Description string   `xml:"pg_description"`
}

func NewClient(cfg config.Config) Client {
	return Client{
		merchantID:     cfg.FreedomPayMerchantID,
		secretKey:      cfg.FreedomPaySecretKey,
		initURL:        cfg.FreedomPayInitURL,
		initScript:     cfg.FreedomPayInitScript,
		callbackScript: cfg.FreedomPayCallbackScript,
		httpClient:     &http.Client{Timeout: 15 * time.Second},
	}
}

func (c Client) InitPayment(ctx context.Context, req InitPaymentRequest) (string, error) {
	values := map[string]string{
		"pg_merchant_id":  c.merchantID,
		"pg_order_id":     strconv.FormatInt(req.OrderID, 10),
		"pg_amount":       formatAmount(req.AmountCents),
		"pg_currency":     req.Currency,
		"pg_description":  req.Description,
		"pg_result_url":   req.ResultURL,
		"pg_success_url":  req.SuccessURL,
		"pg_failure_url":  req.FailureURL,
		"pg_lifetime":     strconv.Itoa(int(req.Lifetime.Seconds())),
		"pg_salt":         Salt(),
		"pg_testing_mode": "0",
	}
	if req.Email != "" {
		values["pg_user_contact_email"] = req.Email
	}
	values["pg_sig"] = Signature(c.initScript, values, c.secretKey)

	form := url.Values{}
	for key, value := range values {
		form.Set(key, value)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.initURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("freedompay returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var parsed initPaymentResponse
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode freedompay response: %w", err)
	}
	if parsed.Status != "ok" || parsed.RedirectURL == "" {
		if parsed.Description == "" {
			parsed.Description = string(body)
		}
		return "", fmt.Errorf("freedompay init failed: %s", parsed.Description)
	}
	return parsed.RedirectURL, nil
}

func (c Client) ValidCallbackSignature(values map[string]string) bool {
	got := values["pg_sig"]
	if got == "" {
		return false
	}
	return strings.EqualFold(got, Signature(c.callbackScript, values, c.secretKey))
}

func (c Client) ResponseSignature(status, description, salt string) string {
	return Signature(c.callbackScript, map[string]string{
		"pg_status":      status,
		"pg_description": description,
		"pg_salt":        salt,
	}, c.secretKey)
}

func Salt() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(b[:])
}

func Signature(scriptName string, values map[string]string, secretKey string) string {
	keys := make([]string, 0, len(values))
	for key, value := range values {
		if key == "pg_sig" || value == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := []string{scriptName}
	for _, key := range keys {
		parts = append(parts, values[key])
	}
	parts = append(parts, secretKey)

	sum := md5.Sum([]byte(strings.Join(parts, ";")))
	return hex.EncodeToString(sum[:])
}

func formatAmount(cents int64) string {
	whole := cents / 100
	fraction := cents % 100
	if fraction == 0 {
		return strconv.FormatInt(whole, 10)
	}
	return fmt.Sprintf("%d.%02d", whole, fraction)
}
