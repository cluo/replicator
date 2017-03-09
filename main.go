package main

import (
	"fmt"

	"github.com/elsevier-core-engineering/replicator/replicator"
)

func main() {
	c := replicator.RuntimeStats()
	fmt.Println(c)
}
