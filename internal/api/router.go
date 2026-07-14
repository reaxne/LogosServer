package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"logosserver/internal/bunny"
	"logosserver/internal/config"
	"logosserver/internal/db"
	"logosserver/internal/freedompay"
)

type Server struct {
	cfg        config.Config
	store      *db.Store
	freedomPay freedompay.Client
	stream     bunny.StreamSigner
}

func NewRouter(cfg config.Config, store *db.Store) (*gin.Engine, error) {
	stream, err := bunny.NewStreamSigner(cfg)
	if err != nil {
		return nil, err
	}
	s := Server{
		cfg:        cfg,
		store:      store,
		freedomPay: freedompay.NewClient(cfg),
		stream:     stream,
	}

	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.Use(gin.Logger(), gin.Recovery(), s.cors())

	router.GET("/health", s.health)
	router.GET("/ready", s.ready)
	router.POST("/api/orders", s.createOrder)
	router.GET("/api/payments/freedompay/callback", s.freedomPayCallback)
	router.POST("/api/payments/freedompay/callback", s.freedomPayCallback)
	router.GET("/api/videos/:video_id/access", s.videoAccess)
	router.GET("/api/videos")
	router.POST("/api/videos", s.upsertVideo)
	router.NoRoute(s.notFound)
	router.NoMethod(s.methodNotAllowed)

	return router, nil
}

func (s Server) notFound(c *gin.Context) {
	if c.Request.Method == "OPTIONS" {
		c.Status(204)
		return
	}
	c.JSON(404, gin.H{"error": "not found"})
}

func (s Server) methodNotAllowed(c *gin.Context) {
	if c.Request.Method == "OPTIONS" {
		c.Status(204)
		return
	}
	c.JSON(405, gin.H{"error": "method not allowed"})
}

func (s Server) cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if s.originAllowed(origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func (s Server) originAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	for _, allowed := range s.cfg.SiteOrigins {
		if allowed == "*" || strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}
