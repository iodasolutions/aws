package aws

import (
	"encoding/json"
	"fmt"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/provider"
	"github.com/iodasolutions/xbee-common/types"
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

func fromMap(aMap map[string]interface{}) (*Model, error) {
	var result Model
	data, err := util.NewJsonIO(aMap).SaveAsBytes()
	if err != nil {
		return nil, fmt.Errorf("unexpected when encoding to json : %v", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unexpected when decoding json to AWS model : %v", err)
	}
	if amisForOsArh, ok := result.Amis[result.OsArch]; ok {
		if ami, ok := amisForOsArh[result.Region]; ok {
			result.Ami = ami.(string)
		} else {
			return nil, fmt.Errorf("unsupported region property : %s", result.Region)
		}
	} else {
		return nil, fmt.Errorf("unsupported osarch property : %s", result.OsArch)
	}

	return &result, nil
}

type ProviderHost struct {
	Specification *Model
	Name          string
	Ports         []string
	User          string
	Volumes       []string
	ExternalIp    string
	PackId        *types.IdJson
	PackHash      string
	SystemId      *types.IdJson
	SystemHash    string
}

func (ph *ProviderHost) EffectivePackId() *types.IdJson {
	if ph.PackId == nil {
		return ph.SystemId
	}
	return ph.PackId
}
func (ph *ProviderHost) EffectiveHash() string {
	if ph.PackId == nil {
		return ph.SystemHash
	}
	return ph.PackHash
}
func (ph *ProviderHost) DisplayName() string {
	name := ph.EffectivePackId().ShortName()
	if ph.PackId != nil {
		name += "-" + ph.SystemId.ShortName()
	}
	return name
}
func HostsByRegion() (map[string]map[string]*ProviderHost, *cmd.XbeeError) {
	hosts := provider.Hosts()
	result := map[string]map[string]*ProviderHost{}
	for _, hReq := range hosts {
		h, err := hostFrom(hReq)
		if err != nil {
			return nil, err
		}
		if _, ok := result[h.Specification.Region]; !ok {
			result[h.Specification.Region] = map[string]*ProviderHost{}
		}
		result[h.Specification.Region][h.Name] = h
	}
	return result, nil
}
func HostsByName() (map[string]*ProviderHost, error) {
	hosts := provider.Hosts()
	result := map[string]*ProviderHost{}
	for _, hReq := range hosts {
		h, err := hostFrom(hReq)
		if err != nil {
			return nil, err
		}
		result[h.Name] = h
	}
	return result, nil
}

func hostFrom(req *provider.Host) (*ProviderHost, *cmd.XbeeError) {
	m, err := fromMap(req.Provider)
	if err != nil {
		return nil, cmd.Error("cannot unmarshal json provider data for host %s : %v", req.Name, err)
	}
	return &ProviderHost{
		Specification: m,
		Name:          req.Name,
		Ports:         req.Ports,
		User:          req.User,
		Volumes:       req.Volumes,
		ExternalIp:    req.ExternalIp,
		PackId:        req.PackId,
		PackHash:      req.PackHash,
		SystemId:      req.SystemId,
		SystemHash:    req.SystemHash,
	}, nil
}

type Volume struct {
	provider.GenericVolume
	Specification *Model
}

func volumeFrom(req *provider.Volume) (*Volume, *cmd.XbeeError) {
	m, err := fromMap(req.Provider)
	if err != nil {
		return nil, cmd.Error("cannot unmarshal json provider data for volume %s : %v", req.Name, err)
	}
	return &Volume{
		GenericVolume: provider.FromVolume(req),
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
