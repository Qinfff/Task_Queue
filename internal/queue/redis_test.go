package queue

import (
	"context"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestConcurrentPush(t *testing.T) {
	mr := miniredis.RunT(t)
	if err := Init(mr.Addr()); err != nil {
		t.Fatalf("Init failed :%v", err)
	}

	ctx := context.Background()

	var goroutines = 10
	var perG = 10
	var want = goroutines * perG

	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id uint) {
			defer wg.Done()
			for j := 0; j < perG; j++ {

				if err := Push(ctx, id); err != nil {
					t.Errorf("push failed :%v", err)
				}
			}
		}(uint(i))
	}
	wg.Wait()

	n, err := Len(ctx)
	if err != nil {
		t.Fatalf("get len failed :%v", err)
	}
	if n != int64(want) {
		t.Fatalf("队列长度 = %d, 期望 %d", n, want)
	}
}

func TestPopBasic(t *testing.T) {
	mr := miniredis.RunT(t)
	if err := Init(mr.Addr()); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	ctx := context.Background()

	for _, id := range []uint{1, 2, 3} {
		if err := Push(ctx, id); err != nil {
			t.Fatalf("Push(%d) 失败: %v", id, err)
		}
	}

	n, err := Len(ctx)
	if err != nil {
		t.Fatalf("Len 失败: %v", err)
	}
	if n != 3 {
		t.Fatalf("队列长度 = %d, 期望 3", n)
	}

	for want := uint64(1); want <= 3; want++ {
		got, err := Pop(ctx)
		if err != nil {
			t.Fatalf("Pop 失败: %v", err)
		}
		if got != want {
			t.Errorf("第 %d 次 Pop = %d, 期望 %d", want, got, want)
		}
	}
}

func TestConcurrentPopNoDuplicate(t *testing.T) {
	mr := miniredis.RunT(t)
	if err := Init(mr.Addr()); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	ctx := context.Background()

	const (
		total      = 100
		goroutines = 10
		perG       = total / goroutines
	)

	for i := 1; i <= total; i++ {
		if err := Push(ctx, uint(i)); err != nil {
			t.Fatalf("Push(%d) 失败: %v", i, err)
		}
	}

	results := make([][]uint64, goroutines)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				id, err := Pop(ctx)
				if err != nil {
					t.Errorf("Pop 失败: %v", err)
					return
				}
				results[idx] = append(results[idx], id)
			}
		}(i)
	}
	wg.Wait()

	seen := make(map[uint64]int, total)
	count := 0
	for _, ids := range results {
		for _, id := range ids {
			seen[id]++
			count++
		}
	}

	if count != total {
		t.Errorf("共 Pop 出 %d 个, 期望 %d", count, total)
	}
	for id, c := range seen {
		if c > 1 {
			t.Errorf("ID %d 被重复消费 %d 次", id, c)
		}
	}
}
