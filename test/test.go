package main

import (
	"fmt"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/newfs"
	"github.com/iodasolutions/xbee-common/provider"
	"github.com/iodasolutions/xbee-common/util"
)

func main() {
	env := initEnv()
	for _, h := range env.Hosts {
		fmt.Printf("osarch=%s\n", h.Element.OsArch)
	}
}

func envJson() newfs.File {
	return newfs.ChildXbee(newfs.CWD()).ChildFileJson("env")
}

func initEnv() *provider.Env {
	var err *cmd.XbeeError
	var env provider.Env
	if env, err = newfs.Unmarshal[provider.Env](envJson()); err != nil {
		newfs.DoExitOnError(err)
	}
	hostProvider := env.Provider["host"].(map[string]interface{})
	for index := range env.Hosts {
		merged := util.MergeMaps(hostProvider, env.Hosts[index].Provider)
		env.Hosts[index].Provider = merged
	}
	volumeProvider := env.Provider["volume"].(map[string]interface{})
	for _, v := range env.Volumes {
		merged := util.MergeMaps(volumeProvider, v.Provider)
		v.Provider = merged
	}
	return &env
}
