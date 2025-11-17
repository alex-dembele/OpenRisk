package main

import (
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
	"net/http"
	"time"
	"os"
)

func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)
		log.Info().
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Dur("duration", duration).
			Msg("Request handled")
	}
}

func JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := c.GetHeader("Authorization")
		if tokenStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "No token"})
			c.Abort()
			return
		}
		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			return []byte(os.Getenv("JWT_SECRET")), nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}
		claims := token.Claims.(jwt.MapClaims)
		role := claims["role"].(string)
		c.Set("role", role) 
		c.Next()
	}
}


func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role != "Admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin only"})
			c.Abort()
			return
		}
		c.Next()
	}
}