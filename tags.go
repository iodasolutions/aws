package aws

import (
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/iodasolutions/xbee-common/provider"
)

func EnvFilters() []types.Filter {
	return []types.Filter{
		{
			Name:   aws.String("tag:xbee.id"),
			Values: []string{provider.EnvId()},
		},
	}
}
func EnvFiltersForResource(name string) []types.Filter {
	return []types.Filter{
		{
			Name:   aws.String("tag:xbee.name"),
			Values: []string{name},
		},
		{
			Name:   aws.String("tag:xbee.id"),
			Values: []string{provider.EnvId()},
		},
	}
}

func TagsForResource(name string) []types.Tag {
	anId := provider.EnvId()
	return []types.Tag{
		{
			Key:   aws.String("xbee.name"),
			Value: &name,
		},
		{
			Key:   aws.String("xbee.id"),
			Value: &anId,
		},
		{
			Key:   aws.String("Name"),
			Value: aws.String(fmt.Sprintf("%s(%s)", name, provider.EnvName())),
		},
	}
}
