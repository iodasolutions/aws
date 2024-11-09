package aws

import (
	"context"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/log2"
	"github.com/iodasolutions/xbee-common/provider"
	"github.com/iodasolutions/xbee-common/util"
	"sync"
)

type Provider struct {
}

func (pv Provider) Up() ([]*provider.InstanceInfo, *cmd.XbeeError) {
	ctx := context.Background()

	if regions, err := regionsForHosts(ctx); err != nil {
		return nil, err
	} else {
		var channels []<-chan *UpInstanceGeneratorResponse
		for _, r := range regions {
			hosts, volumes := r.Existing()
			if len(hosts) > 0 {
				existingInRegion := r.Filter(hosts, volumes)
				channels = append(channels, existingInRegion.StartInstancesGenerator(ctx))
			}
			hosts, volumes = r.NotExisting()
			if len(hosts) > 0 {
				var names []string
				for name := range hosts {
					names = append(names, name)
				}
				notExistingRegion := r.Filter(hosts, volumes)
				sshCreated, xbeeCreated, err := notExistingRegion.ensureDefaultEnvSecurityGroups(ctx)
				if err != nil {
					log2.Infof("unexpected error when calling ensureDefaultEnvSecurityGroups, unable to create hosts %v : %v", names, err)
				} else {
					envName := provider.EnvName()
					if sshCreated {
						log2.Infof("created SSH security group for env %s in region %s", envName, notExistingRegion.Name)
					}
					if xbeeCreated {
						log2.Infof("created XBEE security group for env %s in region %s", envName, notExistingRegion.Name)
					}
					channels = append(channels, notExistingRegion.CreateInstancesGenerator(ctx))
				}

			}
		}
		ch := util.Multiplex(ctx, channels...)
		var createdAndStarted, created []string
		var inError bool
		upStatuses := map[string]*UpInstanceGeneratorResponse{}
		for upStatus := range ch {
			upStatuses[upStatus.Name] = upStatus
			if upStatus.InError {
				inError = true
			}
			if upStatus.InitiallyNotExisting {
				created = append(created, upStatus.Name)
				createdAndStarted = append(createdAndStarted, upStatus.Name)
			} else if upStatus.InitiallyDown {
				createdAndStarted = append(createdAndStarted, upStatus.Name)
			}
		}
		if inError {
			return nil, cmd.Error("up command failed, provider cannot continue")
		}
		infos := map[string]*provider.InstanceInfo{}
		for _, r := range regions {
			filtered := r.FilterByHostInRequest(createdAndStarted)
			if err := r.waitUntilInstancesAreInState(ctx, "running", filtered...); err != nil {
				return nil, err
			}
			rInfos := r.instanceInfos()
			for _, name := range filtered {
				infos[name] = rInfos[name]
				upStatus := upStatuses[name]
				if upStatus.InitiallyUp {
					infos[name].InitialState = "up"
				} else if upStatus.InitiallyDown {
					infos[name].InitialState = "down"
				} else if upStatus.InitiallyNotExisting {
					infos[name].InitialState = "not existing"
				}
			}
		}
		var wg sync.WaitGroup
		for _, r := range regions {
			createdAndStarted = r.FilterByHostInRequest(created)
			if len(createdAndStarted) > 0 {
				for _, name := range createdAndStarted {
					wg.Add(1)
					go func(r *Region2, name string) {
						defer wg.Done()
						if err := r.AttachVolumes(ctx, name); err != nil {
							log2.Errorf(err.Error())
						}
					}(r, name)
				}
			}
		}
		wg.Wait()
		var result []*provider.InstanceInfo
		for _, info := range infos {
			result = append(result, info)
		}
		return result, nil
	}
}

func (pv Provider) Delete() *cmd.XbeeError {
	ctx := context.Background()

	if regions, err := regionsForHosts(ctx); err != nil {
		return err
	} else {
		var wg sync.WaitGroup
		wg.Add(len(regions))
		for _, r := range regions {
			go func(r *Region2) {
				defer wg.Done()
				r.destroyInstances(ctx)
			}(r)
		}
		wg.Wait()
		return nil
	}

}

func (pv Provider) InstanceInfos() ([]*provider.InstanceInfo, *cmd.XbeeError) {
	ctx := context.Background()
	var result []*provider.InstanceInfo
	if regions, err := regionsForHosts(ctx); err != nil {
		return nil, err
	} else {
		for _, r := range regions {
			for _, info := range r.instanceInfos() {
				result = append(result, info)
			}
		}
		return result, nil
	}
}

func (pv Provider) Image() *cmd.XbeeError {
	ctx := context.Background()

	if regions, err := regionsForHosts(ctx); err != nil {
		return err
	} else {
		var channels []<-chan *OperationStatus
		for _, r := range regions {
			channels = append(channels, r.PackInstancesGenerator(ctx))
		}
		ch := util.Multiplex(ctx, channels...)
		var inError bool
		for status := range ch {
			packName := status.Host.EffectivePackId().ShortName()
			if status.InError {
				inError = true
				log2.Errorf("Creation of AMI %s failed", packName)
			} else {
				log2.Infof("Creation of AMI %s succeeded", packName)
			}
		}
		if inError {
			return cmd.Error("AWS image creation operation failed")
		}
		return nil
	}
}
