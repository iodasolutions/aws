package aws2

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/iodasolutions/xbee-common/util"
	"log"
	"strings"
)

type Region struct {
	Name string
	Svc  *ec2.Client
}

func AllRegions(ctx context.Context) map[string]*Region {
	result := map[string]*Region{}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalln(err)
	}

	client := ec2.NewFromConfig(cfg)
	regRes, err := client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{
		AllRegions: aws.Bool(true),
	})
	if err != nil {
		log.Fatalln(err)
	}
	for _, r := range regRes.Regions {
		name := *r.RegionName
		cfg, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(name),
		)
		if err != nil {
			log.Fatalln(err)
		}
		result[name] = &Region{
			Name: name,
			Svc:  ec2.NewFromConfig(cfg),
		}
	}
	return result
}

type Response struct {
	Region string
	Err    error
	Images []types.Image
}

func (r *Region) AmiFor(ctx context.Context, owner string, name string, archi string) <-chan *Response {
	amiCh := make(chan *Response)
	go func() {
		d := util.StartDuration()
		defer func() {
			d.End(fmt.Sprintf("AmiFor %s", r.Name))
		}()
		defer close(amiCh)
		imRes, err := r.Svc.DescribeImages(ctx, &ec2.DescribeImagesInput{
			Filters: []types.Filter{
				{
					Name:   aws.String("owner-id"),
					Values: []string{owner},
				},
				{
					Name:   aws.String("architecture"),
					Values: []string{archi},
				},
				{
					Name:   aws.String("state"),
					Values: []string{"available"},
				},
			},
		})
		var imagesWithName []types.Image
		if imRes != nil {
			for _, im := range imRes.Images {
				//strings.Contains(*image.Description, "20.04") {
				if im.Description != nil {
					if strings.Contains(*im.Description, name) {
						imagesWithName = append(imagesWithName, im)
					}
				}
			}
		}

		resp := Response{
			Region: r.Name,
			Err:    err,
			Images: imagesWithName,
		}
		amiCh <- &resp
	}()

	return amiCh
}
