package main

import (
    "fmt"
    "time"
	"sync"
	"github.com/changlongH/go-timer"
)

func timerDrive() {
	msec := 2500 * time.Microsecond
loop:
	time.Sleep(msec)
	timer.UpdateTime()
	goto loop
}

func main() {
    ct := time.Now()
    fmt.Printf("++++++ system time. unix=%d, nano=%d\n", ct.Unix(), ct.UnixNano())

	var wg sync.WaitGroup
	wg.Add(2)

	timer.Init()
    fmt.Printf("startTime=%d,now=%d\n", timer.StartTime(), timer.Now())

	go timerDrive()

    //timer.Timeout(3, 1003, 2003)
    //smsec :=  time.Duration(5)*time.Millisecond

	go func() {
		times := 10
		for i:= 0; i<times; i++ {
			timer.Timeout(1, 1, 0)
			time.Sleep(10 * time.Millisecond)
		}
		wg.Done()
	}()

	go func() {
		times := 10
		for i:= 0; i<times; i++ {
			timer.Timeout(i, 2, 0)
		}
		wg.Done()
	}()

	fmt.Printf("Test Sleep Begin now=%d\n", timer.Now())
	time.Sleep(10*time.Second)
	fmt.Printf("Test Sleep Finish now=%d\n", timer.Now())
	wg.Wait()
    fmt.Printf("Test Timer Finish startTime=%d,now=%d\n", timer.StartTime(), timer.Now())
}
