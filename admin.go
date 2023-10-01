package aws

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/iodasolutions/xbee-common/log2"
	"github.com/iodasolutions/xbee-common/util"
	"sync"
)

type Admin struct {
}

func (pv Admin) DestroyVolumes(names []string) error {
	log2.Infof("asked to destroy volumes %v ...", names)
	ctx := context.Background()
	if regions, err := pv.regionsFromVolumes(ctx); err != nil {
		return err
	} else {
		var wg sync.WaitGroup
		var allExistingNames []string
		for _, r := range regions {
			existingVolumes, existingNames := r.existingVolumesForNames(names)
			allExistingNames = append(allExistingNames, existingNames...)
			if len(existingVolumes) > 0 {
				wg.Add(len(existingVolumes))
			}
			for index, vol := range existingVolumes {
				go func(r *Region2, vol *types.Volume, name string) {
					defer wg.Done()
					in := &ec2.DeleteVolumeInput{
						VolumeId: vol.VolumeId,
					}
					_, err := r.Svc.DeleteVolume(ctx, in)
					if err == nil {
						log2.Infof("successfully destroyed volume %s", name)
					} else {
						log2.Errorf("could not remove volume %s:\n%v", name, err)
					}

				}(r, vol, existingNames[index])
			}
		}
		wg.Wait()
		namesSets := util.SetFromStringSlice(names).Remove(allExistingNames...)
		if namesSets.Size() > 0 {
			for _, aName := range namesSets.Slice() {
				log2.Warnf("volume %s already do not exist", aName)
			}
		}
		return nil
	}
}

func (pv Admin) regionsFromVolumes(ctx context.Context) (map[string]*Region2, error) {
	volumes, err := VolumesFrom()
	if err != nil {
		return nil, err
	}

	var channels []<-chan *response
	for regionName, volumesForRegion := range volumes {
		channels = append(channels, newRegion(ctx, regionName, nil, volumesForRegion))
	}
	ch := util.Multiplex(ctx, channels...)
	result := map[string]*Region2{}
	for resp := range ch {
		if resp.err != nil {
			log2.Errorf("%v", resp.err)
		} else {
			result[resp.r.Name] = resp.r
		}
	}
	return result, nil
}
