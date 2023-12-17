package aws

import (
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/iodasolutions/xbee-common/provider"
)

func EnvFilters() []types.Filter {
	anId := provider.EnvId()
	return []types.Filter{
		{
			Name:   aws.String("tag:xbee.env.commit"),
			Values: []string{anId.Commit},
		},
		{
			Name:   aws.String("tag:xbee.env.origin"),
			Values: []string{anId.Origin},
		},
	}
}
func EnvFiltersForResource(name string) []types.Filter {
	anId := provider.EnvId()
	return []types.Filter{
		{
			Name:   aws.String("tag:xbee.name"),
			Values: []string{name},
		},
		{
			Name:   aws.String("tag:xbee.env.commit"),
			Values: []string{anId.Commit},
		},
		{
			Name:   aws.String("tag:xbee.env.origin"),
			Values: []string{anId.Origin},
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
			Key:   aws.String("xbee.env.commit"),
			Value: &anId.Commit,
		},
		{
			Key:   aws.String("xbee.env.origin"),
			Value: &anId.Origin,
		},
		{
			Key:   aws.String("Name"),
			Value: aws.String(fmt.Sprintf("%s(%s)", name, anId.Colon())),
		},
	}
}
