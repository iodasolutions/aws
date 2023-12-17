package main

import (
	"context"
	"github.com/iodasolutions/xbee-common/indus"
	"log"
)

func main() {
	ctx := context.TODO()
	if err := indus.BuildAndDeploy(ctx, "main", "aws"); err != nil {
		log.Fatal(err)
	}
}
