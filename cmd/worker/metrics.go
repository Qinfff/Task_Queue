package main

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	tasksProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "taskqueue_tasks_processed_total",
			Help: "Total number of processed tasks by result",
		},
		[]string{"result"},
	)

	taskDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "taskqueue_task_duration_seconds",
			Help:    "Task execution duration in seconds",
			Buckets: []float64{0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.5, 1, 2, 5},
		},
	)
)

func registerWorkerMetrics() {
	prometheus.MustRegister(tasksProcessed, taskDuration)
}
