package aws

import (
	"encoding/json"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/util"
)

type Model struct {
	Region           string                            `json:"region,omitempty"`
	AvailabilityZone string                            `json:"availabilityZone,omitempty"`
	OsArch           string                            `json:"osarch,omitempty"`
	InstanceType     string                            `json:"instanceType,omitempty"`
	VolumeType       string                            `json:"volumeType,omitempty"`
	Size             int                               `json:"size,omitempty"`
	Amis             map[string]map[string]interface{} `json:"amis,omitempty"`

	Ami string `json:"ami,omitempty"` //set at runtime from Region and Amis property
}

func NewModel(p map[string]interface{}) (*Model, *cmd.XbeeError) {
	var result Model
	data, err := util.NewJsonIO(p).SaveAsBytes()
	if err != nil {
		panic(cmd.Error("unexpected error when serializing data provider: %v", err))
	}
	if err := json.Unmarshal(data, &result); err != nil {
		panic(cmd.Error("unexpected error when deserializing data provider : %v", err))
	}
	if amisForOsArh, ok := result.Amis[result.OsArch]; ok {
		if ami, ok := amisForOsArh[result.Region]; ok {
			result.Ami = ami.(string)
		} else {
			return nil, cmd.Error("unsupported region property : %s", result.Region)
		}
	} else {
		return nil, cmd.Error("unsupported osarch property : %s", result.OsArch)
	}

	return &result, nil
}
