package aws

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/log2"
	"github.com/iodasolutions/xbee-common/util"
	"sync"
)

type UpInstanceGeneratorResponse struct {
	InError              bool
	Name                 string
	InitiallyNotExisting bool
	InitiallyDown        bool
	InitiallyUp          bool
}
type response struct {
	r   *Region2
	err error
}

func sendError(ctx context.Context, ch chan *response, err error) {
	select {
	case <-ctx.Done():
	case ch <- &response{
		err: err,
	}:
	}
}

func newRegion(ctx context.Context, name string, hosts map[string]*ProviderHost, volumes map[string]*Volume) <-chan *response {
	ch := make(chan *response)
	go func() {
		defer close(ch)
		cfg, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(name),
		)
		if err != nil {
			sendError(ctx, ch, fmt.Errorf("cannot create session to region %s : %v", name, err))
		} else {
			r := &Region2{
				Name:     name,
				Svc:      ec2.NewFromConfig(cfg),
				Hosts:    hosts,
				Volumes:  volumes,
				ImageMap: map[string]string{},
			}
			var wg sync.WaitGroup
			wg.Add(6)
			go func() {
				defer wg.Done()
				err := r.fillInstances(ctx)
				if err != nil {
					sendError(ctx, ch, fmt.Errorf("an unexpected error occured when describing instances for region %s : %v", name, err))
					return
				}
			}()
			go func() {
				defer wg.Done()
				err = r.fillVolumes(ctx)
				if err != nil {
					sendError(ctx, ch, fmt.Errorf("an unexpected error occured when describing volumes for region %s : %v", name, err))
					return
				}
			}()
			go func() {
				defer wg.Done()
				err = r.ensureVpc(ctx)
				if err != nil {
					sendError(ctx, ch, fmt.Errorf("an unexpected error occured when describing vpc for region %s : %v", name, err))
					return
				}
			}()
			go func() {
				defer wg.Done()
				r.sshSecurityGroupId, r.xbeeSecurityGroupId, err = r.findDefaultEnvSecurityGroups(ctx)
				if err != nil {
					sendError(ctx, ch, fmt.Errorf("an unexpected error occured when searching for default SSH and XBEE security groups in region %s : %v", name, err))
					return
				}
			}()
			go func() {
				defer wg.Done()
				out, err := r.Svc.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{})
				if err != nil {
					sendError(ctx, ch, fmt.Errorf("an unexpected error occured when searching for default Elastic IPs in region %s : %v", name, err))
					return
				}
				r.EIps = out.Addresses
			}()
			go func() {
				defer wg.Done()
				err := r.ensureImages(ctx)
				if err != nil {
					sendError(ctx, ch, fmt.Errorf("an unexpected error occured when searching for AMIs in region %s : %v", name, err))
					return
				}
			}()

			wg.Wait()
			select {
			case <-ctx.Done():
			case ch <- &response{
				r: r,
			}:
			}
		}
	}()
	return ch
}

func (pv Provider) regionsForHosts(ctx context.Context) (map[string]*Region2, *cmd.XbeeError) {
	hosts, err := HostsByRegion()
	if err != nil {
		return nil, err
	}
	volumes, err := VolumesFrom()
	if err != nil {
		return nil, err
	}

	var channels []<-chan *response
	for regionName, hostsForRegion := range hosts {
		volumesForRegion := volumes[regionName]
		channels = append(channels, newRegion(ctx, regionName, hostsForRegion, volumesForRegion))
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

type OperationStatus struct {
	Host    *ProviderHost
	InError bool
}
