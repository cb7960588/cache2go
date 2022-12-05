/*
 * Simple caching library with expiration capabilities
 *     Copyright (c) 2013-2017, Christian Muehlhaeuser <muesli@gmail.com>
 *
 *   For license see LICENSE.txt
 */

package cache2go

import (
	"testing"
	"time"
)

var (
	k = "testkey"
	v = "testvalue"
)

func TestCacheNew(t *testing.T) {
	table := Cache("testCacheNew", true)
	table.Add(k+"_1", 10*time.Second, v)

}
