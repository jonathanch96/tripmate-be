package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/jblabs/tripmate-be/adapters/rest/config"
	"github.com/jblabs/tripmate-be/adapters/rest/handlers"
	_ "github.com/jblabs/tripmate-be/docs"
	appLogger "github.com/jblabs/tripmate-be/pkg/logger"
	tripmate "github.com/jblabs/tripmate-be/services/tripmate/v1/controllers"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

// @title TripMate API
// @version 1.0
// @description Shared-trip expense settlement API.
// @host localhost:8080
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	log := appLogger.New(cfg.App.Env)
	appLogger.SetDefault(log)
	db, err := config.NewDatabase(cfg, log)
	if err != nil {
		log.Error("startup failed", "err", err)
		os.Exit(1)
	}
	engine := config.NewServer(cfg, log)
	registerRoutes(engine, cfg, db, log)
	if err := config.Run(context.Background(), cfg, engine); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func registerRoutes(engine *gin.Engine, cfg *config.Config, db *gorm.DB, log *slog.Logger) {
	engine.GET("/healthz", handlers.Health)
	engine.GET("/readyz", handlers.Ready(db))
	engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	v1 := engine.Group("/api/v1")
	tripmate.NewService(tripmate.Dependencies{DB: db, Cfg: cfg, Log: log}).RegisterRoutes(v1)
}
