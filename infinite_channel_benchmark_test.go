package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"
)

func writeResultsToFile(fileName string, data map[string]int) {
	file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("failed to open file: %s", err)
	}

	defer file.Close()

	// writing the header
	_, err = file.WriteString("Size,MessagesToSend,DroppedMessages\n")
	if err != nil {
		log.Fatalf("failed to write to file: %s", err)
	}

	for key, value := range data {
		line := fmt.Sprintf("%s,%d\n", key, value)
		_, err := file.WriteString(line)
		if err != nil {
			log.Fatalf("failed to write to file: %s", err)
		}
	}
}

func runBenchmark(size, messagesToSend int, results map[string]int, mu *sync.Mutex) {
	dm := 0
	for i := 0; i < 100; i++ {
		ic := NewInfiniteChannel[*int](size, 0)
		// generate messages
		for j := 0; j < messagesToSend; j++ {
			if added := ic.Add(&j); !added {
				dm++
			}
			time.Sleep(10 * time.Microsecond)
		}
		ic.ForceStop()
	}
	avgDroppedMessages := dm / 100
	key := fmt.Sprintf("%d,%d", size, messagesToSend)
	mu.Lock()
	results[key] = avgDroppedMessages
	mu.Unlock()
}

func BenchmarkInfiniteChannel(b *testing.B) {
	results := make(map[string]int)
	var mu sync.Mutex

	for size := 0; size <= 8; size += 1 {
		for messagesToSend := 64; messagesToSend <= 512; messagesToSend += 64 {
			size := size
			k := messagesToSend
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					runBenchmark(size, k, results, &mu)
				}
			})

			fmt.Printf("benchmark for size %d and messages to send %d completed\n", size, messagesToSend)
		}
	}
	writeResultsToFile("benchmark_results.csv", results)
}

// func main() {
// 	b := testing.Benchmark(BenchmarkInfiniteChannel)
// 	fmt.Println(b)
// }
