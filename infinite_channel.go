package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type internalQueue[T any] struct {
	arr  []T
	head int
	tail int
	size int
}

func newInternalyqueue[T any]() *internalQueue[T] {
	return &internalQueue[T]{
		arr: make([]T, 0),
	}
}

func (iq *internalQueue[T]) enqueue(item T) {
	iq.arr = append(iq.arr, item)
	iq.tail++
	iq.size++
}

func (iq *internalQueue[T]) dequeue() (T, bool) {
	var item T
	if iq.isEmpty() {
		return item, false
	}
	item = iq.arr[iq.head]
	iq.head++
	iq.size--

	if iq.head > len(iq.arr)/2 {
		iq.arr = iq.arr[iq.head:]
		iq.tail -= iq.head
		iq.head = 0
	}
	return item, true
}

func (iq *internalQueue[T]) peek() (T, bool) {
	var item T
	if iq.isEmpty() {
		return item, false
	}
	return iq.arr[iq.head], true
}

func (iq *internalQueue[T]) isEmpty() bool {
	return iq.size == 0
}

// InfiniteChannel represents an infinite channel for asynchronous processing.
type InfiniteChannel[T any] struct {
	iq              *internalQueue[T]
	producerChannel chan T
	consumerChannel chan T
	quit            chan struct{}
	isClosed        atomic.Value
	isForceStop     atomic.Value
	wg              *sync.WaitGroup
}

// NewInfiniteChannel initializes a new InfiniteChannel.
func NewInfiniteChannel[T any](producerChannelSize, consumerChannelSize int) *InfiniteChannel[T] {
	ic := &InfiniteChannel[T]{
		iq:              newInternalyqueue[T](),
		producerChannel: make(chan T, producerChannelSize),
		consumerChannel: make(chan T, consumerChannelSize),
		quit:            make(chan struct{}),
		wg:              &sync.WaitGroup{},
	}
	ic.isClosed.Store(false)
	ic.isForceStop.Store(false)

	ic.wg.Add(1)
	go ic.run()

	return ic
}

func (ic *InfiniteChannel[T]) run() {
	defer ic.wg.Done()
	for {
		select {
		case <-ic.quit:
			// allowing the consumers to know that the ic is closed
			ic.isClosed.Store(true)

			// close the producerChannel and make it point to nil
			close(ic.producerChannel)

			// empty the producerChannel into the internal queue
			ic.emptyItemsIntoInternalQueue()

			// empty the internal queue into the consumer channel. This two step is necessary to ensure the order
			toWait := !(ic.isForceStop.Load().(bool))
			ic.addItemsToConsumerChannel(toWait)

			// close the consumer Channel
			close(ic.consumerChannel)

			return

		case item, ok := <-ic.producerChannel:
			if ok {
				// add to the internal queue first
				ic.iq.enqueue(item)

				// try to empty the internal queue into the consumer Channel
				ic.addItemsToConsumerChannel(false)
			}
		}
	}
}

// Add adds an item to the InfiniteChannel.
func (ic *InfiniteChannel[T]) Add(item T) bool {
	select {
	case ic.producerChannel <- item:
		return true
	default:
		return false
	}

}

// // Take retrieves an item from the consumer channel. It blocks if the channel is empty.
// func (ic *InfiniteChannel[T]) Take() (T, bool) {
// 	item, ok := <-ic.consumerChannel
// 	return item, ok
// }

// TakeWithContext retrieves an item from the consumer channel. It does not block if the channel is empty.
func (ic *InfiniteChannel[T]) TakeWithContext(ctx context.Context) (T, bool) {
	select {
	case item := <-ic.consumerChannel:
		return item, ic.isClosed.Load().(bool)
	case <-ctx.Done():
		var item T
		return item, ic.isClosed.Load().(bool)
	}
}

// Stop stops the InfiniteChannel, ensuring all items are processed.
func (ic *InfiniteChannel[T]) Stop() {
	close(ic.quit)
	ic.wg.Wait()
}

// When there are no consumers to consume messages and the consumerChannel is full, Stop method blocks, to exit immediately
// use ForceStop method to discard all the remaining messages in the internal queue and also in the consumerChannel
func (ic *InfiniteChannel[T]) ForceStop() {
	ic.isForceStop.Store(true)
	close(ic.quit)
	ic.wg.Wait()
}

// emptyItemsIntoInternalQueue moves all items from the producer channel to the internal queue.
func (ic *InfiniteChannel[T]) emptyItemsIntoInternalQueue() {
	for item := range ic.producerChannel {
		ic.iq.enqueue(item)
	}
}

// addItemsToConsumerChannel moves items from the internal queue to the consumer channel.
func (ic *InfiniteChannel[T]) addItemsToConsumerChannel(toWait bool) {
	for !ic.iq.isEmpty() {
		item, _ := ic.iq.peek()
		select {
		case ic.consumerChannel <- item:
			ic.iq.dequeue()
		default:
			if !toWait {
				return
			}
		}
	}
}

func main() {
	// create infinite channel with producer channel size of 0 and consume channel size of 0
	ic := NewInfiniteChannel[*int](0, 0)
	pwg := &sync.WaitGroup{}
	pwg.Add(2)

	// producer one adding items to the infinite channel
	go func() {
		defer pwg.Done()
		droppedMessages := make([]*int, 0)
		for i := 0; i < 100; i++ {
			if added := ic.Add(&i); !added {
				droppedMessages = append(droppedMessages, &i)
			}
			time.Sleep(5 * time.Microsecond)
		}
		fmt.Println("dropped messages from producer 1", len(droppedMessages))
	}()

	// second producer adding items to the infinite channel
	go func() {
		defer pwg.Done()
		droppedMessages := make([]*int, 0)
		for i := 10; i < 100; i++ {
			if added := ic.Add(&i); !added {
				droppedMessages = append(droppedMessages, &i)
			}
			time.Sleep(5 * time.Microsecond)
		}
		fmt.Println("dropped messages from producer 2", len(droppedMessages))
	}()

	cwg := &sync.WaitGroup{}
	cwg.Add(1)

	// consumer reading items from the consumer channel
	go func() {
		defer cwg.Done()
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Millisecond)
			defer cancel()

			item, isClosed := ic.TakeWithContext(ctx)
			if item == nil && !isClosed {
				// Skip as context timeout happened but the channel is not closed
				continue
			}
			if isClosed && item == nil {
				fmt.Println("seems like the consumerChannel is closed", item, isClosed)
				break
			}

			fmt.Printf("Received with context %d\n", *item)
		}
	}()

	pwg.Wait()

	fmt.Println("stopping the infinite channel")
	ic.ForceStop()
	fmt.Println("infinite channel stopped successfully")

	fmt.Println("waiting for the consumer to finish")
	cwg.Done()
	fmt.Println("consumer exited")
}
