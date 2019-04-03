package generator

import "github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/config"

type NameTagGenerator struct {
	NameGenerator
	TagGenerator
}

func NewNameTagGenerator(cfg config.Configuration) *NameTagGenerator {
	return &NameTagGenerator{
		NameGenerator{
			ALBNamePrefix: cfg.ALBNamePrefix,
			ForceALBName: cfg.ForceALBName,
		},
		TagGenerator{
			ClusterName: cfg.ClusterName,
			DefaultTags: cfg.DefaultTags,
		},
	}
}
