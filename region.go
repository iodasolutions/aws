package aws

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/constants"
	"github.com/iodasolutions/xbee-common/log2"
	"github.com/iodasolutions/xbee-common/provider"
	"github.com/iodasolutions/xbee-common/util"
	"strconv"
	"strings"
	"time"
)

type Region2 struct {
	Name  string
	Svc   *ec2.Client
	VpcId *string
	EIps  []types.Address

	//attached to server request
	Volumes map[string]*Volume
	Hosts   map[string]*ProviderHost

	//set lazzily, used for each host in an environment.
	sshSecurityGroupId  string
	xbeeSecurityGroupId string

	//can be rebuilt at any time
	Instances  map[string]*types.Instance
	Ec2Volumes map[string]*types.Volume
}

func (r *Region2) Filter(hosts map[string]*ProviderHost, volumes map[string]*Volume) *Region2 {
	reducedInstances := map[string]*types.Instance{}
	for name := range hosts {
		reducedInstances[name] = r.Instances[name]
	}
	return &Region2{
		Name:                r.Name,
		Svc:                 r.Svc,
		VpcId:               r.VpcId,
		Volumes:             volumes,
		Hosts:               hosts,
		sshSecurityGroupId:  r.sshSecurityGroupId,
		xbeeSecurityGroupId: r.xbeeSecurityGroupId,
		Instances:           reducedInstances,
		Ec2Volumes:          r.Ec2Volumes,
		EIps:                r.EIps,
	}
}
func (r *Region2) HostNames() (result []string) {
	for name := range r.Hosts {
		result = append(result, name)
	}
	return
}
func (r *Region2) FilterByHostInRequest(names []string) (result []string) {
	for _, name := range names {
		if _, ok := r.Hosts[name]; ok {
			result = append(result, name)
		}
	}
	return
}
func (r *Region2) HasVolume(name string) bool {
	_, ok := r.Ec2Volumes[name]
	return ok
}
func (r *Region2) existingVolumesForNames(names []string) (result []*types.Volume, resultNames []string) {
	for _, name := range names {
		if r.HasVolume(name) {
			result = append(result, r.Ec2Volumes[name])
			resultNames = append(resultNames, name)
		}
	}
	return
}

func (r *Region2) NotExisting() (map[string]*ProviderHost, map[string]*Volume) {
	hosts := map[string]*ProviderHost{}
	volumes := map[string]*Volume{}
	for name, h := range r.Hosts {
		if _, ok := r.Instances[name]; !ok {
			hosts[name] = h
			for _, name := range h.Volumes {
				volumes[name] = r.Volumes[name]
			}
		}
	}
	return hosts, volumes
}
func (r *Region2) Existing() (map[string]*ProviderHost, map[string]*Volume) {
	hosts := map[string]*ProviderHost{}
	volumes := map[string]*Volume{}
	for name, h := range r.Hosts {
		if _, ok := r.Instances[name]; ok {
			hosts[name] = h
			for _, name := range h.Volumes {
				volumes[name] = r.Volumes[name]
			}
		}
	}
	return hosts, volumes
}

func (r *Region2) SplitInstancesByStateStoppedFor(names []string) (stopped map[string]*types.Instance, other map[string]*types.Instance) {
	stopped = make(map[string]*types.Instance)
	other = make(map[string]*types.Instance)
	for _, name := range names {
		instance := r.Instances[name]
		state := instance.State.Name
		if state == "stopped" {
			stopped[name] = instance
		} else {
			other[name] = instance
		}
	}
	return stopped, other
}

func (r *Region2) fillInstances(ctx context.Context) *cmd.XbeeError {
	r.Instances = make(map[string]*types.Instance)
	if out, err := r.Svc.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: EnvFilters(),
	}); err == nil {
		for _, reservation := range out.Reservations {
			for _, instance := range reservation.Instances {
				instanceState := instance.State.Name
				if instanceState != "terminated" { // terminated should be scheduled by aws to be removed.
					var hostName string
					for _, tag := range instance.Tags {
						if *tag.Key == "xbee.name" {
							hostName = *tag.Value
							break
						}
					}
					if aHost, ok := r.Hosts[hostName]; ok {
						r.Instances[hostName] = &instance
						if instance.State.Name == "running" && aHost.ExternalIp != "" && instance.PublicIpAddress != nil && *instance.PublicIpAddress != aHost.ExternalIp {
							if _, err = r.Svc.AssociateAddress(ctx, &ec2.AssociateAddressInput{
								InstanceId: instance.InstanceId,
								PublicIp:   aws.String(aHost.ExternalIp),
							}); err != nil {
								return cmd.Error("cannot associate ip %s to instance %s: %v", aHost.ExternalIp, *instance.InstanceId, err)
							}
							instance.PublicIpAddress = &aHost.ExternalIp
						}
					}
				}
			}
		}
		return nil
	} else {
		return cmd.Error("an error occured when getting info for instances in region %s: %v", r.Name, err)
	}
}

func (r *Region2) fillVolumes(ctx context.Context) error {
	result := make(map[string]*types.Volume)
	out, err := r.Svc.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
		Filters: EnvFilters(),
	})
	if err != nil {
		return err
	}

	for _, vol := range out.Volumes {
		for _, tag := range vol.Tags {
			if *tag.Key == "xbee.name" {
				result[*tag.Value] = &vol
				break
			}
		}
	}
	r.Ec2Volumes = result
	return nil
}

func (r *Region2) deviceNameForAmi(ctx context.Context, ami string) (*string, error) {
	imagesOut, err := r.Svc.DescribeImages(ctx, &ec2.DescribeImagesInput{
		ImageIds: []string{ami},
	})
	if err != nil {
		return nil, fmt.Errorf("cannot get ami Infos for %s : %v", ami, err)
	}
	deviceName := imagesOut.Images[0].RootDeviceName
	return deviceName, nil
}

func (r *Region2) StartInstancesGenerator(ctx context.Context) <-chan *UpInstanceGeneratorResponse {
	ch := make(chan *UpInstanceGeneratorResponse)
	go func() {
		defer close(ch)
		for _, elt := range r.startInstances(ctx) {
			select {
			case <-ctx.Done():
				return
			case ch <- elt:
			}
		}
	}()
	return ch
}

func (r *Region2) startInstances(ctx context.Context) (result []*UpInstanceGeneratorResponse) {

	stopped, other := r.SplitInstancesByStateStoppedFor(r.HostNames())
	if len(other) > 0 {
		for name := range other {
			instance := other[name]
			status := instance.State.Name
			if status == "running" {
				result = append(result, &UpInstanceGeneratorResponse{
					Name:        name,
					InitiallyUp: true,
				})
				log2.Infof("instance %s already in running state", name)
			} else {
				log2.Infof("instance %s in %s state, can not be started", name, status)
				result = append(result, &UpInstanceGeneratorResponse{
					Name:    name,
					InError: true,
				})
			}
		}
	}
	if len(stopped) > 0 {
		var names []string
		var instanceIds []string
		for name, instance := range stopped {
			names = append(names, name)
			instanceIds = append(instanceIds, *instance.InstanceId)
		}
		_, err := r.Svc.StartInstances(ctx, &ec2.StartInstancesInput{
			InstanceIds: instanceIds,
		})
		if err == nil {
			log2.Infof("successfully called start on aws instances %v", names)
			for _, name := range names {
				result = append(result, &UpInstanceGeneratorResponse{
					Name:          name,
					InitiallyDown: true,
				})
			}
		} else {
			log2.Errorf("cannot start aws instances %v for region %s : %v", names, r.Name, err)
			for _, name := range names {
				result = append(result, &UpInstanceGeneratorResponse{
					Name:    name,
					InError: true,
				})
			}
			return
		}
	}
	return
}

func (r *Region2) destroyInstances(ctx context.Context) {
	var err error
	notExisting, _ := r.NotExisting()
	if len(notExisting) > 0 {
		var names []string
		for name := range notExisting {
			names = append(names, name)
		}
		log2.Infof("instance %v already terminated or do not exist", names)
	}
	existing, _ := r.Existing()
	if len(existing) > 0 {
		var names []string
		var instanceIds []string
		instances := r.Instances //save instances to variables
		for name := range existing {
			names = append(names, name)
			instanceIds = append(instanceIds, *r.Instances[name].InstanceId)
		}
		_, err = r.Svc.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
			InstanceIds: instanceIds,
		})
		if err == nil {
			log2.Infof("successfully called terminate for instances %v in region %s", names, r.Name)
		} else {
			log2.Errorf("cannot terminate aws instances %v for region %s : %v", names, r.Name, err)
			return
		}
		log2.Infof("transitioning instances from shutting-down to terminated, for %v, wait...", names)
		err = r.waitUntilInstancesAreInState(ctx, "terminated", names...)
		if err != nil {
			log2.Errorf(err.Error())
			return
		}
		for name := range existing {
			instance := instances[name]
			for _, secGroup := range instance.SecurityGroups {
				if *secGroup.GroupId != r.sshSecurityGroupId && *secGroup.GroupId != r.xbeeSecurityGroupId {
					_, err = r.Svc.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
						GroupId: secGroup.GroupId,
					})
					if err == nil {
						log2.Infof("successfully deleted security group for %s", name)
					} else {
						log2.Errorf("could not delete security group for %s : %v", name, err)
					}
				}
			}
		}
		_, err = r.Svc.DeleteTags(ctx, &ec2.DeleteTagsInput{
			Resources: instanceIds,
		})
		if err == nil {
			log2.Infof("successfully deleted xbee tags for %s", names)
		} else {
			log2.Errorf("could not delete xbee tags for %s : %v", names, err)
		}

		err = r.deleteDefaultSecurityGroupsForEnvIfPossible(ctx)
		if err != nil {
			log2.Errorf("%v", err)
		}
	} else {
		if r.xbeeSecurityGroupId != "" || r.sshSecurityGroupId != "" {
			err = r.deleteDefaultSecurityGroupsForEnvIfPossible(ctx)
			if err != nil {
				log2.Errorf("%v", err)
			} else {
				anId := provider.EnvId()
				envName := anId.ShortName()
				if r.xbeeSecurityGroupId != "" {
					log2.Infof("successfully delete XBEE security group for env %s in region %s", envName, r.Name)
				}
				if r.sshSecurityGroupId != "" {
					log2.Infof("successfully delete SSH security group for env %s in region %s", envName, r.Name)
				}
			}
		}
	}

}

func (r *Region2) deleteDefaultSecurityGroupsForEnvIfPossible(ctx context.Context) (err error) {
	var hasInstance bool
	for _, info := range r.instanceInfos() {
		if info.State != constants.State.NotExisting {
			hasInstance = true
		}
	}
	if !hasInstance {
		if err = r.deleteDefaultSecurityGroupsForEnv(ctx); err != nil {
			return
		}
	}
	return
}
func (r *Region2) deleteDefaultSecurityGroupsForEnv(ctx context.Context) error {
	anId := provider.EnvId()
	envName := anId.ShortName()
	if r.sshSecurityGroupId != "" {
		_, err := r.Svc.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
			GroupId: &r.sshSecurityGroupId,
		})
		if err != nil {
			return fmt.Errorf("unexpected error when deleting SSH security group for %s in region %s : %v", envName, r.Name, err)
		}
	}
	if r.xbeeSecurityGroupId != "" {
		_, err := r.Svc.RevokeSecurityGroupIngress(ctx, &ec2.RevokeSecurityGroupIngressInput{
			IpPermissions: []types.IpPermission{
				{
					IpProtocol: aws.String("-1"),
					UserIdGroupPairs: []types.UserIdGroupPair{
						{
							GroupId: &r.xbeeSecurityGroupId,
							VpcId:   r.VpcId,
						},
					},
				},
			},
			GroupId: &r.xbeeSecurityGroupId,
		})
		if err != nil {
			return fmt.Errorf("unexpected error when revoking ingress in XBEE security group for %s in region %s : %v", envName, r.Name, err)
		}
		_, err = r.Svc.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
			GroupId: &r.xbeeSecurityGroupId,
		})
		if err != nil {
			return fmt.Errorf("unexpected error when deleting XBEE security group for %s in region %s : %v", envName, r.Name, err)
		}
	}
	return nil
}

func (r *Region2) waitUntilInstancesAreInState(ctx context.Context, state types.InstanceStateName, names ...string) *cmd.XbeeError {

	for {
		select {
		case <-ctx.Done():
			return cmd.Error("an error occured while putting %v for region %s to %s: %v", names, r.Name, state, ctx.Err())
		case <-time.After(500 * time.Millisecond):
			err := r.fillInstances(ctx)
			if err != nil {
				return err
			}
			ok := r.areInstancesInState(state, names...)
			if ok {
				log2.Infof("aws instances are now %s", state)
				return nil
			}
		}
	}
}

func (r *Region2) areInstancesInState(state types.InstanceStateName, names ...string) bool {
	for _, name := range names {
		if instance, ok := r.Instances[name]; ok {
			status := instance.State.Name
			if status != state {
				return false
			}
		} else {
			if state != "terminated" {
				return false
			}
		}
	}
	return true
}
func (r *Region2) ensureVpc(ctx context.Context) error {
	input := &ec2.DescribeVpcsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("isDefault"),
				Values: []string{"true"},
			},
		},
	}
	if out, err := r.Svc.DescribeVpcs(ctx, input); err != nil {
		return err
	} else {
		if len(out.Vpcs) == 0 {
			if out, err := r.Svc.CreateDefaultVpc(ctx, &ec2.CreateDefaultVpcInput{}); err != nil {
				return err
			} else {
				r.VpcId = out.Vpc.VpcId
			}

		} else {
			r.VpcId = out.Vpcs[0].VpcId
		}
	}
	return nil
}

func (r *Region2) CreateInstancesGenerator(ctx context.Context) <-chan *UpInstanceGeneratorResponse {
	var channels []<-chan *UpInstanceGeneratorResponse
	for _, h := range r.Hosts {
		ch := make(chan *UpInstanceGeneratorResponse)
		channels = append(channels, ch)
		go func(h *ProviderHost) {
			defer close(ch)
			if err := r.createOneInstance(ctx, h); err != nil {
				log2.Errorf(err.Error())
				ch <- &UpInstanceGeneratorResponse{
					Name:    h.Name,
					InError: true,
				}
			} else {
				ch <- &UpInstanceGeneratorResponse{
					Name:                 h.Name,
					InitiallyNotExisting: true,
				}
			}
		}(h)
	}
	return util.Multiplex(ctx, channels...)
}

func (r *Region2) createOneInstance(ctx context.Context, h *ProviderHost) error {
	secGroupIds := []string{r.sshSecurityGroupId, r.xbeeSecurityGroupId}
	if len(h.Ports) > 0 {
		secGroupId, err := r.createSecurityGroup(ctx, h)
		if err != nil {
			return err
		}
		secGroupIds = append(secGroupIds, *secGroupId)
	}
	deviceName, err := r.deviceNameForAmi(ctx, h.Specification.Ami)
	if err != nil {
		return err
	}
	placement, err := r.availabilityZoneFor(h)
	if err != nil {
		return err
	}
	tags := TagsForResource(h.Name)
	userData, err := UserDataBase64(h.User)
	if err != nil {
		return err
	}
	out, err := r.Svc.RunInstances(ctx, &ec2.RunInstancesInput{
		Placement: placement,
		BlockDeviceMappings: []types.BlockDeviceMapping{
			{
				DeviceName: deviceName,
				Ebs: &types.EbsBlockDevice{
					VolumeSize:          aws.Int32(int32(h.Specification.Size)),
					DeleteOnTermination: aws.Bool(true),
				},
			},
		},
		// An Amazon Linux AMI ID for t2.micro instances in the us-west-2 region
		ImageId:          aws.String(h.Specification.Ami),
		InstanceType:     types.InstanceType(h.Specification.InstanceType),
		MinCount:         aws.Int32(1),
		MaxCount:         aws.Int32(1),
		SecurityGroupIds: secGroupIds,
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeInstance,
				Tags:         tags,
			},
			{
				ResourceType: types.ResourceTypeVolume,
				Tags:         tags,
			},
		},
		UserData: userData,
	})
	if err != nil {
		return fmt.Errorf("cannot create aws instance for %s : %v", h.Name, err)
	}
	az := out.Instances[0].Placement.AvailabilityZone
	for _, volName := range h.Volumes {
		if !r.HasVolume(volName) {
			if err := r.createVolume(ctx, volName, az); err != nil {
				return err
			}
		}
	}
	//publicIp := *out.Instances[0].PublicIpAddress
	//if publicIp != h.ExternalIp {
	//	if _, err = r.Svc.AssociateAddress(ctx, &ec2.AssociateAddressInput{
	//		InstanceId: out.Instances[0].InstanceId,
	//		PublicIp:   aws.String(h.ExternalIp),
	//	}); err != nil {
	//		return err
	//	}
	//}
	return nil
}

func (r *Region2) createSecurityGroup(ctx context.Context, host *ProviderHost) (*string, error) {
	var secGroupId *string
	if res, err := r.Svc.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		VpcId:       r.VpcId,
		Description: aws.String("created by aws provider for XBEE"),
		GroupName:   aws.String(host.Name),
	}); err != nil {
		return nil, fmt.Errorf("cannot create security group for host %s in region %s : %v", host.Name, r.Name, err)
	} else {
		secGroupId = res.GroupId
		tags := TagsForResource(host.Name)
		if _, err := r.Svc.CreateTags(ctx, &ec2.CreateTagsInput{
			Tags:      tags,
			Resources: []string{*res.GroupId},
		}); err != nil {
			return nil, fmt.Errorf("cannot tag security group for host %s in region %s : %v", host.Name, r.Name, err)
		}
	}
	var fromPorts []int64
	var toPorts []int64
	for _, portMapping := range host.Ports {
		index := strings.Index(portMapping, ":")
		var fromPort, toPort int
		if index != -1 {
			toPort, _ = strconv.Atoi(portMapping[:index])     //format error should not occur at this stage
			fromPort, _ = strconv.Atoi(portMapping[index+1:]) //format error should not occur at this stage
		} else {
			fromPort, _ = strconv.Atoi(portMapping) //format error should not occur at this stage
			toPort = fromPort
		}
		fromPorts = append(fromPorts, int64(fromPort))
		toPorts = append(toPorts, int64(toPort))
	}
	var ipPermissions []types.IpPermission
	for index, port := range fromPorts {
		ipPermissions = append(ipPermissions,
			types.IpPermission{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(int32(port)),
				ToPort:     aws.Int32(int32(toPorts[index])),
				IpRanges: []types.IpRange{
					{CidrIp: aws.String("0.0.0.0/0")},
				},
			})

	}
	if _, err := r.Svc.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId:       secGroupId,
		IpPermissions: ipPermissions,
	}); err != nil {
		return nil, fmt.Errorf("cannot set inbound rules for host %s : %v", host.Name, err)
	}
	log2.Infof("created security group for host %s with allowed ports %v", host.Name, host.Ports)
	return secGroupId, nil
}

func (r *Region2) findDefaultEnvSecurityGroups(ctx context.Context) (string, string, error) {
	var sshSecurityGroupId, xbeeSecurityGroupId string
	anId := provider.EnvId()
	out, err := r.Svc.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: EnvFiltersForResource("SSH"),
	})
	if err != nil {
		return "", "", fmt.Errorf("unexpected error when looking for existing SSH security group for env %s in region %s : %v", anId.ShortName(), r.Name, err)
	}
	if len(out.SecurityGroups) > 0 {
		sshSecurityGroupId = *out.SecurityGroups[0].GroupId
	}
	out, err = r.Svc.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: EnvFiltersForResource("XBEE"),
	})
	if err != nil {
		return "", "", fmt.Errorf("unexpected error when looking for existing XBEE security group for env %s in region %s : %v", anId.ShortName(), r.Name, err)
	}
	if len(out.SecurityGroups) > 0 {
		xbeeSecurityGroupId = *out.SecurityGroups[0].GroupId
	}
	return sshSecurityGroupId, xbeeSecurityGroupId, nil
}

// port 22, 9801 and self security group
func (r *Region2) ensureDefaultEnvSecurityGroups(ctx context.Context) (bool, bool, error) {
	var sshCreated, xbeeCreated bool
	var err error
	if r.sshSecurityGroupId == "" {
		r.sshSecurityGroupId, err = r.createSSHSecurityGroup(ctx)
		if err != nil {
			return false, false, err
		} else {
			sshCreated = true
		}
	}
	if r.xbeeSecurityGroupId == "" {
		r.xbeeSecurityGroupId, err = r.createXbeeSecurityGroup(ctx)
		if err != nil {
			return false, false, err
		} else {
			xbeeCreated = true
		}
	}
	return sshCreated, xbeeCreated, nil
}

func (r *Region2) createSSHSecurityGroup(ctx context.Context) (string, error) {
	var secGroupId string
	anId := provider.EnvId()
	envName := anId.ShortName()
	if res, err := r.Svc.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		VpcId:       r.VpcId,
		Description: aws.String("created by aws provider for XBEE"),
		GroupName:   aws.String(fmt.Sprintf("SSH Securiy Group for env %s", envName)),
	}); err != nil {
		return "", fmt.Errorf("cannot create security group for env %s in region %s : %v", envName, r.Name, err)
	} else {
		secGroupId = *res.GroupId
		tags := TagsForResource("SSH")
		if _, err := r.Svc.CreateTags(ctx, &ec2.CreateTagsInput{
			Tags:      tags,
			Resources: []string{*res.GroupId},
		}); err != nil {
			return "", fmt.Errorf("cannot tag security group SSH for env %s in region %s : %v", envName, r.Name, err)
		}
	}
	fromPorts := []int64{22}
	toPorts := []int64{22}
	var ipPermissions []types.IpPermission
	for index, port := range fromPorts {
		ipPermissions = append(ipPermissions,
			types.IpPermission{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(int32(port)),
				ToPort:     aws.Int32(int32(toPorts[index])),
				IpRanges: []types.IpRange{
					{CidrIp: aws.String("0.0.0.0/0")},
				},
			})
	}
	if _, err := r.Svc.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId:       &secGroupId,
		IpPermissions: ipPermissions,
	}); err != nil {
		return "", fmt.Errorf("cannot set inbound rules for SSH security group for env %s : %v", envName, err)
	}
	return secGroupId, nil
}

func (r *Region2) createXbeeSecurityGroup(ctx context.Context) (string, error) {
	var secGroupId string
	anId := provider.EnvId()
	envName := anId.ShortName()
	if res, err := r.Svc.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		VpcId:       r.VpcId,
		Description: aws.String("created by aws provider for XBEE"),
		GroupName:   aws.String(fmt.Sprintf("Xbee Securiy Group for env %s", envName)),
	}); err != nil {
		return "", fmt.Errorf("cannot create xbee security group for env %s in region %s : %v", envName, r.Name, err)
	} else {
		secGroupId = *res.GroupId
		tags := TagsForResource("XBEE")
		if _, err := r.Svc.CreateTags(ctx, &ec2.CreateTagsInput{
			Tags:      tags,
			Resources: []string{*res.GroupId},
		}); err != nil {
			return "", fmt.Errorf("cannot tag xbee security group for env %s in region %s : %v", envName, r.Name, err)
		}
	}
	ipPermissions := []types.IpPermission{
		{
			IpProtocol: aws.String("-1"),
			UserIdGroupPairs: []types.UserIdGroupPair{
				{
					GroupId: &secGroupId,
					VpcId:   r.VpcId,
				},
			},
		},
	}
	if _, err := r.Svc.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId:       &secGroupId,
		IpPermissions: ipPermissions,
	}); err != nil {
		return "", fmt.Errorf("cannot set inbound rules XBEE security group for env %s : %v", envName, err)
	}
	return secGroupId, nil
}

func (r *Region2) availabilityZoneFor(h *ProviderHost) (*types.Placement, error) {
	var az string
	var existingVolume string
	for _, volName := range h.Volumes {
		if r.HasVolume(volName) {
			existingVolume = volName
			break
		}
	}
	if existingVolume != "" {
		vol := r.Ec2Volumes[existingVolume]
		if h.Specification.AvailabilityZone != "" && h.Specification.AvailabilityZone != *vol.AvailabilityZone {
			return nil, fmt.Errorf("attached volume %s exist in zone %s, but instance must be created in zone %s", existingVolume, *vol.AvailabilityZone, h.Specification.AvailabilityZone)
		}
		az = *vol.AvailabilityZone
	} else {
		az = h.Specification.AvailabilityZone
	}
	if az != "" {
		placement := &types.Placement{
			AvailabilityZone: &az,
		}
		return placement, nil
	}
	return nil, nil
}

func (r *Region2) createVolume(ctx context.Context, volName string, az *string) error {
	vol := r.Volumes[volName]
	v, err := r.Svc.CreateVolume(ctx, &ec2.CreateVolumeInput{
		AvailabilityZone: az,
		Size:             aws.Int32(int32(vol.Size)),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeVolume,
				Tags:         TagsForResource(vol.Name),
			},
		},
		VolumeType: types.VolumeType(vol.Specification.VolumeType),
	})
	if err != nil {
		return fmt.Errorf("cannot create volume %s : %v", vol.Name, err)
	}
	r.Ec2Volumes[vol.Name] = toVolume(v)
	return nil
}

func toVolume(output *ec2.CreateVolumeOutput) *types.Volume {
	v := &types.Volume{
		Attachments:        output.Attachments,
		AvailabilityZone:   output.AvailabilityZone,
		CreateTime:         output.CreateTime,
		Encrypted:          output.Encrypted,
		FastRestored:       output.FastRestored,
		Iops:               output.Iops,
		KmsKeyId:           output.KmsKeyId,
		MultiAttachEnabled: output.MultiAttachEnabled,
		OutpostArn:         output.OutpostArn,
		Size:               output.Size,
		SnapshotId:         output.SnapshotId,
		State:              output.State,
		Tags:               output.Tags,
		Throughput:         output.Throughput,
		VolumeId:           output.VolumeId,
		VolumeType:         output.VolumeType,
	}
	return v
}

func (r *Region2) AttachVolumes(ctx context.Context, hostName string) error {
	toto := "efghijklmn"
	volumeCount := 0
	instance := r.Instances[hostName]
	h := r.Hosts[hostName]
	for _, volume := range h.Volumes {
		ec2Vol := r.Ec2Volumes[volume]
		volumeCount++
		if attachment, err := r.Svc.AttachVolume(ctx, &ec2.AttachVolumeInput{
			Device:     aws.String(fmt.Sprintf("/dev/sd%c", toto[volumeCount])),
			InstanceId: instance.InstanceId,
			VolumeId:   ec2Vol.VolumeId,
		}); err != nil {
			return fmt.Errorf("cannot attach volume %s to instance %s : %v", volume, h.Name, err)
		} else {
			log2.Infof("Volume %s is attached under device %s", volume, *attachment.Device)
		}
	}
	return nil
}

func (r *Region2) instanceInfos() map[string]*provider.InstanceInfo {
	result := map[string]*provider.InstanceInfo{}
	for hostName, instance := range r.Instances {
		info := &provider.InstanceInfo{
			Name:  hostName,
			State: xbeeState(string(instance.State.Name)),
			User:  r.Hosts[hostName].User,
		}
		result[hostName] = info
		if info.State == constants.State.Up {
			info.ExternalIp = *instance.PublicIpAddress
			info.SSHPort = "22"
			for _, ifeth := range instance.NetworkInterfaces { //Warn only last private ip is returned
				info.Ip = *ifeth.PrivateIpAddress
			}
		}
	}
	for hostName := range r.Hosts {
		if _, ok := result[hostName]; !ok {
			info := &provider.InstanceInfo{
				Name:  hostName,
				State: constants.State.NotExisting,
				User:  r.Hosts[hostName].User,
			}
			result[hostName] = info
		}
	}
	return result
}

func xbeeState(state string) string {
	switch state {
	case "stopping":
		return constants.State.Stopping
	case "shutting-down":
		return constants.State.ShuttingDown
	case "pending":
		return constants.State.Pending
	case "stopped":
		return constants.State.Down
	case "running":
		return constants.State.Up
	case "terminated":
		return constants.State.NotExisting
	default:
		panic(cmd.Error("unsupported state %s", state))
	}
}
