package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"taskqueue/internal/handler"
	"taskqueue/internal/metrics"
	"taskqueue/internal/queue"
	"time"

	"taskqueue/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {

	//先加载.env
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file")
	}
	//连数据库
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		log.Fatal("MYSQL_DSN is empty")
	}
	if err := store.Init(dsn); err != nil {
		log.Fatalf("init store err:%v", err)
	}
	//连redis
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		log.Fatal("REDIS_ADDR is empty")
	}
	if err := queue.Init(redisAddr); err != nil {
		log.Fatalf("init redis err :%v", err)
	}

	//设置metrics
	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":9090"
	}
	registerBusinessMetrics()
	metrics.StartServer(metricsAddr)

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	r := gin.Default()

	r.GET("/health", handler.Health)
	r.POST("/tasks", handler.CreateTask)
	r.GET("/tasks/:id", handler.GetTask)
	r.GET("/ready", handler.Ready)

	//优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}

	}()

	<-ctx.Done()
	stop()
	log.Println("shutdown signal received")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("shutdown forced: %v", err)
		srv.Close()

	}
	log.Println("server stopped")
}
