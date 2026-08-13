package tasks

import "sync"

// RunConcurrent 并发执行 fn for each item，最大并发 max
func RunConcurrent[T any](items []T, max int, fn func(T)) {
	if max <= 0 {
		max = 1
	}
	sem := make(chan struct{}, max)
	var wg sync.WaitGroup
	for _, item := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(it T) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(it)
		}(item)
	}
	wg.Wait()
}