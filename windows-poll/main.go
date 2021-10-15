package main

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	bufferSizeMin = 1 << 10   // KiB
	bufferSizeMax = 512 << 10 // KiB

	parallelism          = 8
	numberWrittenBuffers = 8
	connectionTimeout    = 50 * time.Millisecond
)

func randBuffer(reuseBuffer []byte) []byte {
	size := bufferSizeMin + rand.Intn(bufferSizeMax-bufferSizeMin)
	return sizedBuffer(reuseBuffer, size)
}

func sizedBuffer(reuseBuffer []byte, size int) []byte {
	// Fill the entire buffer with "size size size ...".
	// This helps debug things and notice corruption.
	sizePattern := strconv.AppendInt(reuseBuffer[:0], int64(size), 10)
	sizePattern = append(sizePattern, ' ')

	buf := reuseBuffer[:size]
	for done := len(sizePattern); done < len(buf); {
		done += copy(buf[done:], sizePattern)
	}
	return buf
}

func main() {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	addr := ln.Addr().String()
	println("addr:", addr)
	println("bufferSizeMin:", bufferSizeMin)
	println("bufferSizeMax:", bufferSizeMax)
	println("parallelism:", parallelism)
	println("numberWrittenBuffers:", numberWrittenBuffers)
	println()

	var atomicFinished uint64
	var atomicCanceled uint64

	for i := 0; i < parallelism; i++ {
		go func() {
			reuseBuffer := make([]byte, bufferSizeMax)
			for {
				conn, err := net.Dial("tcp", addr)
				if err != nil {
					panic(err)
				}
				if err := conn.SetDeadline(time.Now().Add(connectionTimeout)); err != nil {
					panic(err)
				}
				for i := 0; i < numberWrittenBuffers; i++ {
					buf := randBuffer(reuseBuffer)
					if n, err := conn.Write(buf); err != nil {
						if errors.Is(err, os.ErrDeadlineExceeded) {
							atomic.AddUint64(&atomicCanceled, 1)
							break
						} else {
							println(os.IsTimeout(err))
							panic(err)
						}
					} else if n != len(buf) {
						panic(fmt.Sprintf("n=%d len(buf)=%d", n, len(buf)))
					}
				}
				conn.Close()
			}
		}()
	}

	go func() {
		for range time.Tick(time.Second) {
			finished := atomic.LoadUint64(&atomicFinished)
			canceled := atomic.LoadUint64(&atomicCanceled)
			fmt.Fprintf(os.Stderr, "finished: %d canceled: %d\n", finished, canceled)
		}
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			panic(err)
		}
		if err := conn.SetDeadline(time.Now().Add(connectionTimeout)); err != nil {
			panic(err)
		}
		go func() {
			buf, err := io.ReadAll(conn)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					atomic.AddUint64(&atomicCanceled, 1)
					return
				} else {
					panic(err)
				}
			}
			min := numberWrittenBuffers * bufferSizeMin
			max := numberWrittenBuffers * bufferSizeMax
			if len(buf) < min || len(buf) > max {
				panic(fmt.Sprintf("size=%d min=%d max=%d", len(buf), min, max))
			}
			conn.Close()
			atomic.AddUint64(&atomicFinished, 1)
		}()
	}
}
