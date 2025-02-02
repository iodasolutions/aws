package aws

import (
	"encoding/json"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/provider"
	"github.com/iodasolutions/xbee-common/util"
)

type AwsHostData struct {
	AvailabilityZone string `json:"availabilityZone"`
	InstanceType     string `json:"instanceType"`
	Region           string `json:"region"`
	Size             int    `json:"size"`

	Ami string `json:"ami"`
}

type Host struct {
	*provider.XbeeHost
	Specification *AwsHostData
}

func NewHost(host *provider.XbeeHost) (*Host, *cmd.XbeeError) {
	var result AwsHostData
	data, err := util.NewJsonIO(host.Provider).SaveAsBytes()
	if err != nil {
		panic(cmd.Error("unexpected error when serializing data provider: %v", err))
	}
	if err := json.Unmarshal(data, &result); err != nil {
		panic(cmd.Error("unexpected error when deserializing data provider : %v", err))
	}
	amis := provider.SystemProviderDataFor(host.SystemHash)["amis"].(map[string]interface{})
	if amisForOsArh, ok := amis[host.OsArch].(map[string]interface{}); ok {
		if ami, ok := amisForOsArh[result.Region]; ok {
			result.Ami = ami.(string)
		} else {
			return nil, cmd.Error("unsupported region property : %s", result.Region)
		}
	} else {
		return nil, cmd.Error("unsupported osarch property : %s", host.OsArch)
	}
	return &Host{XbeeHost: host, Specification: &result}, nil
}

func HostsByRegion() (map[string]map[string]*Host, *cmd.XbeeError) {
	hosts := provider.Hosts()
	result := map[string]map[string]*Host{}
	for _, hReq := range hosts {
		h, err := NewHost(hReq)
		if err != nil {
			return nil, err
		}
		if _, ok := result[h.Specification.Region]; !ok {
			result[h.Specification.Region] = map[string]*Host{}
		}
		result[h.Specification.Region][h.Name] = h
	}
	return result, nil
}
