---
name: use-modern-go
description: Modern Go syntax guidelines for Go 1.22+. Use when writing or reviewing Go code.
keywords:
- go
- modern
- 1.22
- 1.23
- 1.24
- 1.25
- 1.26
- best-practice
---

## Go Version

This project targets **Go 1.26**. Use all features up to Go 1.26.

---

## Go 1.22+

```go
// cmp.Or: first non-zero value
name := cmp.Or(os.Getenv("NAME"), flagName, "default")

// Range over integer
for i := range n { ... }

// Enhanced ServeMux
mux.HandleFunc("GET /api/{id}", handler)
id := r.PathValue("id")
```

## Go 1.23+

```go
// Iterator patterns — iterate directly, no intermediate slice
for k := range maps.Keys(m)   { process(k) }
for v := range maps.Values(m) { process(v) }

// Collect when you need a slice
keys := slices.Collect(maps.Keys(m))
sortedKeys := slices.Sorted(maps.Keys(m))
```

## Go 1.24+

```go
// Always use t.Context() in tests
func TestFoo(t *testing.T) {
    ctx := t.Context()  // not context.WithCancel(context.Background())
}

// Always use omitzero for time.Time, time.Duration, structs, slices, maps
type Config struct {
    Timeout time.Duration `json:"timeout,omitzero"`
}

// Use b.Loop() in benchmarks
func BenchmarkFoo(b *testing.B) {
    for b.Loop() { doWork() }
}

// SplitSeq / FieldsSeq when iterating (no allocation)
for part := range strings.SplitSeq(s, ",") { process(part) }
```

## Go 1.25+

```go
// wg.Go() instead of wg.Add(1) + go func() { defer wg.Done() }
var wg sync.WaitGroup
for _, item := range items {
    wg.Go(func() { process(item) })
}
wg.Wait()
```

## Go 1.26+

```go
// new(val) returns pointer — type inferred
cfg := Config{Timeout: new(30), Debug: new(true)}

// errors.AsType[T] instead of errors.As + var declaration
if pathErr, ok := errors.AsType[*os.PathError](err); ok {
    handle(pathErr)
}
```