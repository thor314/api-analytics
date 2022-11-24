package api

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	supa "github.com/nedpals/supabase-go"
)

var (
	app *gin.Engine
)

type User struct {
	UserID    string    `json:"user_id"`
	APIKey    string    `json:"api_key"`
	CreatedAt time.Time `json:"created_at"`
}

func GenAPIKeyHandler(supabase *supa.Client) gin.HandlerFunc {
	genAPIKey := func(c *gin.Context) {
		var row struct{} // Insert empty row, use default values
		var result []User
		err := supabase.DB.From("Users").Insert(row).Execute(&result)
		if err != nil {
			panic(err)
		}

		// Get API key auto generated from new row insertion
		apiKey := result[0].APIKey

		// Return API key
		c.JSON(200, gin.H{"value": apiKey})
	}

	return gin.HandlerFunc(genAPIKey)
}

type Request struct {
	APIKey       string `json:"api_key"`
	Path         string `json:"path"`
	UserAgent    string `json:"user_agent"`
	Method       int16  `json:"method"`
	StatusCode   int16  `json:"status_code"`
	ResponseTime int16  `json:"response_time"`
	Framework    int16  `json:"framework"`
}

func LogRequestHandler(supabase *supa.Client) gin.HandlerFunc {
	logRequest := func(c *gin.Context) {
		// Collect API request data sent via POST request
		var request Request
		if err := c.BindJSON(&request); err != nil {
			panic(err)
		}

		// Insert request data into database
		var result []interface{}
		err := supabase.DB.From("Requests").Insert(request).Execute(&result)
		if err != nil {
			panic(err)
		}

		// Return success response
		c.JSON(http.StatusCreated, gin.H{"status": http.StatusCreated, "message": "API request logged successfully."})
	}

	return gin.HandlerFunc(logRequest)
}

func GetUserIDHandler(supabase *supa.Client) gin.HandlerFunc {
	getUserID := func(c *gin.Context) {
		// Collect API key sent via POST request
		var apiKey string
		if err := c.BindJSON(&apiKey); err != nil {
			panic(err)
		}

		// Fetch user ID corresponding with API key
		var result []struct {
			UserID string `json:"user_id"`
		}
		err := supabase.DB.From("Users").Select("user_id").Filter("api_key", "eq", apiKey).Execute(&result)
		if err != nil {
			panic(err)
		}

		userID := result[0].UserID

		// Return user ID
		c.JSON(200, gin.H{"value": userID})
	}

	return gin.HandlerFunc(getUserID)
}

type RequestRow struct {
	RequestID    int16  `json:"request_id" `
	APIKey       string `json:"api_key"`
	Path         string `json:"path"`
	UserAgent    string `json:"user_agent"`
	Method       int16  `json:"method"`
	StatusCode   int16  `json:"status_code"`
	ResponseTime int16  `json:"response_time"`
	Framework    int16  `json:"framework"`
}

func GetDataHandler(supabase *supa.Client) gin.HandlerFunc {
	getData := func(c *gin.Context) {
		// Collect user ID sent via POST request
		var userID string
		if err := c.BindJSON(&userID); err != nil {
			panic(err)
		}

		fmt.Println(userID)

		// Fetch all API request data associated with this account
		var result []RequestRow
		err := supabase.DB.From("Requests").Select("*").Filter("user_id", "eq", userID).Execute(&result)
		if err != nil {
			panic(err)
		}

		fmt.Println(result)

		// Return API request data
		c.JSON(200, gin.H{"value": result})
	}

	return gin.HandlerFunc(getData)
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func registerRouter(r *gin.RouterGroup, supabase *supa.Client) {
	r.GET("/generate-api-key", GenAPIKeyHandler(supabase))
	r.POST("/log-request", LogRequestHandler(supabase))
	r.POST("/user-id", GetUserIDHandler(supabase))
	r.POST("/data", GetDataHandler(supabase))
}

func getDBLogin() (string, string) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_KEY")
	return supabaseURL, supabaseKey
}

func init() {
	supabaseURL, supabaseKey := getDBLogin()
	supabase := supa.CreateClient(supabaseURL, supabaseKey)

	app = gin.New()

	r := app.Group("/api") // Vercel - must be /api/xxx

	// r.Use(CORSMiddleware())
	r.Use(cors.Default())
	registerRouter(r, supabase) // Register route
}

func Handler(w http.ResponseWriter, r *http.Request) {
	app.ServeHTTP(w, r)
}
