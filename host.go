package aws

import (
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/provider"
	"github.com/iodasolutions/xbee-common/types"
)

type Host struct {
	provider.XbeeHost
	Specification *Model
}

func NewHost(host provider.XbeeElement[provider.XbeeHost]) (*Host, *cmd.XbeeError) {
	m, err := NewModel(host.Provider)
	if err != nil {
		return nil, err
	}
	return &Host{XbeeHost: host.Element, Specification: m}, nil
}

func (ph *Host) EffectivePackId() *types.IdJson {
	if ph.PackId == nil {
		return ph.SystemId
	}
	return ph.PackId
}
func (ph *Host) EffectiveHash() string {
	if ph.PackId == nil {
		return ph.SystemHash
	}
	return ph.PackHash
}
func (ph *Host) DisplayName() string {
	name := ph.EffectivePackId().ShortName()
	if ph.PackId != nil {
		name += "-" + ph.SystemId.ShortName()
	}
	return name
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
