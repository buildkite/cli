package main

import (
	"context"
	"fmt"
	"time"
)

func contextProblems() {
	// context.Background() when it should be context.TODO()
	ctx := context.Background()

	// context not passed as first parameter
	doSomething("data", ctx)

	// time.Sleep in production code
	time.Sleep(5 * time.Second)
}

func doSomething(data string, ctx context.Context) {
	fmt.Println(data, ctx)
}

func sliceProblems() {
	// slice declaration that could be make
	var s []int
	s = append(s, 1, 2, 3)

	// inefficient slice initialization
	items := []string{}
	for i := 0; i < 100; i++ {
		items = append(items, fmt.Sprintf("item%d", i))
	}

	// range loop issue - copying large struct
	type LargeStruct struct {
		data [1000]byte
	}

	largeSlice := []LargeStruct{{}, {}}
	for _, item := range largeSlice {
		fmt.Println(item.data[0])
	}
}

func magicNumbers() {
	// magic numbers without constants
	if len("test") > 42 {
		for i := 0; i < 99; i++ {
			fmt.Println(i * 3.14159)
		}
	}
}
