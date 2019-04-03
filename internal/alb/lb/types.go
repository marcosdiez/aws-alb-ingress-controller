package lb

// LoadBalancer contains information of LoadBalancer in AWS
type LoadBalancer struct {
	Arn     string
	DNSName string
}

// NameGenerator generates name for loadBalancer resources
type NameGenerator interface {
	NameLB(namespace string, ingressName string) string
}

// TagGenerator generates tags for loadBalancer resources
type TagGenerator interface {
	TagLB(namespace string, ingressName string, usingOnlyOneAlb bool) map[string]string
}

// NameTagGenerator combines NameGenerator & TagGenerator
type NameTagGenerator interface {
	NameGenerator
	TagGenerator
}
