package common

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/packer"
)

type StepCreateTags struct {
	Tags map[string]string
}

func (s *StepCreateTags) Run(state multistep.StateBag) multistep.StepAction {
	ec2conn := state.Get("ec2").(*ec2.EC2)
	ui := state.Get("ui").(packer.Ui)
	amis := state.Get("amis").(map[string]string)

	if len(s.Tags) > 0 {
		for region, ami := range amis {
			ui.Say(fmt.Sprintf("Adding tags to AMI (%s)...", ami))

			var ec2Tags []*ec2.Tag
			for key, value := range s.Tags {
				ui.Message(fmt.Sprintf("Adding tag: \"%s\": \"%s\"", key, value))
				ec2Tags = append(ec2Tags, &ec2.Tag{
					Key:   aws.String(key),
					Value: aws.String(value),
				})
			}

			// Declare list of resources to tag
			resourceIds := []*string{&ami}

			regionconn := ec2.New(&aws.Config{
				Credentials: ec2conn.Config.Credentials,
				Region:      aws.String(region),
			})

			attempts := 0
			max_retries := 60
			sleep_between_retries := 5
			for attempts < max_retries {
				ui.Say(fmt.Sprintf("Testing fix for DescribeImages (%s) attempt %d...", ami, attempts))
				// Retrieve image list for given AMI
				imageResp, err := regionconn.DescribeImages(&ec2.DescribeImagesInput{
					ImageIds: resourceIds,
				})

				if err != nil {
					err := fmt.Errorf("Error retrieving details for AMI (%s): %s, attempt %d", ami, err, attempts)
					state.Put("error", err)
					ui.Error(err.Error())
					if attempts == max_retries - 1 {
						return multistep.ActionHalt
					}
				}

				if len(imageResp.Images) == 0 {
					err := fmt.Errorf("Error retrieving details for AMI (%s), no images found, attempt %d", ami, attempts)
					state.Put("error", err)
					ui.Error(err.Error())
					if attempts == max_retries - 1 {
						return multistep.ActionHalt
					}
				}
				image := imageResp.Images[0]

				if err == nil && len(imageResp.Images) != 0 {
				// Add only those with a Snapshot ID, i.e. not Ephemeral
					for _, device := range image.BlockDeviceMappings {
						if device.Ebs != nil && device.Ebs.SnapshotId != nil {
							ui.Say(fmt.Sprintf("Tagging snapshot: %s", *device.Ebs.SnapshotId))
							resourceIds = append(resourceIds, device.Ebs.SnapshotId)
						}
					}

					_, err = regionconn.CreateTags(&ec2.CreateTagsInput{
						Resources: resourceIds,
						Tags:      ec2Tags,
					})

					if err != nil {
						err := fmt.Errorf("Error adding tags to Resources (%#v): %s, attempt %d", resourceIds, err, attempts)
						state.Put("error", err)
						ui.Error(err.Error())
						if attempts == max_retries - 1 {
							return multistep.ActionHalt
						}
					} else {
					   break
					}
				}

				attempts += 1
				time.Sleep(time.Duration(sleep_between_retries) * time.Second)
			}
		}
	}

	return multistep.ActionContinue
}

func (s *StepCreateTags) Cleanup(state multistep.StateBag) {
	// No cleanup...
}
