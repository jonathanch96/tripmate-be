package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jblabs/tripmate-be/adapters/rest/config"
	"github.com/jblabs/tripmate-be/adapters/rest/handlers"
	_ "github.com/jblabs/tripmate-be/docs"
	"github.com/jblabs/tripmate-be/pkg/apperror"
	appLogger "github.com/jblabs/tripmate-be/pkg/logger"
	"github.com/jblabs/tripmate-be/pkg/ocr"
	"github.com/jblabs/tripmate-be/pkg/ocr/gemini"
	"github.com/jblabs/tripmate-be/pkg/response"
	"github.com/jblabs/tripmate-be/pkg/storage"
	tripmate "github.com/jblabs/tripmate-be/services/tripmate/v1/controllers"
	receiptdomain "github.com/jblabs/tripmate-be/services/tripmate/v1/domain/receipt"
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
	if err = registerRoutes(engine, cfg, db, log); err != nil {
		log.Error("storage startup failed", "err", err)
		os.Exit(1)
	}
	if err := config.Run(context.Background(), cfg, engine); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func registerRoutes(engine *gin.Engine, cfg *config.Config, db *gorm.DB, log *slog.Logger) error {
	engine.GET("/healthz", handlers.Health)
	engine.GET("/readyz", handlers.Ready(db))
	engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	v1 := engine.Group("/api/v1")
	var objectStore storage.ObjectStorage
	var localStore *storage.Filesystem
	if cfg.Storage.Endpoint != "" {
		var err error
		objectStore, err = storage.NewS3(context.Background(), storage.S3Config{Endpoint: cfg.Storage.Endpoint,
			AccessKey: cfg.Storage.AccessKey, SecretKey: cfg.Storage.SecretKey, Bucket: cfg.Storage.Bucket,
			Region: cfg.Storage.Region, UseSSL: cfg.Storage.UseSSL})
		if err != nil {
			return err
		}
	} else {
		localStore = storage.NewFilesystem(cfg.Storage.LocalRoot, cfg.Storage.PublicURL, []byte(cfg.JWT.AccessSecret))
		objectStore = localStore
	}
	var provider receiptdomain.OCRProvider = ocr.NewNoop()
	if cfg.OCR.GeminiAPIKey != "" {
		provider = gemini.New(gemini.Config{APIKey: cfg.OCR.GeminiAPIKey, Model: cfg.OCR.GeminiModel, Timeout: cfg.OCR.Timeout})
	}
	if localStore != nil {
		v1.GET("/receipt-images/*key", func(c *gin.Context) {
			expires, err := strconv.ParseInt(c.Query("expires"), 10, 64)
			if err != nil {
				response.Error(c, apperror.New("RECEIPT_IMAGE_LINK_INVALID"))
				return
			}
			key := strings.TrimPrefix(c.Param("key"), "/")
			file, err := localStore.Open(key, c.Query("signature"), expires)
			if err != nil {
				response.Error(c, apperror.New("RECEIPT_IMAGE_LINK_INVALID"))
				return
			}
			defer func() { _ = file.Close() }()
			contentType := "image/jpeg"
			if strings.HasSuffix(key, ".png") {
				contentType = "image/png"
			}
			if strings.HasSuffix(key, ".webp") {
				contentType = "image/webp"
			}
			c.DataFromReader(http.StatusOK, -1, contentType, file, nil)
		})
	}
	tripmate.NewService(tripmate.Dependencies{DB: db, Cfg: cfg, Log: log, Storage: objectStore, OCR: provider}).RegisterRoutes(v1)
	return nil
}
