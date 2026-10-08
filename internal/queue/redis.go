package queue

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var client *redis.Client

var ErrEmpty = errors.New("queue is empty")

const(
	queueKey = "task:queue"
	popTimeout = 5 * time.Second
)

func Init(addr string) error{
	client = redis.NewClient(&redis.Options{
		Addr : addr,
	})

	ctx, cancel := context.WithTimeout(context.Background(),5*time.Second)
	defer cancel()

	return client.Ping(ctx).Err()
}
func Len(ctx context.Context)(int64, error){
	return client.LLen(ctx,queueKey).Result()

}

func Push(ctx context.Context,taskID uint) error{
	return client.LPush(ctx,queueKey,strconv.FormatUint(uint64(taskID),10)).Err()
}

func Pop(ctx context.Context)(uint64, error){
	result, err := client.BRPop(ctx,popTimeout,queueKey).Result()
	if errors.Is(err,redis.Nil){
		return 0, ErrEmpty
	}

	if err!=nil{
		return 0, err
	}
	id, err := strconv.ParseUint(result[1],10,64)
	if err!=nil{
		return 0, err
	}
	return  id,nil
}