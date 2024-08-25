package aws

import (
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/provider"
)

type Volume struct {
	provider.XbeeVolume
	Specification *Model
}

func volumeFrom(req provider.XbeeElement[provider.XbeeVolume]) (*Volume, *cmd.XbeeError) {
	m, err := NewModel(req.Provider)
	if err != nil {
		return nil, cmd.Error("cannot unmarshal json provider data for volume %s : %v", req.Element.Name, err)
	}
	return &Volume{
		XbeeVolume:    req.Element,
		Specification: m,
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
