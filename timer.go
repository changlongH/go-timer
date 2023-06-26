package timer

import (
	//"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	_ "unsafe" // for go:linkname
)

//go:linkname walltime runtime.walltime
func walltime() (sec int64, nsec int32)

//go:linkname nanotime runtime.nanotime
func nanotime() (mono int64)

const (
	nearShift  = 8
	nearSlot   = 1 << nearShift // 256 slot
	levelShift = 6
	levelSlot  = 1 << levelShift // 64 slot

	nearMask  = nearSlot - 1
	levelMask = levelSlot - 1

	// nsec/nsecNotation centisecond: 1/100 second(10ms)
	nsecNotation uint64 = 10000000
)

type (
	timerNode struct {
		expire uint32
		next   *timerNode

		// hard code: timeout args
		handle  uint32
		session int
	}

	linkList struct {
		head timerNode
		tail *timerNode
	}

	Timer struct {
		near      [nearSlot]linkList
		piles     [4][levelSlot]linkList
		lock      uint32 // spinlock flag
		time      uint32 // frame: (1/100 second) since process starts
		starttime uint32 // time stamp when the process starts(since the year 1970)
		current   uint64 // realtime (1/100 second)
		curPoint  uint64 // delay change current fix: current less than walltime
	}
)

var timeInstance *Timer = nil
var timerNodePool sync.Pool // use pool maybe better?

func getTime() uint64 {
	mono := uint64(nanotime())
	return mono / nsecNotation
}

// If the lock is already in use, the calling goroutine
// blocks until the locker is available.
func (T *Timer) Lock() {
loop:
	if !atomic.CompareAndSwapUint32(&T.lock, 0, 1) {
		runtime.Gosched()
		goto loop
	}
}

func (T *Timer) Unlock() {
	atomic.StoreUint32(&T.lock, 0)
}

func link(list *linkList, node *timerNode) {
	list.tail.next = node
	list.tail = node
	node.next = nil
}

func linkClear(list *linkList) *timerNode {
	ret := list.head.next
	list.head.next = nil
	list.tail = &list.head
	return ret
}

func addNode(T *Timer, node *timerNode) {
	expire := node.expire
	ct := T.time

	// 000000 000000 000000 000000 11111111 => slot [8, 1]
	if (expire | nearMask) == (ct | nearMask) {
		// high 24bit equal link near node
		link(&T.near[expire&nearMask], node)
	} else {
		// step1: 000000 000000 000000 111111 11111111 => slot [14, 9]
		// step2: 000000 000000 111111 111111 11111111 => slot [20, 15]
		// step3: 000000 111111 111111 111111 11111111 => slot [26, 21]
		mask := uint32(nearSlot << levelShift)
		var i int
		for i = 0; i < 3; i++ {
			if (expire | (mask - 1)) == (ct | (mask - 1)) {
				break
			}
			mask <<= levelShift
		}

		//high := expire>>(nearShift + i*levelShift)
		//slot := high & levelMask
		// if expire overflow then level=3(i) slot = 0(000000&111111)
		link(&T.piles[i][((expire>>(nearShift+i*levelShift))&levelMask)], node)
	}
}

func moveList(T *Timer, level int, idx int) {
	node := linkClear(&T.piles[level][idx])
	for node != nil {
		temp := node.next
		addNode(T, node)
		node = temp
	}
}

func timerShift(T *Timer) {
	mask := nearSlot
	T.time++
	ct := T.time
	if ct == 0 {
		moveList(T, 3, 0) // overflow 497day
	} else {
		htime := ct >> nearShift // cache high time
		i := 0

		// step 1(near 255): 000000 000000 000000 000000 11111111 & ct
		// step 2(level 0): 000000 000000 000000 111111 11111111 & ct
		// step 3(level 1): 000000 000000 111111 111111 11111111 & ct
		// step 4(level 2): 000000 111111 111111 111111 11111111 & ct
		// step 5(level 3): 111111 111111 111111 111111 11111111 & ct
		for (ct & uint32(mask-1)) == 0 {
			idx := int(htime & levelMask)
			if idx != 0 {
				moveList(T, i, idx)
				break
			}
			mask <<= levelShift
			htime >>= levelShift
			i++
		}
	}
}

func releaseTimerNode(node *timerNode) {
	node.next = nil
	timerNodePool.Put(node)
}

func acquireTimerNode() *timerNode {
	node := timerNodePool.Get()
	if node == nil {
		return &timerNode{}
	}
	return node.(*timerNode)
}

func dispatchList(node *timerNode) {
	var temp *timerNode
	for {
		// TODO trigger event
		temp = node
		node = node.next
		releaseTimerNode(temp)
		if node == nil {
			break
		}
	}
}

func timerExecute(T *Timer) {
	idx := T.time & nearMask

	var node *timerNode
	for T.near[idx].head.next != nil {
		node = linkClear(&T.near[idx])
		// dispatch list don't need lock T
		T.Unlock()
		dispatchList(node)
		T.Lock()
	}
}

func timerUpdate(T *Timer) {
	T.Lock()

	// try to dispatch timeout 0 (rare condition)
	timerExecute(T)

	// shift time first, and then dispatch timer message
	timerShift(T)

	timerExecute(T)

	T.Unlock()
}

func createTimer() *Timer {
	r := new(Timer)

	for i := 0; i < nearSlot; i++ {
		linkClear(&r.near[i])
	}

	for i := 0; i < 4; i++ {
		for j := 0; j < levelSlot; j++ {
			linkClear(&r.piles[i][j])
		}
	}

	r.lock = 0
	r.current = 0
	return r
}

func timerAdd(T *Timer, duration int, handle uint32, session int) {
	node := acquireTimerNode()
	node.handle = handle
	node.session = session

	T.Lock()

	node.expire = T.time + uint32(duration)
	addNode(T, node)

	T.Unlock()
}

func Timeout(duration int, handle uint32, session int) int {
	if duration <= 0 {
		//TODO dispatch now
		duration = 0
	}
	timerAdd(timeInstance, duration, handle, session)
	return session
}

func UpdateTime() {
	cp := getTime()
	if cp < timeInstance.curPoint {
		//trow exception errors.New("change from " + cp + " to "+ timeInstance.curPoint)
		timeInstance.curPoint = cp
	} else if cp != timeInstance.curPoint {
		diff := cp - timeInstance.curPoint
		timeInstance.curPoint = cp
		timeInstance.current += diff
		for i := 0; i < int(diff); i++ {
			timerUpdate(timeInstance)
		}
	}
}

func StartTime() uint32 {
	return timeInstance.starttime
}

func Now() uint64 {
	return timeInstance.current
}

// init unsafe for use by multiple goroutines
func Init() {
	timeInstance = createTimer()
	sec, nsec := walltime()

	timeInstance.starttime = uint32(sec)
	timeInstance.current = uint64(nsec) / nsecNotation
	timeInstance.curPoint = getTime()
}
