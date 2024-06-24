/*
 * Simple caching library with expiration capabilities
 *     Copyright (c) 2013-2017, Christian Muehlhaeuser <muesli@gmail.com>
 *
 *   For license see LICENSE.txt
 */

package cache2go

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"
)

var (
	shardNum      = 1024
	cleanInterval = 5 * time.Second
	k             = "testkey"
	v             = "testvalue"
)

func TestCacheNew(t *testing.T) {
	Init(context.Background())
	table := Cache(context.Background(), "testCacheNew", shardNum, cleanInterval)
	var wg sync.WaitGroup
	for i := 0; i < 99999999/3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			table.Add(k+"_"+strconv.Itoa(i), 5*time.Second, v)
		}(i)
	}
	for i := 99999999 / 3; i > 0; i-- {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			table.Add(k+"_"+strconv.Itoa(i), 5*time.Second, v)
		}(i)
	}

	wg.Wait()
}

/*
go test -bench=. -run=none                                                                                                                                                    ░▒▓ ✔  system   at 10:35:12  
goos: darwin
goarch: amd64
pkg: github.com/cb7960588/cache2go
cpu: Intel(R) Core(TM) i7-8750H CPU @ 2.20GHz
BenchmarkCacheNew-12              644256              2137 ns/op
PASS
ok      github.com/cb7960588/cache2go   2.216s
*/
func BenchmarkCacheNew(b *testing.B) {
	b.ResetTimer()
	table := Cache(context.Background(), "testCacheNew", shardNum, cleanInterval)

	for i := 0; i < b.N; i++ {
		key := k + "_1" + strconv.Itoa(i)
		table.Add(key, 10*time.Second, v)

		if _, err := table.Value(key); err != nil {
			b.Fatalf("err:%v", err)
		}

		if _, err := table.Value(key); err != nil {
			b.Fatalf("err:%v", err)
		}
	}
	b.StopTimer()
}

/**
[before]
go test -bench=CacheNewParallel -run=none
goos: darwin
goarch: amd64
pkg: github.com/cb7960588/cache2go
cpu: Intel(R) Core(TM) i7-8750H CPU @ 2.20GHz
BenchmarkCacheNewParallel-12             1079112              1262 ns/op
PASS
ok      github.com/cb7960588/cache2go   3.125s


go test -bench=CacheNewParallel -run=none -benchtime=8s                                                                                                                             ░▒▓ ✔  system   at 11:40:30  
goos: darwin
goarch: amd64
pkg: github.com/cb7960588/cache2go
cpu: Intel(R) Core(TM) i7-8750H CPU @ 2.20GHz
BenchmarkCacheNewParallel-12             9048769              7054 ns/op
PASS
ok      github.com/cb7960588/cache2go   65.677s


[after]
go test -bench=CacheNewParallel -run=none                                                                                                                         ░▒▓ ✔  took 5s   system   at 11:32:08  
goos: darwin
goarch: amd64
pkg: github.com/cb7960588/cache2go
cpu: Intel(R) Core(TM) i7-8750H CPU @ 2.20GHz
BenchmarkCacheNewParallel-12             3694575               336.5 ns/op
PASS
ok      github.com/cb7960588/cache2go   2.136s


 go test -bench=CacheNewParallel -run=none -benchtime=8s                                                                                                         ░▒▓ ✔  took 8s   system   at 11:39:28  
goos: darwin
goarch: amd64
pkg: github.com/cb7960588/cache2go
cpu: Intel(R) Core(TM) i7-8750H CPU @ 2.20GHz
BenchmarkCacheNewParallel-12            31105671               413.2 ns/op
PASS
ok      github.com/cb7960588/cache2go   13.565s

[after: clean-10s]
go test -bench=CacheNewParallel -run=none -benchtime=20s                                                                                                       ░▒▓ ✔  took 15s   system   at 11:59:02  
goos: darwin
goarch: amd64
pkg: github.com/cb7960588/cache2go
cpu: Intel(R) Core(TM) i7-8750H CPU @ 2.20GHz
BenchmarkCacheNewParallel-12            81961603               453.3 ns/op
PASS
ok      github.com/cb7960588/cache2go   38.620s

[after: clean-5s]
go test -bench=CacheNewParallel -run=none -benchtime=20s                                                                                                           ░▒▓ 1 ✘  took 8s   system   at 14:29:18  
goos: darwin
goarch: amd64
pkg: github.com/cb7960588/cache2go
cpu: Intel(R) Core(TM) i7-8750H CPU @ 2.20GHz
BenchmarkCacheNewParallel-12            71451643               466.4 ns/op
PASS
ok      github.com/cb7960588/cache2go   34.394s

*/
func BenchmarkCacheNewParallel(b *testing.B) {
	b.ResetTimer()
	table := Cache(context.Background(), "testCacheNew", shardNum, cleanInterval)

	b.RunParallel(func(pb *testing.PB) {
		i := 1
		for pb.Next() {
			key := k + "_1" + strconv.Itoa(i)
			table.Add(key, 3*time.Second, v)

			if _, err := table.Value(key); err != nil {
				//b.Fatalf("err:%v", err)
			}

			if _, err := table.Value(key); err != nil {
				//b.Fatalf("err:%v", err)
			}
			i++
		}
	})

	b.StopTimer()
}
