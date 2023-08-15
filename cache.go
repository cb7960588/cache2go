/*
 * Simple caching library with expiration capabilities
 *     Copyright (c) 2012, Radu Ioan Fericean
 *                   2013-2017, Christian Muehlhaeuser <muesli@gmail.com>
 *
 *   For license see LICENSE.txt
 */

package cache2go

import (
	"context"
	"sync"
	"time"
)

var (
	cache = make(map[string]*CacheTable)
	mutex sync.RWMutex
)

// Cache returns the existing cache table with given name or creates a new one
// if the table does not exist yet.
func Cache(ctx context.Context, table string, shardNum int) *CacheTable {
	mutex.RLock()
	t, ok := cache[table]
	mutex.RUnlock()

	if !ok {
		mutex.Lock()
		t, ok = cache[table]
		// Double check whether the table exists or not.
		if !ok {
			t = &CacheTable{
				name:               table,
				expireByCreateTime: true,
				hash:               newDefaultHasher(),
				shards:             make([]shard, shardNum),
				shardLock:          make([]sync.RWMutex, shardNum),
				shardMask:          uint64(shardNum - 1),
				deleteChan:         make(chan interface{}, 1024*10*5),
			}

			// 定时清理过期缓存
			go func(t *CacheTable, c context.Context) {
				ticker := time.NewTicker(10 * time.Second)
				for {
					select {
					case <-ctx.Done():
						ticker.Stop()
					case <-ticker.C:
						for i, sad := range t.shards {
							t.shardLock[i].RLock()
							for key, r := range sad {
								if time.Now().Sub(r.createdOn) > r.lifeSpan {
									t.deleteChan <- key
								}
							}
							t.shardLock[i].RUnlock()
						}
					}
				}
			}(t, ctx)

			// 作用于清理过期缓存
			go func(t *CacheTable, c context.Context) {
				for {
					select {
					case <-ctx.Done():
						t.Stop()
					case key := <-t.deleteChan:
						t.deleteInternal(key)
					}
				}
			}(t, ctx)

			cache[table] = t
		}
		mutex.Unlock()
	}

	return t
}
