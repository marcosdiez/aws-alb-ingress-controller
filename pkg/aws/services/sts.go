package services

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
)

type STS interface {
	stsiface.STSAPI

	// wrapper on GetCallerIdentity
	GetCallerIdentityAsString() string
}

// NewSTS constructs new STS implementation.
func NewSTS(session *session.Session) *defaultSTS {
	return &defaultSTS{
		STSAPI: sts.New(session),
	}
}

type defaultSTS struct {
	stsiface.STSAPI
}

func (c *defaultSTS) GetCallerIdentityAsString() string {
	input := &sts.GetCallerIdentityInput{}
	result, err := c.GetCallerIdentity(input)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("Account: %s, ARN: %s, UserId: %s", *result.Account, *result.Arn, *result.UserId)
}
