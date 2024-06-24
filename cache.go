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
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

var (
	cache = make(map[string]*CacheTable)
	mutex sync.RWMutex
)

func Init(ctx context.Context) {
	go func() {
		gcTicker := time.NewTicker(4 * time.Second)
		for {
			select {
			case <-ctx.Done():
				gcTicker.Stop()
				return
			case <-gcTicker.C:
				//fmt.Printf("[before] %s\n", traceMemStats())
				runtime.GC()
				debug.FreeOSMemory()
				//fmt.Printf("[after] %s\n", traceMemStats())
				fmt.Println(time.Now().Unix(), "GC")
				time.Sleep(5 * time.Second)
			}
		}
	}()

	fmt.Println(time.Now().Unix(), "运行")
}

// Cache returns the existing cache table with given name or creates a new one
// if the table does not exist yet.
func Cache(ctx context.Context, table string, shardNum int, cleanInterval time.Duration) *CacheTable {
	mutex.RLock()
	t, ok := cache[table]
	mutex.RUnlock()

	if !ok {
		mutex.Lock()
		t, ok = cache[table]
		// Double check whether the table exists or not.
		if !ok {
			t = &CacheTable{
				name:            table,
				hash:            newDefaultHasher(),
				L1Shards:        make(shardItems, shardNum),
				L2Shards:        make(shardItems, shardNum),
				shardMask:       uint64(shardNum - 1),
				cleanupInterval: cleanInterval,
			}

			for i := 0; i < shardNum; i++ {
				t.L1Shards[i] = newShardItem()
				t.L2Shards[i] = newShardItem()
			}

			// 定时清理过期缓存
			go func(t *CacheTable, ctx context.Context) {
				ticker := time.NewTicker(cleanInterval)
				reBuildTicker := time.NewTicker(30 * time.Minute)
				for {
					select {
					case <-ctx.Done():
						ticker.Stop()
						reBuildTicker.Stop()
						return
					case <-ticker.C:
						fmt.Println(t.name, time.Now().Unix(), "clean-before")
						t.Lock()
						// 扫描需要删除的key
						var deleteList []*CacheItem

						// 先处理l1，再处理l2
						t.switchMask = 1 << 1
						now := time.Now()

						// 处理l1
						// 不允许l1读写入，读写通过l2

						fmt.Println(t.name, time.Now().Unix(), "l1mask-before")
						for {
							if atomic.LoadInt32(&t.l1Mask) == 0 {
								break
							}
						}
						fmt.Println(t.name, time.Now().Unix(), "l1mask-after")

						fmt.Println(t.name, time.Now().Unix(), "l1 - delete-before")
						for i, sad := range t.L1Shards {
							c := 0
							sad.lock.RLock()
							for _, r := range sad.m {
								if now.Sub(r.createdOn).Seconds() > r.lifeSpan.Seconds() {
									deleteList = append(deleteList, r)
								}
								c++
							}
							sad.lock.RUnlock()
							fmt.Println(t.name, time.Now().Unix(), fmt.Sprintf("l1-shards[%d] [len=%d]", i, c))
						}
						fmt.Println(t.name, time.Now().Unix(), "l1 - delete-middle")
						// 开始删除
						for _, item := range deleteList {
							t.L1Shards[item.hashedKey&t.shardMask].lock.Lock()
							delete(t.L1Shards[item.hashedKey&t.shardMask].m, item.key)
							t.L1Shards[item.hashedKey&t.shardMask].lock.Unlock()
						}
						fmt.Println(t.name, time.Now().Unix(), "l1 - delete-after")

						deleteList = make([]*CacheItem, 0)

						// 处理l2
						t.switchMask = 1 << 2

						// 堵塞的item加回来
						l1Length := len(t.l1BlockChan)
						for _, item := range t.l1BlockChan {
							//fmt.Println(t.name, t.L1Shards[item.hashedKey&t.shardMask])
							if item != nil {
								t.L1Shards[item.hashedKey&t.shardMask].lock.Lock()
								t.L1Shards[item.hashedKey&t.shardMask].m[item.key] = item
								t.L1Shards[item.hashedKey&t.shardMask].lock.Unlock()
							}
						}

						// 重置l1BlockChan
						t.l1BlockChan = make([]*CacheItem, 0, l1Length/2)

						fmt.Println(t.name, time.Now().Unix(), "l2mask-before")
						// 不允许l2读写入，读写通过l1
						for {
							if atomic.LoadInt32(&t.l2Mask) == 0 {
								break
							}
						}
						fmt.Println(t.name, time.Now().Unix(), "l2mask-after")

						fmt.Println(t.name, time.Now().Unix(), "l2 - delete-before")
						for i, sad := range t.L2Shards {
							c := 0
							sad.lock.RLock()
							for _, r := range sad.m {
								if now.Sub(r.createdOn).Seconds() > r.lifeSpan.Seconds() {
									deleteList = append(deleteList, r)
								}
								c++
							}
							sad.lock.RUnlock()
							fmt.Println(t.name, time.Now().Unix(), fmt.Sprintf("l2-shards[%d] [len=%d]", i, c))
						}
						fmt.Println(t.name, time.Now().Unix(), "l2 - delete-middle")

						// 开始删除
						for _, item := range deleteList {
							t.L2Shards[item.hashedKey&t.shardMask].lock.Lock()
							delete(t.L2Shards[item.hashedKey&t.shardMask].m, item.key)
							t.L2Shards[item.hashedKey&t.shardMask].lock.Unlock()
						}
						fmt.Println(t.name, time.Now().Unix(), "l2 - delete-after")

						// 恢复正常
						t.switchMask = 1 >> 1

						fmt.Println(t.name, time.Now().Unix(), "l2-b - add-before")

						l2Length := len(t.l2BlockChan)
						for _, item := range t.l2BlockChan {
							//fmt.Println(t.name, t.L1Shards[item.hashedKey&t.shardMask])
							if item != nil {
								t.L2Shards[item.hashedKey&t.shardMask].lock.Lock()
								t.L2Shards[item.hashedKey&t.shardMask].m[item.key] = item
								t.L2Shards[item.hashedKey&t.shardMask].lock.Unlock()
							}
						}

						fmt.Println(t.name, time.Now().Unix(), "l2-b - add-after")

						// 重置l2BlockChan
						t.l2BlockChan = make([]*CacheItem, 0, l2Length/2)

						t.Unlock()
						fmt.Println(t.name, time.Now().Unix(), "clean-after")

					case <-reBuildTicker.C:
						fmt.Println(t.name, time.Now().Unix(), "rebuild-before")
						t.Lock()
						// 为了释放map内存

						// 先处理l1，再处理l2
						t.switchMask = 1 << 1
						now := time.Now()

						// 处理l1
						// 不允许l1读写入，读写通过l2
						fmt.Println(t.name, time.Now().Unix(), "l1-rebuild-before")
						for {
							if atomic.LoadInt32(&t.l1Mask) == 0 {
								break
							}
						}

						fmt.Println(t.name, time.Now().Unix(), "l1-rebuild-after")

						for _, sad := range t.L1Shards {
							sad.lock.Lock()
							nm := make(shard, len(sad.m))
							for key, r := range sad.m {
								if now.Sub(r.createdOn).Seconds() < r.lifeSpan.Seconds() {
									nm[key] = r
								}
							}
							sad.m = nil
							sad.m = nm
							sad.lock.Unlock()
						}

						fmt.Println(t.name, time.Now().Unix(), "l2-rebuild-before")
						// 先处理l1，再处理l2
						t.switchMask = 1 << 2
						for {
							if atomic.LoadInt32(&t.l2Mask) == 0 {
								break
							}
						}

						fmt.Println(t.name, time.Now().Unix(), "l2-rebuild-after")

						for _, sad := range t.L2Shards {
							sad.lock.Lock()
							nm := make(shard, len(sad.m))
							for key, r := range sad.m {
								if now.Sub(r.createdOn).Seconds() < r.lifeSpan.Seconds() {
									nm[key] = r
								}
							}
							sad.m = nil
							sad.m = nm
							sad.lock.Unlock()
						}

						// 恢复正常
						t.switchMask = 1 >> 1

						fmt.Println(t.name, time.Now().Unix(), "gc")
						runtime.GC()
						fmt.Println(t.name, time.Now().Unix(), "release")
						debug.FreeOSMemory()
						t.Unlock()
						fmt.Println(t.name, time.Now().Unix(), "rebuild-after")
					}
				}
			}(t, ctx)

			cache[table] = t
		}
		mutex.Unlock()
	}

	return t
}
