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
	"domain-detection-go/internal/service"
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

	// Initialize Site24x7 client
	site24x7Config := monitor.Site24x7Config{
		ClientID:     os.Getenv("SITE24X7_CLIENT_ID"),
		ClientSecret: os.Getenv("SITE24X7_CLIENT_SECRET"),
		RefreshToken: os.Getenv("SITE24X7_REFRESH_TOKEN"),
		BaseURL:      "https://www.site24x7.com/api",
	}
	site24x7Client := monitor.NewSite24x7Client(site24x7Config)

	telegramConfig := notification.TelegramConfig{
		APIToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
	}

	// Add email configuration
	emailConfig := notification.EmailConfig{
		SMTPHost:     os.Getenv("SMTP_HOST"),
		SMTPPort:     os.Getenv("SMTP_PORT"),
		SMTPUsername: os.Getenv("SMTP_USERNAME"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		FromEmail:    os.Getenv("FROM_EMAIL"),
		FromName:     os.Getenv("FROM_NAME"),
	}

	// Initialize services
	authService := auth.NewAuthService(db, cfg.JWTSecret, cfg.EncryptionKey)
	domainService := domain.NewDomainService(db, uptrendsClient, site24x7Client)
	promptService := service.NewTelegramPromptService(db)
	telegramService := notification.NewTelegramService(telegramConfig, db, promptService)
	emailService := notification.NewEmailService(emailConfig, db, promptService)
	monitorService := monitor.NewMonitorService(uptrendsClient, site24x7Client, domainService, telegramService, emailService)

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authService)
	domainHandler := handler.NewDomainHandler(domainService)
	telegramHandler := handler.NewTelegramHandler(telegramService)
	telegramBotHandler := handler.NewTelegramBotHandler(telegramService, domainService)
	promptHandler := handler.NewTelegramPromptHandler(promptService)
	emailHandler := handler.NewEmailHandler(emailService)
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

	// Add webhook endpoint for Telegram bot (public, no auth required)
	router.POST("/api/telegram/webhook", telegramBotHandler.WebhookHandler)

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

		// Add email API routes
		emailRoutes := protected.Group("/email")
		{
			emailRoutes.GET("/configs", emailHandler.GetEmailConfigs)
			emailRoutes.POST("/configs", emailHandler.AddEmailConfig)
			emailRoutes.PUT("/configs/:id", emailHandler.UpdateEmailConfig)
			emailRoutes.DELETE("/configs/:id", emailHandler.DeleteEmailConfig)
			emailRoutes.POST("/configs/:id/test", emailHandler.SendTestEmail)
		}

		// prompt management routes
		protected.GET("/telegram-prompts", promptHandler.GetPrompts)
		protected.GET("/telegram-prompts/:id", promptHandler.GetPrompt)
		protected.POST("/telegram-prompts", promptHandler.CreatePrompt)
		protected.PUT("/telegram-prompts/:id", promptHandler.UpdatePrompt)
		protected.DELETE("/telegram-prompts/:id", promptHandler.DeletePrompt)

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
