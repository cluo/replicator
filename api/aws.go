package api

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/elsevier-core-engineering/replicator/logging"
)

// DescribeAWSRegion uses the EC2 InstanceMetaData endpoint to discover the AWS
// region in which the instance is running.
func DescribeAWSRegion() (region string, err error) {

	ec2meta := ec2metadata.New(session.New())
	identity, err := ec2meta.GetInstanceIdentityDocument()
	if err != nil {
		return "", err
	}
	return identity.Region, nil
}

// NewAWSAsgService creates a new AWS API Session and ASG service connection for
// use across all calls as required.
func NewAWSAsgService(region string) (Session *autoscaling.AutoScaling) {
	sess := session.Must(session.NewSession())
	svc := autoscaling.New(sess, &aws.Config{Region: aws.String(region)})
	return svc
}

// DescribeScalingGroup returns the AWS ASG information of the specified ASG.
func DescribeScalingGroup(asgName string, svc *autoscaling.AutoScaling) (asg *autoscaling.DescribeAutoScalingGroupsOutput, err error) {

	params := &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []*string{
			aws.String(asgName),
		},
	}
	resp, err := svc.DescribeAutoScalingGroups(params)

	if err != nil {
		return resp, err
	}

	return resp, nil
}

// ScaleOutCluster scales the Nomad worker pool by 1 instance, using the current
// configuration as the basis for undertaking the work.
func ScaleOutCluster(asgName string, svc *autoscaling.AutoScaling) error {

	// Get the current ASG configuration so that we have the basis on which to
	// update to our new desired state.
	asg, err := DescribeScalingGroup(asgName, svc)
	if err != nil {
		return err
	}

	// The DesiredCapacity is incramented by 1, while the TerminationPolicies and
	// AvailabilityZones which are required parameters are copied from the Info
	// recieved from the initial call to DescribeScalingGroup. These params could
	// be directly referenced within UpdateAutoScalingGroupInput but are here for
	// clarity.
	newDesiredCapacity := *asg.AutoScalingGroups[0].DesiredCapacity + int64(1)
	terminationPolicies := asg.AutoScalingGroups[0].TerminationPolicies
	availabilityZones := asg.AutoScalingGroups[0].AvailabilityZones

	// Setup the Input parameters ready for the AWS API call and then trigger the
	// call which will update the ASG.
	params := &autoscaling.UpdateAutoScalingGroupInput{
		AutoScalingGroupName: aws.String(asgName),
		AvailabilityZones:    availabilityZones,
		DesiredCapacity:      aws.Int64(newDesiredCapacity),
		TerminationPolicies:  terminationPolicies,
	}

	logging.Info("cluster scaling (scale-out) will now be initiated")

	// Currently it is assumed that no error received from the API means that the
	// increase in ASG size has been successful, or at least will be. This may
	// want to change in the future.
	_, err = svc.UpdateAutoScalingGroup(params)
	if err != nil {
		return err
	}

	// Setup a ticker to poll the autoscaling group and report when an instance
	// has been successfully launched.
	ticker := time.NewTicker(time.Millisecond * 500)
	timeout := time.Tick(time.Minute * 3)

	logging.Info("cluster scaling operation (scale-out) will now be verfied, this may take a few minutes...")

	for {
		select {
		case <-timeout:
			logging.Info("timeout %v reached while waiting for autoscaling group %v",
				timeout, asgName)
			return nil
		case <-ticker.C:
			asg, err := DescribeScalingGroup(asgName, svc)
			if err != nil {
				logging.Error("an error occurred while attempting to check autoscaling group: %v", err)
			} else {
				if len(asg.AutoScalingGroups[0].Instances) == int(newDesiredCapacity) {
					logging.Info("cluster scaling operation (scale-out) has been successfully verified")
					return nil
				}
			}
		}
	}
}

// ScaleInCluster scales the cluster size by 1 by using the DetachInstances call
// to target an instance to remove from the ASG.
func ScaleInCluster(asgName, instanceIP string, svc *autoscaling.AutoScaling) error {

	instanceID := translateIptoID(instanceIP, *svc.Config.Region)

	// Setup the Input parameters ready for the AWS API call and then trigger the
	// call which will remove the identified instance from the ASG and decrement
	// the capacity to ensure no new instances are launched into the cluster thus
	// preserving the scale-in situation.
	params := &autoscaling.DetachInstancesInput{
		AutoScalingGroupName:           aws.String(asgName),
		ShouldDecrementDesiredCapacity: aws.Bool(true),
		InstanceIds: []*string{
			aws.String(instanceID),
		},
	}

	resp, err := svc.DetachInstances(params)
	if err != nil {
		return err
	}

	// The initial scaling activity StatusCode is available, so we might as well
	// check it before calling checkScalingActivityResult even though its highly
	// unlikely to have already completed.
	if *resp.Activities[0].StatusCode != "Successful" {
		err = checkClusterScalingResult(resp.Activities[0].ActivityId, svc)
	}

	if err != nil {
		return err
	}

	// The instance must now be terminated using the AWS EC2 API.
	err = terminateInstance(instanceID, *svc.Config.Region)

	if err != nil {
		return fmt.Errorf("an error occured terminating instance %v", instanceID)
	}

	return nil
}

// CheckClusterScalingTimeThreshold checks the last cluster scaling event time
// and compares against the cooldown period to determine whether or not a
// cluster scaling event can happen.
func CheckClusterScalingTimeThreshold(cooldown float64, asgName string, svc *autoscaling.AutoScaling) error {

	// Only supply the ASG name as we want to see all the recent scaling activity
	// to be able to make the correct descision.
	params := &autoscaling.DescribeScalingActivitiesInput{
		AutoScalingGroupName: aws.String(asgName),
	}

	// The last scaling activity to happen is determined irregardless of whether
	// or not it was successful; it was still a scaling event. Times from AWS are
	// based on UTC, and so the current time does the same.
	timeThreshold := time.Now().UTC().Add(-time.Second * time.Duration(cooldown))

	ticker := time.NewTicker(time.Second * time.Duration(10))
	timeOut := time.Tick(time.Minute * 3)
	var lastActivity time.Time

L:
	for {
		select {
		case <-timeOut:
			return fmt.Errorf("timeout %v reached on checking scaling activity threshold", timeOut)
		case <-ticker.C:

			// Make a call to the AWS API every tick to ensure we get the latest Info
			// about the scaling activity status.
			resp, err := svc.DescribeScalingActivities(params)
			if err != nil {
				return err
			}

			// If a scaling activity is in progess, the endtime will not be available
			// yet.
			if *resp.Activities[0].Progress == 100 {
				lastActivity = *resp.Activities[0].EndTime
				break L
			}
		}
	}
	// Compare the two dates to see if the current time minus the cooldown is
	// before the last scaling activity. If it was before, this indicates the
	// cooldown has not been met.
	if !lastActivity.Before(timeThreshold) {
		return fmt.Errorf("cluster scaling cooldown not yet reached")
	}
	return nil
}

// checkClusterScalingResult is used to poll the scaling activity and check for
// a successful completion.
func checkClusterScalingResult(activityID *string, svc *autoscaling.AutoScaling) error {

	// Setup our timeout and ticker value. TODO: add a backoff for every call we
	// make where the scaling event has not completed successfully.
	ticker := time.NewTicker(time.Second * time.Duration(10))
	timeOut := time.Tick(time.Minute * 3)

	for {
		select {
		case <-timeOut:
			return fmt.Errorf("timeout %v reached on checking scaling activity success", timeOut)
		case <-ticker.C:

			// Make a call to the AWS API every tick to ensure we get the latest Info
			// about the scaling activity status.
			params := &autoscaling.DescribeScalingActivitiesInput{
				ActivityIds: []*string{
					aws.String(*activityID),
				},
			}
			resp, err := svc.DescribeScalingActivities(params)
			if err != nil {
				return err
			}

			// Fail fast; if the scaling activity is in a failed or cancelled state
			// we exit. The final check is to see whether or not we have got the
			// Successful state which indicates a completed scaling activity.
			if *resp.Activities[0].StatusCode == "Failed" || *resp.Activities[0].StatusCode == "Cancelled" {
				return fmt.Errorf("scaling activity %v was unsuccessful ", activityID)
			}
			if *resp.Activities[0].StatusCode == "Successful" {
				return nil
			}
		}
	}
}

// terminateInstance will terminate the supplied EC2 instance.
func terminateInstance(instanceID, region string) error {

	// Setup the session and the EC2 service link to use for this operation.
	sess := session.Must(session.NewSession())
	svc := ec2.New(sess, &aws.Config{Region: aws.String(region)})

	params := &ec2.TerminateInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceID),
		},
		DryRun: aws.Bool(false),
	}

	// We do not care about the response data, only if we recieve an error or not
	// from the API which should be indication enough to ensure the instance is
	// terminating.
	_, err := svc.TerminateInstances(params)

	if err != nil {
		return err
	}

	return nil
}

func translateIptoID(ip, region string) (id string) {
	sess := session.Must(session.NewSession())
	svc := ec2.New(sess, &aws.Config{Region: aws.String(region)})

	params := &ec2.DescribeInstancesInput{
		DryRun: aws.Bool(false),
		Filters: []*ec2.Filter{
			{
				Name: aws.String("private-ip-address"),
				Values: []*string{
					aws.String(ip),
				},
			},
		},
	}
	resp, err := svc.DescribeInstances(params)

	if err != nil {
		logging.Error("unable to convert nomad instance IP to AWS ID: %v", err)
		return
	}

	return *resp.Reservations[0].Instances[0].InstanceId
}
