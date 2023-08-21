/*
 * Simple caching library with expiration capabilities
 *     Copyright (c) 2013-2017, Christian Muehlhaeuser <muesli@gmail.com>
 *
 *   For license see LICENSE.txt
 */

package cache2go

import (
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type shard map[interface{}]*CacheItem

type shardItem struct {
	m    map[interface{}]*CacheItem
	lock sync.RWMutex
}

func newShardItem() *shardItem {
	return &shardItem{
		m: make(shard),
	}
}

type shardItems []*shardItem

// CacheTable is a table within the cache
type CacheTable struct {
	sync.RWMutex

	hash *fnv64a

	// The table's name.
	name string

	L1Shards  shardItems
	L2Shards  shardItems
	shardMask uint64

	// Timer responsible for triggering cleanup.
	cleanupTimer *time.Timer
	// Current timer duration.
	cleanupInterval time.Duration

	// The logger used for this table.
	logger *log.Logger

	l1BlockChan []*CacheItem // key
	l2BlockChan []*CacheItem // key
	isStop      bool

	l1Mask int32
	l2Mask int32

	switchMask uint8
}

// SetLogger sets the logger to be used by this cache table.
func (table *CacheTable) SetLogger(logger *log.Logger) {
	table.Lock()
	defer table.Unlock()
	table.logger = logger
}

//// Delete an item from the cache.
//func (table *CacheTable) Delete(key interface{}) (*CacheItem, error) {
//	keyBytes, _ := json.Marshal(key)
//	hashedKey := globalHasher.Sum64(Bytes2String(keyBytes))
//
//}

func (table *CacheTable) Add(key interface{}, lifeSpan time.Duration, data interface{}) *CacheItem {
	item := NewCacheItem(key, lifeSpan, data)

	if table.switchMask != 1<<1 {
		atomic.AddInt32(&table.l1Mask, 1)
		defer atomic.AddInt32(&table.l1Mask, -1)
		table.L1Shards[item.hashedKey&table.shardMask].lock.Lock()
		table.L1Shards[item.hashedKey&table.shardMask].m[item.key] = item
		table.L1Shards[item.hashedKey&table.shardMask].lock.Unlock()
	} else {
		table.l1BlockChan = append(table.l1BlockChan, item)
	}

	if table.switchMask != 1<<2 {
		atomic.AddInt32(&table.l2Mask, 1)
		defer atomic.AddInt32(&table.l2Mask, -1)
		table.L2Shards[item.hashedKey&table.shardMask].lock.Lock()
		table.L2Shards[item.hashedKey&table.shardMask].m[item.key] = item
		table.L2Shards[item.hashedKey&table.shardMask].lock.Unlock()
	} else {
		table.l2BlockChan = append(table.l2BlockChan, item)
	}

	return item
}

func (table *CacheTable) Value(key interface{}, args ...interface{}) (*CacheItem, error) {
	keyBytes, _ := json.Marshal(key)
	hashedKey := table.hash.Sum64(string(keyBytes))
	var sm *shardItem
	if table.switchMask == 1>>1 {
		// 先查l1
		sm = table.L1Shards[hashedKey&table.shardMask]
		sm.lock.RLock()
		r, ok := sm.m[key]
		sm.lock.RUnlock()

		if ok {
			// 正常返回结果
			return r, nil
		}

		// 再查l2
		sm = table.L2Shards[hashedKey&table.shardMask]
		sm.lock.RLock()
		r, ok = sm.m[key]
		sm.lock.RUnlock()

		if ok {
			// 正常返回结果
			return r, nil
		}

		// 找不到key
		return nil, ErrKeyNotFound

	} else if table.switchMask == 1<<1 {
		// 正在处理l1，需要从l2读
		sm = table.L2Shards[hashedKey&table.shardMask]
		sm.lock.RLock()
		r, ok := sm.m[key]
		sm.lock.RUnlock()
		if ok {
			// 正常返回结果
			return r, nil
		}
		// 找不到key
		return nil, ErrKeyNotFound
	} else {
		// 正在处理l2，需要从l1读
		sm = table.L1Shards[hashedKey&table.shardMask]
		sm.lock.RLock()
		r, ok := sm.m[key]
		sm.lock.RUnlock()

		if ok {
			// 正常返回结果
			return r, nil
		}

		// 找不到key
		return nil, ErrKeyNotFound
	}
}

// Internal logging method for convenience.
func (table *CacheTable) log(v ...interface{}) {
	if table.logger == nil {
		return
	}

	table.logger.Println(v...)
}
