# Performance Benchmarks

Benchmark results for the in-memory store in `ttl-kv-service`.

All numbers are measured, not estimated. Results vary across runs due to OS
scheduling, thermal throttling and background processes.

## Environment

| Detail | Value |
| :--- | :--- |
| OS | Windows 11 (10.0.26300.0, amd64) |
| CPU | 12th Gen Intel Core i5-1235U, 12 logical cores |
| Go | go1.27.3 windows/amd64 |
| Benchmark framework | Go `testing.B` (`-benchmem -count=3`) |
| Date | November 2026 |

> **Note:** This is a development laptop, not a dedicated benchmark host.
> Numbers reflect relative performance between operations, not production
> capacity claims.

## How to reproduce

```powershell
go test -run='^$' -bench='.' -benchmem -count=3 ./internal/store
```

## Results

Raw output from `go test -bench=. -benchmem -count=3`:

```text
goos: windows
goarch: amd64
pkg: github.com/ROHAN-BHUTANI/ttl-kv-service/internal/store
cpu: 12th Gen Intel(R) Core(TM) i5-1235U
BenchmarkStoreSet-12               7151485    185.9 ns/op  1376.97 MB/s   256 B/op   1 allocs/op
BenchmarkStoreSet-12               6830071    147.6 ns/op  1733.99 MB/s   256 B/op   1 allocs/op
BenchmarkStoreSet-12               5543011    208.5 ns/op  1227.73 MB/s   256 B/op   1 allocs/op
BenchmarkStoreGet-12               8888203    178.9 ns/op  1430.58 MB/s   256 B/op   1 allocs/op
BenchmarkStoreGet-12               7008170    183.8 ns/op  1392.53 MB/s   256 B/op   1 allocs/op
BenchmarkStoreGet-12               9004202    150.2 ns/op  1704.03 MB/s   256 B/op   1 allocs/op
BenchmarkStoreDelete-12            5413624    304.1 ns/op                    0 B/op   0 allocs/op
BenchmarkStoreDelete-12            5545239    296.6 ns/op                    0 B/op   0 allocs/op
BenchmarkStoreDelete-12            5277183    323.3 ns/op                    0 B/op   0 allocs/op
BenchmarkStoreKeys/Size10-12       2456094    499.6 ns/op                  160 B/op   1 allocs/op
BenchmarkStoreKeys/Size10-12       2861007    491.4 ns/op                  160 B/op   1 allocs/op
BenchmarkStoreKeys/Size10-12       2536880    512.5 ns/op                  160 B/op   1 allocs/op
BenchmarkStoreKeys/Size100-12       153763   9173   ns/op                 1792 B/op   1 allocs/op
BenchmarkStoreKeys/Size100-12       155086   7742   ns/op                 1792 B/op   1 allocs/op
BenchmarkStoreKeys/Size100-12       158473   8056   ns/op                 1792 B/op   1 allocs/op
BenchmarkStoreKeys/Size1000-12        8787 121099   ns/op                16384 B/op   1 allocs/op
BenchmarkStoreKeys/Size1000-12        9722 123104   ns/op                16384 B/op   1 allocs/op
BenchmarkStoreKeys/Size1000-12       10389 114303   ns/op                16384 B/op   1 allocs/op
BenchmarkStoreParallel-12          2439248    539.8 ns/op   474.27 MB/s   256 B/op   1 allocs/op
BenchmarkStoreParallel-12          2121207    565.0 ns/op   453.09 MB/s   256 B/op   1 allocs/op
BenchmarkStoreParallel-12          2174382    526.4 ns/op   486.30 MB/s   256 B/op   1 allocs/op
BenchmarkStoreMixedWorkload-12     2316513    469.3 ns/op   545.51 MB/s   194 B/op   0 allocs/op
BenchmarkStoreMixedWorkload-12     2191384    537.0 ns/op   476.76 MB/s   193 B/op   0 allocs/op
BenchmarkStoreMixedWorkload-12     2486002    507.1 ns/op   504.80 MB/s   192 B/op   0 allocs/op
```

## Summary

| Benchmark | Median latency | Allocs | Notes |
| :--- | :--- | :--- | :--- |
| Set (256 B value) | ~180 ns/op | 256 B / 1 alloc | Defensive copy of value slice |
| Get (256 B value) | ~179 ns/op | 256 B / 1 alloc | Defensive copy on read |
| Delete | ~304 ns/op | 0 B / 0 allocs | Pure map delete under write lock |
| Keys (10 keys) | ~500 ns/op | 160 B / 1 alloc | Slice + sort.Strings |
| Keys (100 keys) | ~8.3 µs/op | 1792 B / 1 alloc | O(N log N) sort dominates |
| Keys (1000 keys) | ~121 µs/op | 16384 B / 1 alloc | Linear in number of keys |
| Parallel (12 goroutines) | ~544 ns/op | 256 B / 1 alloc | 80% reads, 20% writes |
| Mixed workload (12 goroutines) | ~504 ns/op | ~193 B / 0 allocs | 80% R / 15% W / 5% D |

## Interpretation

1. **The 256 B/op allocation in Set and Get** is the intentional defensive byte
   copy (`append([]byte(nil), value...)`). This prevents callers from mutating
   stored data through retained slice references.

2. **Delete allocates nothing** — it only acquires the write lock and removes
   the map entry.

3. **Keys scales as O(N log N)** because of `sort.Strings`. The single
   allocation is the pre-sized `[]string` slice.

4. **Parallel throughput** drops compared to sequential because all goroutines
   contend on a single `sync.RWMutex`. This is the expected tradeoff of a
   single-lock design chosen for simplicity and correctness.

## Limitations

- These benchmarks measure the store layer in isolation, not HTTP round-trip
  latency.
- A laptop under variable thermal and power conditions is not a stable
  benchmarking environment. Results should be compared across runs, not treated
  as absolute throughput guarantees.
- Sharding the map into multiple lock-guarded buckets would reduce contention
  under heavy parallel writes but adds complexity.
