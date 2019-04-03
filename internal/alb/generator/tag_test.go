package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_TagIngress(t *testing.T) {
	gen := TagGenerator{
		ClusterName: "cluster",
		DefaultTags: map[string]string{
			"key": "value",
		},
	}
	expected := map[string]string{
		"kubernetes.io/cluster/cluster": "owned",
		TagKeyIngressName:               "ingress",
		TagKeyNamespace:                 "namespace",
		"key":                           "value",
	}

	expected_single_load_balancer := map[string]string{
		"kubernetes.io/cluster/cluster": "owned",
		"key":                           "value",
	}
	assert.Equal(t, gen.TagLB("namespace", "ingress", false), expected)
	assert.Equal(t, gen.TagLB("namespace", "ingress", true), expected_single_load_balancer)
	assert.Equal(t, gen.TagTGGroup("namespace", "ingress"), expected)
}

func Test_TagTG(t *testing.T) {
	gen := TagGenerator{}
	expected := map[string]string{
		TagKeyServiceName: "service",
		TagKeyServicePort: "port",
	}
	assert.Equal(t, gen.TagTG("service", "port"), expected)
}
