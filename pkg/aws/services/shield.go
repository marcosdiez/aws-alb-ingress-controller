package services

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/shield"
	"github.com/aws/aws-sdk-go/service/shield/shieldiface"
)

type Shield interface {
	shieldiface.ShieldAPI
}

// NewShield constructs new Shield implementation.
func NewShield(session *session.Session, cfgs ...*aws.Config) Shield {
	this_config := aws.NewConfig()
	if len(cfgs) > 0 {
		this_config = cfgs[0]
	}
	// shield is only available as a global API in us-east-1.
	this_config.WithRegion("us-east-1")

	return &defaultShield{
		ShieldAPI: shield.New(session, this_config),
	}
}

// default implementation for Shield.
type defaultShield struct {
	shieldiface.ShieldAPI
}
