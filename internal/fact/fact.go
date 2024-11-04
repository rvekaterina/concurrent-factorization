package fact

import (
	"errors"
	"fmt"
	"io"
	"math"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

var (
	// ErrFactorizationCancelled is returned when the factorization process is cancelled via the done channel.
	ErrFactorizationCancelled = errors.New("cancelled")

	// ErrWriterInteraction is returned if an error occurs while interacting with the writer
	// triggering early termination.
	ErrWriterInteraction = errors.New("writer interaction")

	// ErrInvalidConfigFact is returned if config contains FactorizationWorkers <= 0
	ErrInvalidConfigFact = errors.New("invalid config FactorizationWorkers")

	// ErrInvalidConfigWrite is returned if config contains WriteWorkers <= 0
	ErrInvalidConfigWrite = errors.New("invalid config WriteWorkers")
)

// Config defines the configuration for factorization and write workers.
type Config struct {
	FactorizationWorkers int
	WriteWorkers         int
}

// Factorization interface represents a concurrent prime factorization task with configurable workers.
// Thread safety and error handling are implemented as follows:
// - The provided writer must be thread-safe to handle concurrent writes from multiple workers.
// - Output uses '\n' for newlines.
// - Factorization has a time complexity of O(sqrt(n)) per number.
// - If an error occurs while writing to the writer, early termination is triggered across all workers.
type Factorization interface {
	// Do performs factorization on a list of integers, writing the results to an io.Writer.
	// - done: a channel to signal early termination.
	// - numbers: the list of integers to factorize.
	// - writer: the io.Writer where factorization results are output.
	// - config: optional worker configuration.
	// Returns an error if the process is cancelled or if a writer error occurs.
	Do(done <-chan struct{}, numbers []int, writer io.Writer, config ...Config) error
}

// factorizationImpl provides an implementation for the Factorization interface.
type factorizationImpl struct{}

func New() *factorizationImpl {
	return &factorizationImpl{}
}

func generator(done <-chan struct{}, wg *sync.WaitGroup, errCh <-chan error, numbers []int) <-chan int {
	numCh := make(chan int)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(numCh)
		for _, i := range numbers {
			select {
			case <-errCh:
				return
			case <-done:
				return
			case numCh <- i:
			}
		}
	}()
	return numCh
}

func fact(done <-chan struct{}, result chan<- []string, errCh <-chan error, numbersCh <-chan int) {
	for num := range numbersCh {
		select {
		case <-done:
			return
		case <-errCh:
			return
		case result <- factorize(num):
		}
	}
}

func write(done <-chan struct{}, result <-chan []string, errCh chan<- error, writer io.Writer) {
	for res := range result {
		select {
		case <-done:
			return
		default:
			_, err := fmt.Fprintf(writer, "%s = %s\n", res[0], strings.Join(res[1:], " * "))
			if err != nil {
				select {
				case errCh <- errors.Join(err, ErrWriterInteraction):
				default:
				}
				return
			}
		}
	}
}

func factorize(n int) []string {
	factors := []string{strconv.Itoa(n)}
	if n == 1 || n == math.MinInt {
		return append(factors, factors[0])
	}
	if n < 0 {
		factors = append(factors, "-1")
		n = -n
	}
	for i := 2; i*i <= n; i++ {
		for ; n%i == 0; n /= i {
			factors = append(factors, strconv.Itoa(i))
		}
	}

	if n != 1 {
		factors = append(factors, strconv.Itoa(n))
	}
	return factors
}

func createConfig(config ...Config) Config {
	if len(config) > 0 {
		return config[0]
	}
	defaultWorkers := runtime.GOMAXPROCS(0)
	return Config{
		FactorizationWorkers: defaultWorkers,
		WriteWorkers:         defaultWorkers,
	}
}

func checkConfig(config Config) error {
	if config.FactorizationWorkers <= 0 {
		return ErrInvalidConfigFact
	}
	if config.WriteWorkers <= 0 {
		return ErrInvalidConfigWrite
	}
	return nil
}

func (f *factorizationImpl) Do(
	done <-chan struct{},
	numbers []int,
	writer io.Writer,
	config ...Config,
) error {
	cfg := createConfig(config...)
	err := checkConfig(cfg)
	if err != nil {
		return err
	}

	wg := &sync.WaitGroup{}
	errCh := make(chan error, 1)
	numbersCh := generator(done, wg, errCh, numbers)
	result := make(chan []string, cfg.FactorizationWorkers)

	wgFact := &sync.WaitGroup{}
	for factWorker := 0; factWorker < cfg.FactorizationWorkers && factWorker < len(numbers); factWorker++ {
		wgFact.Add(1)
		go func() {
			defer wgFact.Done()
			fact(done, result, errCh, numbersCh)
		}()
	}

	wgWrite := &sync.WaitGroup{}
	for writeWorker := 0; writeWorker < cfg.WriteWorkers && writeWorker < len(numbers); writeWorker++ {
		wgWrite.Add(1)
		go func() {
			defer wgWrite.Done()
			write(done, result, errCh, writer)
		}()
	}

	wg.Wait()
	wgFact.Wait()
	close(result)
	wgWrite.Wait()
	close(errCh)

	select {
	case <-done:
		return ErrFactorizationCancelled
	default:
		select {
		case err := <-errCh:
			return err
		default:
			return nil
		}
	}
}
