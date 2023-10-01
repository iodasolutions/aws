package main

import (
	"github.com/iodasolutions/aws"
	"github.com/iodasolutions/xbee-common/provider"
)

func main() {
	var p aws.Provider
	var a aws.Admin
	provider.Execute(p, a)
}
