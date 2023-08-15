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
	"time"
	"unsafe"
)

type shard map[interface{}]*CacheItem

// CacheTable is a table within the cache
type CacheTable struct {
	sync.RWMutex

	hash *fnv64a

	// The table's name.
	name string
	// All cached items.
	//items map[interface{}]*CacheItem

	shards    []shard
	shardLock []sync.RWMutex
	shardMask uint64

	// Timer responsible for triggering cleanup.
	cleanupTimer *time.Timer
	// Current timer duration.
	cleanupInterval time.Duration

	// The logger used for this table.
	logger *log.Logger

	// Callback method triggered when adding a new item to the cache.
	addedItem []func(item *CacheItem)
	// expire check by createdtime
	expireByCreateTime bool

	deleteChan chan interface{} // key
	isStop     bool
}

// SetLogger sets the logger to be used by this cache table.
func (table *CacheTable) SetLogger(logger *log.Logger) {
	table.Lock()
	defer table.Unlock()
	table.logger = logger
}

func (table *CacheTable) addInternal(item *CacheItem) {
	table.log("Adding item with key", item.key, "and lifespan of", item.lifeSpan, "to table", table.name)
	keyBytes, _ := json.Marshal(item.key)
	hashedKey := table.hash.Sum64(Bytes2String(keyBytes))
	shardTable := table.getShard(hashedKey)
	lock := table.getShardLock(hashedKey)
	lock.Lock()
	shardTable[item.key] = item
	lock.Unlock()
}

// Add adds a key/value pair to the cache.
// Parameter key is the item's cache-key.
// Parameter lifeSpan determines after which time period without an access the item
// will get removed from the cache.
// Parameter data is the item's value.
func (table *CacheTable) Add(key interface{}, lifeSpan time.Duration, data interface{}) *CacheItem {
	item := NewCacheItem(key, lifeSpan, data)

	table.addInternal(item)

	return item
}

func (table *CacheTable) getShard(hashedKey uint64) (shard shard) {
	lock := table.getShardLock(hashedKey)
	lock.RLock()
	defer lock.RUnlock()

	return table.shards[hashedKey&table.shardMask]
}

func (table *CacheTable) getShardLock(hashedKey uint64) (lock *sync.RWMutex) {
	return &table.shardLock[hashedKey&table.shardMask]
}

func (table *CacheTable) deleteInternal(key interface{}) (*CacheItem, error) {
	keyBytes, _ := json.Marshal(key)
	hashedKey := table.hash.Sum64(Bytes2String(keyBytes))
	shardTable := table.getShard(hashedKey)
	lock := table.getShardLock(hashedKey)
	lock.RLock()
	r, ok := shardTable[key]
	lock.RUnlock()
	if !ok {
		return nil, ErrKeyNotFound
	}

	lock.Lock()
	defer lock.Unlock()
	table.log("Deleting item with key", key, "created on", r.createdOn, "and hit", r.accessCount, "times from table", table.name)
	delete(shardTable, key)

	return r, nil
}

// Delete an item from the cache.
func (table *CacheTable) Delete(key interface{}) (*CacheItem, error) {
	return table.deleteInternal(key)
}

// Value returns an item from the cache and marks it to be kept alive. You can
// pass additional arguments to your DataLoader callback function.
func (table *CacheTable) Value(key interface{}, args ...interface{}) (*CacheItem, error) {
	keyBytes, _ := json.Marshal(key)
	hashedKey := table.hash.Sum64(Bytes2String(keyBytes))
	shardTable := table.getShard(hashedKey)
	lock := table.getShardLock(hashedKey)
	lock.RLock()
	r, ok := shardTable[key]
	lock.RUnlock()

	if ok {
		// 过期删除元素
		if time.Now().Sub(r.createdOn) > r.lifeSpan {
			table.deleteChan <- key
			return nil, ErrKeyNotFound
		}

		// 正常返回结果
		return r, nil
	}

	// 找不到key
	return nil, ErrKeyNotFound
}

// Internal logging method for convenience.
func (table *CacheTable) log(v ...interface{}) {
	if table.logger == nil {
		return
	}

	table.logger.Println(v...)
}

func (table *CacheTable) Stop() {
	table.Lock()
	defer table.Unlock()
	if !table.isStop {
		close(table.deleteChan)
		table.isStop = true
	}
}

func Bytes2String(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
