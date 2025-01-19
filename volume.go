package aws

import (
	"encoding/json"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/provider"
	"github.com/iodasolutions/xbee-common/util"
)

type AwsVolumeData struct {
	Size       int    `json:"size"`
	VolumeType string `json:"volumeType"`
	Region     string `json:"region"`
}

type Volume struct {
	provider.XbeeVolume
	Specification *AwsVolumeData
}

func volumeFrom(req provider.XbeeVolume) (*Volume, *cmd.XbeeError) {
	var result AwsVolumeData
	data, err := util.NewJsonIO(req.Provider).SaveAsBytes()
	if err != nil {
		panic(cmd.Error("unexpected error when serializing data provider: %v", err))
	}
	if err := json.Unmarshal(data, &result); err != nil {
		panic(cmd.Error("unexpected error when deserializing data provider : %v", err))
	}
	return &Volume{
		XbeeVolume:    req,
		Specification: &result,
	}, nil
}

func VolumesFrom() (map[string]map[string]*Volume, *cmd.XbeeError) {
	volumes := provider.VolumesForEnv()
	result := make(map[string]map[string]*Volume)
	for _, vReq := range volumes {
		v, err := volumeFrom(vReq)
		if err != nil {
			return nil, err
		}
		if _, ok := result[v.Specification.Region]; !ok {
			result[v.Specification.Region] = map[string]*Volume{}
		}
		result[v.Specification.Region][v.Name] = v
	}
	return result, nil
}
