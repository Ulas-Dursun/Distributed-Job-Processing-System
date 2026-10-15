package api

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/ulasdursun/queueforge/internal/api/handler"
	"github.com/ulasdursun/queueforge/internal/api/middleware"
	"github.com/ulasdursun/queueforge/internal/queue"
	"github.com/ulasdursun/queueforge/internal/repository"
)

// NewRouter constructs the full HTTP router for API mode or all-in-one mode.
func NewRouter(
	jobRepo *repository.JobRepository,
	q *queue.RedisQueue,
	maxRetries int,
	db *sql.DB,
	redisClient *redis.Client,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger())

	r.Static("/static", "./static")
	r.StaticFile("/", "./static/index.html")

	healthHandler := handler.NewHealthHandler(db, redisClient)
	r.GET("/health", healthHandler.Health)
	r.GET("/health/live", healthHandler.Live)
	r.GET("/health/ready", healthHandler.Ready)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	jobHandler := handler.NewJobHandler(jobRepo, q, maxRetries)
	api := r.Group("/api")
	{
		jobs := api.Group("/jobs")
		{
			jobs.POST("", jobHandler.CreateJob)
			jobs.GET("", jobHandler.ListJobs)
			jobs.GET("/:id", jobHandler.GetJob)
		}
	}

	return r
}

// NewWorkerRouter constructs a minimal router exposing metrics and health checks for standalone workers.
func NewWorkerRouter(db *sql.DB, redisClient *redis.Client) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())

	healthHandler := handler.NewHealthHandler(db, redisClient)
	r.GET("/health", healthHandler.Health)
	r.GET("/health/live", healthHandler.Live)
	r.GET("/health/ready", healthHandler.Ready)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	return r
}
