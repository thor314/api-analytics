package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/tom-draper/api-analytics/server/database"
	"github.com/tom-draper/api-analytics/server/logger/lib/log"
	"github.com/tom-draper/api-analytics/server/logger/lib/ratelimit"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/oschwald/geoip2-golang"
)

func main() {
	log.LogToFile("Starting logger...")

	gin.SetMode(gin.ReleaseMode)
	app := gin.New()

	app.Use(cors.Default())

	handler := logRequestHandler()
	app.POST("/api/log-request", handler)
	app.POST("/api/requests", handler)

	app.Run(":8000")
}

type RequestData struct {
	Path         string `json:"path"`
	Hostname     string `json:"hostname"`
	IPAddress    string `json:"ip_address"`
	UserAgent    string `json:"user_agent"`
	Method       string `json:"method"`
	Status       int16  `json:"status"`
	ResponseTime int16  `json:"response_time"`
	UserID       string `json:"user_id"`
	CreatedAt    string `json:"created_at"`
}

type Payload struct {
	APIKey       string        `json:"api_key"`
	Requests     []RequestData `json:"requests"`
	Framework    string        `json:"framework"`
	PrivacyLevel PrivacyLevel  `json:"privacy_level"`
}

type PrivacyLevel int

const (
	P1 PrivacyLevel = iota // Client IP address stored, location inferred from IP and stored
	P2                     // Location inferred from IP and stored, client IP address discarded
	P3                     // Client IP address never be sent to server, optional custom user ID field is the only user identification
)

func getCountryCode(IPAddress string) string {
	if IPAddress == "" {
		return ""
	}
	db, err := geoip2.Open("GeoLite2-Country.mmdb")
	if err != nil {
		return ""
	}
	defer db.Close()

	ip := net.ParseIP(IPAddress)
	if ip == nil {
		return ""
	}
	record, err := db.Country(ip)
	if err != nil {
		return ""
	}
	location := record.Country.IsoCode
	return location
}

func logRequestHandler() gin.HandlerFunc {
	var rateLimiter = ratelimit.RateLimiter{}

	const maxInsert int = 2000

	var methodID = map[string]int16{
		"GET":     0,
		"POST":    1,
		"PUT":     2,
		"PATCH":   3,
		"DELETE":  4,
		"OPTIONS": 5,
		"CONNECT": 6,
		"HEAD":    7,
		"TRACE":   8,
	}

	var frameworkID = map[string]int16{
		"FastAPI":      0,
		"Flask":        1,
		"Gin":          2,
		"Echo":         3,
		"Express":      4,
		"Fastify":      5,
		"Koa":          6,
		"Chi":          7,
		"Fiber":        8,
		"Actix":        9,
		"Axum":         10,
		"Tornado":      11,
		"Django":       12,
		"Rails":        13,
		"Laravel":      14,
		"Sinatra":      15,
		"Rocket":       16,
		"ASP.NET Core": 17,
	}

	return func(c *gin.Context) {
		// Collect API request data sent via POST request
		var payload Payload
		err := c.BindJSON(&payload)
		if err != nil {
			msg := "Invalid request data."
			log.LogErrorToFile(c.ClientIP(), "", msg)
			c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": msg})
			return
		} else if payload.APIKey == "" {
			msg := "API key requied."
			log.LogErrorToFile(c.ClientIP(), payload.APIKey, msg)
			c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": msg})
			return
		} else if rateLimiter.RateLimited(payload.APIKey) {
			msg := "Too many requests."
			log.LogErrorToFile(c.ClientIP(), payload.APIKey, msg)
			c.JSON(http.StatusTooManyRequests, gin.H{"status": http.StatusTooManyRequests, "message": msg})
			return
		} else if len(payload.Requests) == 0 {
			msg := "Payload contains no logged requests."
			log.LogErrorToFile(c.ClientIP(), payload.APIKey, msg)
			c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": msg})
			return
		}

		framework, ok := frameworkID[payload.Framework]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": "Unsupported API framework."})
			return
		}

		var query strings.Builder
		query.WriteString("INSERT INTO requests (api_key, path, hostname, ip_address, user_agent, status, response_time, method, framework, location, user_id, created_at) VALUES ")
		arguments := make([]any, 0)
		inserted := 0
		badUserAgents := []string{}
		for _, request := range payload.Requests {
			// Temporary 1000 request per minute limit
			if inserted >= maxInsert {
				break
			}

			var location string
			if payload.PrivacyLevel < P3 {
				// Location inferred from IP and stored for privacy level P1 and P2
				location = getCountryCode(request.IPAddress)
			}

			if payload.PrivacyLevel > P1 {
				// Client IP address discarded for privacy level P2 and P3
				request.IPAddress = ""
			}

			method, ok := methodID[request.Method]
			if !ok {
				continue
			}

			if len(request.UserAgent) > 255 {
				request.UserAgent = request.UserAgent[:255]
			}
			if !database.ValidUserAgent(request.UserAgent) {
				badUserAgents = append(badUserAgents, request.UserAgent)
				continue
			}

			if len(request.UserID) > 255 {
				request.UserID = request.UserID[:255]
			}
			if !database.ValidUserID(request.UserID) {
				continue
			}

			if len(request.Hostname) > 255 {
				request.Hostname = request.Hostname[:255]
			}
			if !database.ValidHostname(request.Hostname) {
				continue
			}

			if len(request.Path) > 255 {
				request.Path = request.Path[:255]
			}
			if !database.ValidPath(request.Path) {
				continue
			}

			// If not at final row in query, separate with comma
			if inserted > 0 && inserted < maxInsert && inserted < len(payload.Requests) {
				query.WriteString(",")
			}
			numArgs := len(arguments)
			query.WriteString(
				fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
					numArgs+1,
					numArgs+2,
					numArgs+3,
					numArgs+4,
					numArgs+5,
					numArgs+6,
					numArgs+7,
					numArgs+8,
					numArgs+9,
					numArgs+10,
					numArgs+11,
					numArgs+12),
			)
			arguments = append(
				arguments,
				payload.APIKey,
				request.Path,
				request.Hostname,
				request.IPAddress,
				request.UserAgent,
				request.Status,
				request.ResponseTime,
				method,
				framework,
				location,
				request.UserID,
				request.CreatedAt)
			inserted += 1
		}

		// Record in log file for debugging
		log.LogRequestsToFile(payload.APIKey, inserted, len(payload.Requests))
		// Log any bad user agents found
		if len(badUserAgents) > 0 {
			var msg bytes.Buffer
			for i, userAgent := range badUserAgents {
				msg.WriteString(fmt.Sprintf("[%d] bad user agent: %s\n", i, userAgent))
			}
			log.LogToFile(msg.String())
		}

		// If no valid logged requests received
		if inserted == 0 {
			log.LogToFile("No rows inserted.")
			c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": "Invalid request data."})
			return
		}

		query.WriteString(";")

		// Insert logged requests into database
		conn := database.NewConnection()
		_, err = conn.Exec(context.Background(), query.String(), arguments...)
		conn.Close(context.Background())
		if err != nil {
			log.LogToFile(err.Error())
			c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": "Invalid data."})
			return
		}

		// Return success response
		c.JSON(http.StatusCreated, gin.H{"status": http.StatusCreated, "message": "API requests logged successfully."})
	}
}
