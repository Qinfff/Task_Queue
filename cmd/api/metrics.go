package main

import (
	"context"
	"log"
	"math"
	"taskqueue/internal/model"
	"taskqueue/internal/queue"
	"taskqueue/internal/store"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type taskStatusCollector struct {
	desc *prometheus.Desc
}

func newTaskStatusCollector() *taskStatusCollector {
	return &taskStatusCollector{
		desc: prometheus.NewDesc(
			"taskqueue_tasks",
			"Number of tasks by status",
			[]string{"status"},
			nil,
		),
	}
}

func (c *taskStatusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *taskStatusCollector) Collect(ch chan<- prometheus.Metric) {
	var rows []struct {
		Status string
		Total  int64
	}

	if err := store.DB.Model(&model.Task{}).Select("status,count(*) as total").Group(
		"status").Scan(&rows).Error; err != nil {
		log.Printf("collect taskqueue_tasks failed :%v", err)
		return
	}
	for _, r := range rows {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(r.Total), r.Status)

	}
}

func registerBusinessMetrics() {
	prometheus.MustRegister(newTaskStatusCollector())

	prometheus.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "taskqueue_queue_length",
			Help: "Number of tasks waiting in the redis queue",
		},
		func() float64 {
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			n, err := queue.Len(ctx)
			if err != nil {
				log.Printf("collect taskqueue_queue_length failed: %v", err)
				return math.NaN()
			}
			return float64(n)
		},
	))
}
