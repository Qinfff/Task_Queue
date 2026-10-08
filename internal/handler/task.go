package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"taskqueue/internal/model"
	"taskqueue/internal/queue"
	"taskqueue/internal/store"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type createTaskRequest struct {
	Type    string `json:"type" binding:"required"`
	Payload string `json:"payload"`
}


func CreateTask(c *gin.Context) {
	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("bind request failed :%v",err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	task := model.Task{
		Type:    req.Type,
		Payload: req.Payload,
		Status:  model.StatusPending,
	}
	//先数据库再Redis

	if err := store.DB.Create(&task).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		log.Printf("create task fail :%v", err)
		return
	}
	ctx := c.Request.Context()

	if err := queue.Push(ctx,task.ID);err !=nil{
		log.Printf("taskid: %d redis push failed",task.ID)
	}

	c.JSON(http.StatusCreated, gin.H{
		"task_id": task.ID,
		"status":  task.Status,
	})
}

func GetTask(c *gin.Context) {

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id"})
		return
	}

	var task model.Task
	if err := store.DB.First(&task, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "can not find it",
			})
			return
		}
		log.Printf("get task failed,id=%v,err=%v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, task)
}
