package services

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/wafregional"
	"github.com/aws/aws-sdk-go/service/wafregional/wafregionaliface"
)

type WAFRegional interface {
	wafregionaliface.WAFRegionalAPI

	Available() bool
}

// NewWAFRegional constructs new WAFRegional implementation.
func NewWAFRegional(session *session.Session, region string, cfgs ...*aws.Config) WAFRegional {
	this_config := aws.NewConfig()
	if len(cfgs) > 0 {
		this_config = cfgs[0]
	}
	this_config.WithRegion(region)

	return &defaultWAFRegional{

		WAFRegionalAPI: wafregional.New(session, this_config),
		region:         region,
	}
}

// default implementation for WAFRegional.
type defaultWAFRegional struct {
	wafregionaliface.WAFRegionalAPI
	region string
}

func (c *defaultWAFRegional) Available() bool {
	resolver := endpoints.DefaultResolver()
	_, err := resolver.EndpointFor(wafregional.EndpointsID, c.region, endpoints.StrictMatchingOption)
	return err == nil
}
