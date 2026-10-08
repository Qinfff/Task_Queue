package main

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"syscall"

	"log"
	"os"
	"taskqueue/internal/metrics"
	"taskqueue/internal/model"
	"taskqueue/internal/queue"
	"taskqueue/internal/store"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

const (
	recycleInterval = 30 * time.Second
	stuckTimeout    = 2 * time.Minute
	maxRetries      = 3
	taskTimeout     = 10 * time.Second
)

var ErrTimeout = errors.New("task timed out")

func recycleLoop(ctx context.Context) {
	ticker := time.NewTicker(recycleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			recycle(ctx)
		}
	}
}

func recycle(ctx context.Context) {
	cutoff := time.Now().Add(-stuckTimeout).Unix()
	now := time.Now().Unix()

	var ids []uint

	/*	1.执行中卡死
		2.执行失败待重试
		3.没入过队的僵尸
	*/
	due := store.DB.Model(&model.Task{}).
		Where("status = ? AND updated_at < ?", model.StatusRunning,
			cutoff).Or(
		"status = ? AND next_retry_at > 0 AND next_retry_at <= ?", model.StatusPending,
		now).Or(
		"status = ? AND next_retry_at = 0 AND updated_at < ?", model.StatusPending,
		cutoff)

	if err := store.DB.Model(&model.Task{}).Where(due).Pluck("id", &ids).Error; err != nil {
		log.Printf("recycle scan failed :%v", err)
		return
	}
	//先查出
	//再逐条更新

	n := 0
	for _, id := range ids {
		res := store.DB.Model(&model.Task{}).
			Where(due).
			Where("id = ?", id).
			Updates(map[string]any{
				"status":        model.StatusPending,
				"updated_at":    now,
				"next_retry_at": 0,
			})
		if res.Error != nil {
			log.Printf("recycle update %d failed :%v", id, res.Error)
			continue
		}
		if res.RowsAffected == 0 {
			continue
		}
		if err := queue.Push(ctx, id); err != nil {
			log.Printf("recycle redis push %d failed :%v", id, err)
			continue
		}
		n++
	}

	if n > 0 {
		log.Printf("recycled %d task(s)", n)
	}
}

func execute(ctx context.Context, task *model.Task) error {
	select {
	case <-time.After(200 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	switch task.Type {
	case "fail":
		return errors.New("simulated failure")
	case "slow":
		time.Sleep(30 * time.Second)
		log.Printf("slow task %d finished", task.ID)
	case "effect":
		if res := store.DB.Create(&model.TaskResult{
			TaskID: task.ID,
			Result: "effect applied",
		}); res.Error != nil {
			if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
				log.Printf("task %d effect already applied, skip", task.ID)
				return nil
			}
			return res.Error
		}
		log.Printf("task %d effect applied", task.ID)
	}
	return nil
}

//退避算法
func backoff(retires int) time.Duration {
	if retires > 6 {
		retires = 6
	}
	return time.Duration(1<<uint(retires)) * time.Second
}

func main() {

	if err := godotenv.Load(); err != nil {
		log.Println("no .env file")
	}

	dsn := os.Getenv("MYSQL_DSN")
	redisAddr := os.Getenv("REDIS_ADDR")

	if dsn == "" {
		log.Fatal("MYSQL_DSN is empty")
	}

	if redisAddr == "" {
		log.Fatal("REDIS_ADDR is empty")
	}

	if err := store.Init(dsn); err != nil {
		log.Fatalf("init store err:%v", err)
	}

	if err := queue.Init(redisAddr); err != nil {
		log.Fatalf("init queue err:%v", err)
	}

	//设置metrics
	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":9090"
	}
	registerWorkerMetrics()
	metrics.StartServer(metricsAddr)

	log.Println("worker started")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	//垃圾回收
	go recycleLoop(ctx)

	//正常处理队列里的任务
	for {
		id, err := queue.Pop(ctx)

		if ctx.Err() != nil {
			log.Printf("shutdown signal received, stop consuming")
			break
		}
		if errors.Is(err, queue.ErrEmpty) {
			continue
		}

		if err != nil {
			log.Printf("pop failed :%v", err)
			continue
		}

		//抢锁
		//pending -> running
		res := store.DB.Model(&model.Task{}).
			Where("id = ? AND status = ?", id, model.StatusPending).
			Updates(model.Task{Status: model.StatusRunning})
		if res.Error != nil {
			log.Printf("claim task %d failed: %v", id, res.Error)
			continue
		}
		if res.RowsAffected == 0 {
			log.Printf("task %d already claimed, skip", id)
			continue
		}

		var task model.Task
		if err := store.DB.First(&task, id).Error; err != nil {
			log.Printf("load task %d failed :%v", id, err)
			continue
		}
		log.Printf("executing task %d", id)
		execStart := time.Now()
		//模拟超时任务
		execCtx, cancel := context.WithTimeout(ctx, taskTimeout)
		execCh := make(chan error, 1)

		go func() { execCh <- execute(execCtx, &task) }()

		var execErr error
		select {
		case execErr = <-execCh:
		case <-execCtx.Done():
			if ctx.Err() != nil {
				execErr = ctx.Err()
			} else {
				execErr = fmt.Errorf("%w after %s", ErrTimeout, taskTimeout)
			}

		}
		cancel()
		taskDuration.Observe(time.Since(execStart).Seconds())
		//检查是否是上游ctx.cancel(),如果取消就放回队列
		if errors.Is(execErr, context.Canceled) {
			if err := store.DB.Model(&model.Task{}).Where("id = ?", id).
				Updates(map[string]any{
					"status":        model.StatusPending,
					"updated_at":    time.Now().Unix(),
					"next_retry_at": 0,
				}).Error; err != nil {
				log.Printf("task %d requeue failed due to can not update DB :%v", id, err)
				break
			}
			if err := queue.Push(context.Background(), uint(id)); err != nil {
				log.Printf("requeue task %d failed :%v", id, err)
			}
			log.Printf("task %d requeued due to shutdown", id)
			break

		}

		//失败分支
		if execErr != nil {
			tasksProcessed.WithLabelValues("failed").Inc()
			retries := task.Retries + 1

			if retries >= maxRetries {
				res := store.DB.Model(&model.Task{}).Where("id = ?", id).
					Updates(map[string]any{
						"status":     model.StatusDead,
						"retries":    retries,
						"last_error": execErr.Error(),
					})
				if res.Error != nil {
					log.Printf("mark task %d dead failed: %v", id, res.Error)
				}
				log.Printf("task %d dead after %d retries: %v", id, retries, execErr)
				continue
			}
			next := time.Now().Add(backoff(retries)).Unix()
			res := store.DB.Model(&model.Task{}).Where("id = ?", id).
				Updates(map[string]any{
					"status":        model.StatusPending,
					"retries":       retries,
					"last_error":    execErr.Error(),
					"next_retry_at": next,
				})
			if res.Error != nil {
				log.Printf("schedule retry for task %d failed: %v", id, res.Error)
				continue
			}
			log.Printf("task %d failed, retry %d at %s: %v",
				id, retries, time.Unix(next, 0).Format("15:04:05"), execErr)
			continue

		}
		tasksProcessed.WithLabelValues("success").Inc()
		//成功分支
		if err := store.DB.Model(&model.Task{}).
			Where("id = ?", id).
			Updates(model.Task{Status: model.StatusSuccess}).Error; err != nil {
			log.Printf("mark task %d success failed: %v", id, err)
		}

	}

}
