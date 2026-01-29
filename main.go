package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup

	// Create a chain of unbuffered channels. channels[i] will be used by goroutine i to signal goroutine i+1.
	// channels[0] is used to start the first goroutine.
	channels := make([]chan struct{}, 6) // Need 5 channels for signals between 5 goroutines, plus one to start the first.
	for i := range channels {
		channels[i] = make(chan struct{})
	}

	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			<-channels[id-1] // Wait for the signal from the previous stage (or the initial signal for id=1)
			fmt.Println(id)

			if id < 5 {
				channels[id] <- struct{}{} // Signal the next stage
			}
		}(i)
	}

	channels[0] <- struct{}{} // Start the first goroutine by sending an initial signal

	wg.Wait()
}
