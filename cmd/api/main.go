package main

import (
	"log"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"domain-detection-go/internal/auth"
	"domain-detection-go/internal/domain"
	"domain-detection-go/internal/handler"
	"domain-detection-go/internal/middleware"
	"domain-detection-go/internal/monitor"
	"domain-detection-go/internal/notification"
	"domain-detection-go/pkg/config"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Connect to database
	db, err := sqlx.Connect("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Initialize monitor service
	uptrendsConfig := monitor.UptrendsConfig{
		APIKey:      os.Getenv("UPTRENDS_API_KEY"),
		APIUsername: os.Getenv("UPTRENDS_USERNAME"),
		BaseURL:     os.Getenv("UPTRENDS_API_URL"), // Optional
		MaxRetries:  3,
		RetryDelay:  2 * time.Second,
	}
	uptrendsClient := monitor.NewUptrendsClient(uptrendsConfig)

	telegramConfig := notification.TelegramConfig{
		APIToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
	}

	// Initialize services
	authService := auth.NewAuthService(db, cfg.JWTSecret, cfg.EncryptionKey)
	domainService := domain.NewDomainService(db, uptrendsClient)
	telegramService := notification.NewTelegramService(telegramConfig, db)
	monitorService := monitor.NewMonitorService(uptrendsClient, domainService, telegramService)

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authService)
	domainHandler := handler.NewDomainHandler(domainService)
	telegramHandler := handler.NewTelegramHandler(telegramService)
	// monitorHandler := handler.NewMonitorHandler(monitorService)

	// Start the scheduled domain check in a goroutine
	go func() {
		monitorService.RunScheduledChecks()
	}()

	// Set up Gin router
	router := gin.Default()

	// Apply comprehensive CORS middleware
	corsConfig := cors.Config{
		AllowOrigins:     []string{"*"}, // Your Vue frontend URL
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           86400, // 24 hours
	}
	router.Use(cors.New(corsConfig))

	// Public routes
	router.POST("/api/login", authHandler.Login)
	router.POST("/api/register", authHandler.Register)
	router.GET("/api/regions", authHandler.GetRegions)

	// Protected routes
	protected := router.Group("/api")
	protected.Use(middleware.JWTAuthMiddleware(cfg.JWTSecret))
	{
		// 2FA routes
		protected.POST("/2fa/setup", authHandler.SetupTwoFactor)
		protected.POST("/2fa/verify", authHandler.VerifyTwoFactor)
		protected.POST("/2fa/disable", authHandler.DisableTwoFactor)

		// User profile
		protected.GET("/user/profile", authHandler.GetUserProfile)
		protected.PUT("/user/password", authHandler.UpdatePassword)

		// Domain management routes
		protected.GET("/domains", domainHandler.GetDomains)
		protected.GET("/domains/:id", domainHandler.GetDomain)
		protected.POST("/domains", domainHandler.AddDomain)
		protected.PUT("/domains/:id", domainHandler.UpdateDomain)
		protected.PUT("/domains/batch", domainHandler.UpdateAllDomains)
		protected.DELETE("/domains/:id", domainHandler.DeleteDomain)
		protected.POST("/domains/batch", domainHandler.AddBatchDomains)

		// Set up Telegram API routes
		telegramRoutes := protected.Group("/telegram")
		{
			telegramRoutes.GET("/bot", telegramHandler.GetBotInfo)
			telegramRoutes.GET("/configs", telegramHandler.GetTelegramConfigs)
			telegramRoutes.POST("/configs", telegramHandler.AddTelegramConfig)
			telegramRoutes.PUT("/configs/:id", telegramHandler.UpdateTelegramConfig)
			telegramRoutes.DELETE("/configs/:id", telegramHandler.DeleteTelegramConfig)

			// Add this new route for sending test messages
			telegramRoutes.POST("/configs/:id/test", telegramHandler.SendTestMessage)
		}

		// Admin routes
		admin := protected.Group("/admin")
		// TODO: Add admin middleware
		{
			admin.PUT("/settings/domain-limit", domainHandler.UpdateDomainLimit)
		}
	}

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
