package aws

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	amerrors "k8s.io/apimachinery/pkg/util/errors"
	epresolver "sigs.k8s.io/aws-load-balancer-controller/pkg/aws/endpoints"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/metrics"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/throttle"
)

// NewCloud constructs new Cloud implementation.
func NewCloud(cfg CloudConfig, metricsRegisterer prometheus.Registerer, logger logr.Logger) (services.Cloud, error) {
	hasIPv4 := true
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		hasIPv4 = false
		for _, addr := range addrs {
			str := addr.String()
			if !strings.HasPrefix(str, "127.") && !strings.Contains(str, ":") {
				hasIPv4 = true
				break
			}
		}
	}

	endpointsResolver := epresolver.NewResolver(cfg.AWSEndpoints)
	metadataCFG := aws.NewConfig().WithEndpointResolver(endpointsResolver)
	opts := session.Options{}
	opts.Config.MergeIn(metadataCFG)
	if !hasIPv4 {
		opts.EC2IMDSEndpointMode = endpoints.EC2IMDSEndpointModeStateIPv6
	}

	metadataSess := session.Must(session.NewSessionWithOptions(opts))
	metadata := services.NewEC2Metadata(metadataSess)

	if len(cfg.Region) == 0 {
		region := os.Getenv("AWS_DEFAULT_REGION")
		if region == "" {
			region = os.Getenv("AWS_REGION")
		}

		if region == "" {
			err := (error)(nil)
			region, err = metadata.Region()
			if err != nil {
				return nil, errors.Wrap(err, "failed to introspect region from EC2Metadata, specify --aws-region instead if EC2Metadata is unavailable")
			}
		}
		cfg.Region = region
	}
	awsCFG := aws.NewConfig().WithRegion(cfg.Region).WithSTSRegionalEndpoint(endpoints.RegionalSTSEndpoint).WithMaxRetries(cfg.MaxRetries).WithEndpointResolver(endpointsResolver)
	opts = session.Options{}
	opts.Config.MergeIn(awsCFG)
	if !hasIPv4 {
		opts.EC2IMDSEndpointMode = endpoints.EC2IMDSEndpointModeStateIPv6
	}
	sess := session.Must(session.NewSessionWithOptions(opts))
	injectUserAgent(&sess.Handlers)

	if cfg.ThrottleConfig != nil {
		throttler := throttle.NewThrottler(cfg.ThrottleConfig)
		throttler.InjectHandlers(&sess.Handlers)
	}
	if metricsRegisterer != nil {
		metricsCollector, err := metrics.NewCollector(metricsRegisterer)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to initialize sdk metrics collector")
		}
		metricsCollector.InjectHandlers(&sess.Handlers)
	}

	ec2Service := services.NewEC2(sess)

	vpcID, err := getVpcID(cfg, ec2Service, metadata, logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get VPC ID")
	}
	cfg.VpcID = vpcID

	thisObj := &defaultCloud{
		cfg:         cfg,
		ec2:         ec2Service,
		acm:         services.NewACM(sess),
		wafv2:       services.NewWAFv2(sess),
		wafRegional: services.NewWAFRegional(sess, cfg.Region),
		shield:      services.NewShield(sess),
		rgt:         services.NewRGT(sess),

		assumeRoleElbV2: make(map[string]services.ELBV2),
		session:         sess,
		awsCFG:          awsCFG,
		logger:          logger,
	}

	thisObj.elbv2 = services.NewELBV2(sess, thisObj, awsCFG)

	return thisObj, nil
}

func getVpcID(cfg CloudConfig, ec2Service services.EC2, metadata services.EC2Metadata, logger logr.Logger) (string, error) {

	if cfg.VpcID != "" {
		logger.V(1).Info("vpcid is specified using flag --aws-vpc-id, controller will use the value", "vpc: ", cfg.VpcID)
		return cfg.VpcID, nil
	}

	if cfg.VpcTags != nil {
		return inferVPCIDFromTags(ec2Service, cfg.VpcNameTagKey, cfg.VpcTags[cfg.VpcNameTagKey])
	}

	return inferVPCID(metadata, ec2Service)
}

func inferVPCID(metadata services.EC2Metadata, ec2Service services.EC2) (string, error) {
	var errList []error
	vpcId, err := metadata.VpcID()
	if err == nil {
		return vpcId, nil
	} else {
		errList = append(errList, errors.Wrap(err, "failed to fetch VPC ID from instance metadata"))
	}

	nodeName := os.Getenv("NODENAME")
	if strings.HasPrefix(nodeName, "i-") {
		output, err := ec2Service.DescribeInstances(&ec2.DescribeInstancesInput{
			InstanceIds: []*string{&nodeName},
		})
		if err != nil {
			errList = append(errList, errors.Wrapf(err, "failed to describe instance %q", nodeName))
			return "", amerrors.NewAggregate(errList)
		}
		if len(output.Reservations) != 1 {
			errList = append(errList, fmt.Errorf("found more than one reservation for instance %q", nodeName))
			return "", amerrors.NewAggregate(errList)
		}
		if len(output.Reservations[0].Instances) != 1 {
			errList = append(errList, fmt.Errorf("found more than one instance with instance ID %q", nodeName))
			return "", amerrors.NewAggregate(errList)
		}

		vpcID := output.Reservations[0].Instances[0].VpcId
		if vpcID != nil {
			return *vpcID, nil
		}

	}
	return "", amerrors.NewAggregate(errList)
}

func inferVPCIDFromTags(ec2Service services.EC2, VpcNameTagKey string, VpcNameTagValue string) (string, error) {
	vpcs, err := ec2Service.DescribeVPCsAsList(context.Background(), &ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + VpcNameTagKey),
				Values: []*string{aws.String(VpcNameTagValue)},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch VPC ID with tag: %w", err)
	}
	if len(vpcs) == 0 {
		return "", fmt.Errorf("no VPC exists with tag: %w", err)
	}
	if len(vpcs) > 1 {
		return "", fmt.Errorf("multiple VPCs exists with tag: %w", err)
	}

	return *vpcs[0].VpcId, nil
}

var _ services.Cloud = &defaultCloud{}

type defaultCloud struct {
	cfg CloudConfig

	ec2   services.EC2
	elbv2 services.ELBV2

	acm         services.ACM
	wafv2       services.WAFv2
	wafRegional services.WAFRegional
	shield      services.Shield
	rgt         services.RGT

	assumeRoleElbV2 map[string]services.ELBV2
	session         *session.Session
	awsCFG          *aws.Config
	logger          logr.Logger
}

// returns ELBV2 client for the given assumeRoleArn, or the default ELBV2 client if assumeRoleArn is empty
func (c *defaultCloud) GetAssumedRoleELBV2(assumeRoleArn string, externalId string) services.ELBV2 {

	if assumeRoleArn == "" {
		return c.elbv2
	}

	assumedRoleELBV2, exists := c.assumeRoleElbV2[assumeRoleArn]
	if exists {
		return assumedRoleELBV2
	}
	c.logger.Info("awsCloud", "method", "GetAssumedRoleELBV2", "AssumeRoleArn", assumeRoleArn, "externalId", externalId)
	creds := stscreds.NewCredentials(c.session, assumeRoleArn, func(p *stscreds.AssumeRoleProvider) {
		p.ExternalID = &externalId
	})

	c.awsCFG.Credentials = creds
	newObj := services.NewELBV2(c.session, c, c.awsCFG)
	c.assumeRoleElbV2[assumeRoleArn] = newObj

	return newObj
}

func (c *defaultCloud) EC2() services.EC2 {
	return c.ec2
}

func (c *defaultCloud) ELBV2() services.ELBV2 {
	return c.elbv2
}

func (c *defaultCloud) ACM() services.ACM {
	return c.acm
}

func (c *defaultCloud) WAFv2() services.WAFv2 {
	return c.wafv2
}

func (c *defaultCloud) WAFRegional() services.WAFRegional {
	return c.wafRegional
}

func (c *defaultCloud) Shield() services.Shield {
	return c.shield
}

func (c *defaultCloud) RGT() services.RGT {
	return c.rgt
}

func (c *defaultCloud) Region() string {
	return c.cfg.Region
}

func (c *defaultCloud) VpcID() string {
	return c.cfg.VpcID
}
