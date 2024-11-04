# Concurrent Prime Factorization

A Go implementation of concurrent prime number factorization with configurable worker pools.

## Features

- Concurrent factorization with separate worker pools for processing and writing
- Configurable number of workers for optimal performance
- Early termination support via context cancellation
- Thread-safe writer integration
- Efficient O(√n) factorization algorithm

## Interface

```go
type Factorization interface {
    Do(done <-chan struct{}, numbers []int, writer io.Writer, config ...Config) error
}
```
## Usage example

```go
slice := []int{100, -17, 25}
done := make(chan struct{})
if err := fact.New().Do(done, slice, os.Stdout, fact.Config{	
    FactorizationWorkers: 2,
    WriteWorkers:         2,
}); err != nil {
    log.Fatal(err)
}
```

### Output format
Each number is factorized and written in the format:


```
25 = 5 * 5
-17 = -1 * 17
100 = 2 * 2 * 5 * 5
```

