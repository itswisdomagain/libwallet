package syncutils

import (
	"fmt"
	"sync"
)

type lister[T any] struct {
	mtx  sync.RWMutex
	list map[string]T
	wg   sync.WaitGroup
}

func newLister[T any]() *lister[T] {
	return &lister[T]{
		list: make(map[string]T),
	}
}

func (lister *lister[T]) Add(key string, item T) error {
	lister.mtx.Lock()
	defer lister.mtx.Unlock()
	if _, exists := lister.list[key]; exists {
		return fmt.Errorf("duplicate entry for key: %s", key)
	}
	lister.list[key] = item
	return nil
}

func (lister *lister[T]) Remove(key string) {
	lister.mtx.Lock()
	defer lister.mtx.Unlock()
	delete(lister.list, key)
}

func (lister *lister[T]) RangeAsync(rangeFn func(T)) {
	lister.mtx.RLock()
	defer lister.mtx.RUnlock()
	for _, listener := range lister.list {
		lister.doAsync(listener, rangeFn)
	}
}

func (lister *lister[T]) doAsync(item T, do func(T)) {
	lister.wg.Add(1)
	go func() {
		defer lister.wg.Done()
		do(item)
	}()
}
