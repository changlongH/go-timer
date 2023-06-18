package timer

import (
    "fmt"
)

func Timeout(handle uint32, time int, session int) int {
    if time < 0 {
        return -1
    }else {
        return 0
    }
    return session
}

func UpdateTime() {
}

func StartTime() uint32 {
	return 0
}

func Now() uint64 {
	return 0
}

func Init() {
	fmt.Println("Init Timer")
}

// for profile
func ThreadTime() uint64 {
	return 0
}
