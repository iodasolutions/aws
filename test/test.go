package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"time"
)

func main() {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("eu-west-3"),
	)
	if err != nil {
		// Gérer l'erreur
		return
	}

	svc := ec2.NewFromConfig(cfg)

	input := &ec2.CreateImageInput{
		InstanceId: aws.String("i-08c3a2390a87d00e5"), // Remplacez par l'ID de votre instance
		Name:       aws.String("ubuntu-de1548135e"),   // Nommez votre AMI
		NoReboot:   aws.Bool(true),
	}

	result, err := svc.CreateImage(ctx, input)
	if err != nil {
		// Gérer l'erreur
		fmt.Println(err)
		return
	}
	tags := []types.Tag{
		{
			Key:   aws.String("xbee.id"),
			Value: aws.String("ubuntu-de1548135e"),
		},
		{
			Key:   aws.String("xbee.os_arch"),
			Value: aws.String("linux_amd64"),
		},
	}
	if _, err := svc.CreateTags(ctx, &ec2.CreateTagsInput{
		Tags:      tags,
		Resources: []string{*result.ImageId},
	}); err != nil {
		fmt.Println(err)
	}

	for {
		describeInput := &ec2.DescribeImagesInput{
			ImageIds: []string{*result.ImageId},
		}

		describeResult, _ := svc.DescribeImages(ctx, describeInput)
		if len(describeResult.Images) > 0 {
			state := describeResult.Images[0].State
			fmt.Printf("L'état de l'AMI est : %s\n", state)

			if state == "available" || state == "failed" {
				break
			}
		}
		time.Sleep(30 * time.Second) // Pause de 30 secondes avant la prochaine vérification
	}

	fmt.Println("La création de l'AMI est terminée.")
}
