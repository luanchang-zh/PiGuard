package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/auth"
)

const identityKey = "piguard.identity"

func deny(c *gin.Context, status, code int, message string) {
	if status == http.StatusUnauthorized {
		c.Header("WWW-Authenticate", `Basic realm="PiGuard", charset="UTF-8"`)
	}
	c.AbortWithStatusJSON(status, Response{Code: code, Message: message})
}

func originGuard(manager *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			deny(c, http.StatusUnauthorized, apperr.CodeUnauthorized, "authentication unavailable")
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Writer.Header().Add("Vary", "Origin")
		origin, allowed := manager.Origin(c.Request)
		if !allowed {
			deny(c, http.StatusForbidden, apperr.CodeForbidden, "origin denied")
			return
		}
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == http.MethodOptions {
			c.Writer.Header().Add("Vary", "Access-Control-Request-Method")
			c.Writer.Header().Add("Vary", "Access-Control-Request-Headers")
			methods := c.Request.Header.Values("Access-Control-Request-Method")
			if origin == "" || len(methods) != 1 || !allowedMethod(methods[0]) {
				deny(c, http.StatusForbidden, apperr.CodeForbidden, "preflight denied")
				return
			}
			for _, value := range c.Request.Header.Values("Access-Control-Request-Headers") {
				for _, header := range strings.Split(value, ",") {
					switch strings.ToLower(strings.TrimSpace(header)) {
					case "authorization", "content-type", "x-csrf-token":
					default:
						deny(c, http.StatusForbidden, apperr.CodeForbidden, "preflight denied")
						return
					}
				}
			}
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-CSRF-Token")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func allowedMethod(method string) bool {
	return method == "GET" || method == "POST" || method == "PUT" || method == "DELETE"
}

func accessGuard(manager *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			deny(c, http.StatusUnauthorized, apperr.CodeUnauthorized, "authentication unavailable")
			return
		}
		if manager.Disabled() {
			c.Next()
			return
		}
		identity, ok := manager.Authenticate(c.Request)
		if !ok {
			deny(c, http.StatusUnauthorized, apperr.CodeUnauthorized, "authentication required")
			return
		}
		if identity.Role != "platform" && !(identity.Role == "device" && c.Request.Method == "POST" && c.FullPath() == "/api/v1/devices/:device_id/frames" && c.Param("device_id") == identity.Username) {
			deny(c, http.StatusForbidden, apperr.CodeForbidden, "permission denied")
			return
		}
		if c.FullPath() == "/api/v1/auth/session" && c.Request.Method == "POST" && identity.Mode != "basic" {
			deny(c, http.StatusUnauthorized, apperr.CodeUnauthorized, "platform Basic authentication required")
			return
		}
		if identity.Mode == "cookie" && c.Request.Method != "GET" && c.Request.Method != "HEAD" {
			tokens := c.Request.Header.Values("X-CSRF-Token")
			if len(tokens) != 1 || !identity.Session.ValidCSRF(tokens[0]) {
				deny(c, http.StatusForbidden, apperr.CodeForbidden, "CSRF token required")
				return
			}
		}
		c.Set(identityKey, identity)
		c.Next()
	}
}

func sessionIdentity(c *gin.Context) auth.Identity {
	value, _ := c.Get(identityKey)
	identity, _ := value.(auth.Identity)
	return identity
}

func registerSessions(v1 *gin.RouterGroup, manager *auth.Manager) {
	if manager == nil || manager.Disabled() {
		return
	}
	v1.POST("/auth/session", func(c *gin.Context) {
		identity := sessionIdentity(c)
		if identity.Mode != "basic" {
			deny(c, http.StatusUnauthorized, apperr.CodeUnauthorized, "platform Basic authentication required")
			return
		}
		s, err := manager.CreateSession()
		if errors.Is(err, auth.ErrCapacity) {
			deny(c, http.StatusServiceUnavailable, apperr.CodeSessionCapacity, "session capacity reached")
			return
		}
		if err != nil {
			writeError(c, err)
			return
		}
		http.SetCookie(c.Writer, s.Cookie(c.Request))
		c.JSON(http.StatusCreated, Response{Code: 0, Message: "ok", Data: gin.H{"expires_at": s.ExpiresAt, "csrf_token": s.CSRFToken}})
	})
	v1.GET("/auth/session", func(c *gin.Context) {
		identity := sessionIdentity(c)
		data := gin.H{"username": identity.Username, "role": identity.Role, "auth_mode": identity.Mode, "expires_at": nil, "csrf_token": nil}
		s := identity.Session
		if s == nil {
			s = manager.RequestSession(c.Request)
		}
		if s != nil {
			data["expires_at"], data["csrf_token"] = s.ExpiresAt, s.CSRFToken
		}
		writeOK(c, data)
	})
	v1.DELETE("/auth/session", func(c *gin.Context) {
		manager.Revoke(manager.RequestSession(c.Request))
		http.SetCookie(c.Writer, auth.ClearCookie(c.Request))
		c.Status(http.StatusNoContent)
	})
}
