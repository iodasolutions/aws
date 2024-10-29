package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/iodasolutions/aws/scripts/aws2"
	"github.com/iodasolutions/xbee-common/log2"
	"github.com/iodasolutions/xbee-common/util"
	"log"
	"time"
)

// ubuntu: "099720109477", "24.04 LTS", "x86_64"
// rockylinux: "792107900819", "Rocky-9-EC2-Base-9.3-20231113.0.x86_64", "x86_64"
// rockylinux: "792107900819", "Rocky-9-EC2-Base-9.3-20231113.0.aarch64", "arm64"
func main() {
	d := util.StartDuration()
	defer func() {
		d.End("test.main")
	}()
	ctx := context.Background()
	result := map[string]types.Image{}
	// arch = x86_64 | arm64
	for response := range AmisGenerator(ctx, "792107900819", "Rocky-9-EC2-Base-9.3-20231113.0.aarch64", "arm64") {
		if response.Err == nil {
			for _, image := range response.Images {
				if existing, ok := result[response.Region]; ok {
					myDate, err := time.Parse(time.RFC3339, *image.CreationDate)
					if err != nil {
						log.Fatalln(err)
					}
					existingDate, err := time.Parse(time.RFC3339, *existing.CreationDate)
					if err != nil {
						log.Fatalln(err)
					}
					if myDate.After(existingDate) {
						result[response.Region] = image
					}
				} else {
					result[response.Region] = image
				}
			}
		} else {
			log2.Errorf("Failed to search in region %s", response.Region)
		}
	}
	for region, image := range result {
		fmt.Printf("                %s: %s\n", region, *image.ImageId)
	}
}

func AmisGenerator(ctx context.Context, ownerId string, name string, archi string) <-chan *aws2.Response {
	var channels []<-chan *aws2.Response
	regions := aws2.AllRegions(ctx)
	for _, r := range regions {
		channels = append(channels, r.AmiFor(ctx, ownerId, name, archi))
	}
	return util.Multiplex(ctx, channels...)
}
