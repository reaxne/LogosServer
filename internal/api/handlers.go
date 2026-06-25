package api

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"logosserver/internal/db"
	"logosserver/internal/freedompay"
)

type createOrderRequest struct {
	UserID  string `json:"user_id"`
	VideoID string `json:"video_id"`
	Email   string `json:"email"`
}

type createOrderResponse struct {
	OrderID    int64  `json:"order_id"`
	Status     string `json:"status"`
	PaymentURL string `json:"payment_url"`
}

type upsertVideoRequest struct {
	ID                  string `json:"id"`
	Title               string `json:"title"`
	PriceCents          int64  `json:"price_cents"`
	CloudflareStreamUID string `json:"cloudflare_stream_uid"`
}

type videoAccessResponse struct {
	Unlocked     bool   `json:"unlocked"`
	VideoID      string `json:"video_id"`
	Title        string `json:"title,omitempty"`
	PlaybackURL  string `json:"playback_url,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
}

func (s Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s Server) ready(c *gin.Context) {
	databaseReady := s.store != nil && s.store.Ping(c.Request.Context()) == nil
	paymentReady := s.cfg.FreedomPayConfigured()
	status := http.StatusOK
	if !databaseReady || !paymentReady {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{
		"status":           readinessStatus(databaseReady && paymentReady),
		"database_ready":   databaseReady,
		"freedompay_ready": paymentReady,
	})
}

func (s Server) createOrder(c *gin.Context) {
	if !s.requireStore(c) || !s.requireFreedomPay(c) {
		return
	}
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.UserID = strings.TrimSpace(req.UserID)
	req.VideoID = strings.TrimSpace(req.VideoID)
	if req.UserID == "" || req.VideoID == "" {
		writeError(c, http.StatusBadRequest, "user_id and video_id are required")
		return
	}

	video, err := s.store.GetVideo(c.Request.Context(), req.VideoID)
	if errors.Is(err, db.ErrNotFound) {
		writeError(c, http.StatusNotFound, "video not found")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "could not load video")
		return
	}

	order, err := s.store.CreateOrder(c.Request.Context(), db.CreateOrderParams{
		UserID:     req.UserID,
		VideoID:    req.VideoID,
		Amount:     video.PriceCents,
		Currency:   s.cfg.Currency,
		CustomerID: req.Email,
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "could not create order")
		return
	}

	paymentURL, err := s.freedomPay.InitPayment(c.Request.Context(), freedompay.InitPaymentRequest{
		OrderID:     order.ID,
		AmountCents: order.Amount,
		Currency:    order.Currency,
		Description: fmt.Sprintf("Access to %s", video.Title),
		Email:       req.Email,
		ResultURL:   s.publicURL(c) + "/api/payments/freedompay/callback",
		SuccessURL:  s.paymentReturnURL(c, s.cfg.SuccessURL, "/payment/success", order.ID),
		FailureURL:  s.paymentReturnURL(c, s.cfg.FailureURL, "/payment/failure", order.ID),
		Lifetime:    s.cfg.PaymentLifetime,
	})
	if err != nil {
		writeError(c, http.StatusBadGateway, "could not initialize payment")
		return
	}

	c.JSON(http.StatusCreated, createOrderResponse{
		OrderID:    order.ID,
		Status:     order.Status,
		PaymentURL: paymentURL,
	})
}

func (s Server) freedomPayCallback(c *gin.Context) {
	if !s.requireStore(c) || !s.requireFreedomPayXML(c) {
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		s.writeFreedomPayXML(c, "error", "invalid form")
		return
	}

	values := make(map[string]string, len(c.Request.Form))
	for key := range c.Request.Form {
		values[key] = c.Request.Form.Get(key)
	}
	if !s.freedomPay.ValidCallbackSignature(values) {
		s.writeFreedomPayXML(c, "error", "invalid signature")
		return
	}

	callback, err := freedompay.ParseCallback(values)
	if err != nil {
		s.writeFreedomPayXML(c, "error", err.Error())
		return
	}

	order, err := s.store.GetOrder(c.Request.Context(), callback.OrderID)
	if errors.Is(err, db.ErrNotFound) {
		s.writeFreedomPayXML(c, "rejected", "order not found")
		return
	}
	if err != nil {
		s.writeFreedomPayXML(c, "error", "could not load order")
		return
	}

	if order.Amount != callback.AmountCents || order.Currency != callback.Currency {
		s.writeFreedomPayXML(c, "rejected", "amount or currency mismatch")
		return
	}

	if callback.Paid {
		if err := s.store.MarkOrderPaid(c.Request.Context(), callback.OrderID, callback.PaymentID); err != nil {
			s.writeFreedomPayXML(c, "error", "could not unlock video")
			return
		}
	} else if callback.Failed {
		_ = s.store.MarkOrderFailed(c.Request.Context(), callback.OrderID, callback.PaymentID)
	}

	s.writeFreedomPayXML(c, "ok", "processed")
}

func (s Server) videoAccess(c *gin.Context) {
	if !s.requireStore(c) {
		return
	}
	videoID := strings.TrimSpace(c.Param("video_id"))
	userID := strings.TrimSpace(c.Query("user_id"))
	if userID == "" {
		writeError(c, http.StatusBadRequest, "user_id is required")
		return
	}

	video, err := s.store.GetVideo(c.Request.Context(), videoID)
	if errors.Is(err, db.ErrNotFound) {
		writeError(c, http.StatusNotFound, "video not found")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "could not load video")
		return
	}

	unlocked, err := s.store.UserHasAccess(c.Request.Context(), userID, videoID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "could not check access")
		return
	}
	resp := videoAccessResponse{
		Unlocked: unlocked,
		VideoID:  video.ID,
		Title:    video.Title,
	}
	if unlocked {
		playbackURL, err := s.stream.PlaybackURL(video.CloudflareStreamUID, time.Now().Add(s.cfg.PlaybackTokenLifetime))
		if err != nil {
			writeError(c, http.StatusInternalServerError, "could not create playback URL")
			return
		}
		resp.PlaybackURL = playbackURL
		resp.ThumbnailURL = s.stream.ThumbnailURL(video.CloudflareStreamUID)
	}
	c.JSON(http.StatusOK, resp)
}

func (s Server) upsertVideo(c *gin.Context) {
	if !s.requireStore(c) {
		return
	}
	if s.cfg.AdminToken == "" || c.GetHeader("Authorization") != "Bearer "+s.cfg.AdminToken {
		writeError(c, http.StatusUnauthorized, "missing or invalid admin token")
		return
	}
	var req upsertVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	req.Title = strings.TrimSpace(req.Title)
	req.CloudflareStreamUID = strings.TrimSpace(req.CloudflareStreamUID)
	if req.ID == "" || req.Title == "" || req.PriceCents <= 0 || req.CloudflareStreamUID == "" {
		writeError(c, http.StatusBadRequest, "id, title, price_cents, and cloudflare_stream_uid are required")
		return
	}
	video, err := s.store.UpsertVideo(c.Request.Context(), db.Video{
		ID:                  req.ID,
		Title:               req.Title,
		PriceCents:          req.PriceCents,
		CloudflareStreamUID: req.CloudflareStreamUID,
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "could not save video")
		return
	}
	c.JSON(http.StatusOK, video)
}

func writeError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

func (s Server) requireStore(c *gin.Context) bool {
	if s.store != nil {
		return true
	}
	writeError(c, http.StatusServiceUnavailable, "database is not configured or unavailable")
	return false
}

func (s Server) requireFreedomPay(c *gin.Context) bool {
	if s.cfg.FreedomPayConfigured() {
		return true
	}
	writeError(c, http.StatusServiceUnavailable, "Freedom Pay is not configured")
	return false
}

func (s Server) requireFreedomPayXML(c *gin.Context) bool {
	if s.cfg.FreedomPayConfigured() {
		return true
	}
	s.writeFreedomPayXML(c, "error", "Freedom Pay is not configured")
	return false
}

func readinessStatus(ready bool) string {
	if ready {
		return "ready"
	}
	return "not_ready"
}

func (s Server) publicURL(c *gin.Context) string {
	if s.cfg.PublicURL != "" {
		return s.cfg.PublicURL
	}
	proto := c.GetHeader("X-Forwarded-Proto")
	if proto == "" {
		proto = "https"
	}
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}
	return strings.TrimRight(proto+"://"+host, "/")
}

func (s Server) paymentReturnURL(c *gin.Context, configuredURL, fallbackPath string, orderID int64) string {
	rawURL := strings.TrimSpace(configuredURL)
	if rawURL == "" {
		rawURL = s.publicURL(c) + fallbackPath
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	query.Set("order_id", strconv.FormatInt(orderID, 10))
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

type freedomPayResponse struct {
	XMLName     xml.Name `xml:"response"`
	Salt        string   `xml:"pg_salt"`
	Result      string   `xml:"pg_status"`
	Description string   `xml:"pg_description"`
	Signature   string   `xml:"pg_sig"`
}

func (s Server) writeFreedomPayXML(c *gin.Context, status, description string) {
	salt := freedompay.Salt()
	c.Header("Content-Type", "application/xml")
	c.XML(http.StatusOK, freedomPayResponse{
		Salt:        salt,
		Result:      status,
		Description: description,
		Signature:   s.freedomPay.ResponseSignature(status, description, salt),
	})
}
