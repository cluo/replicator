package api

import (
	"reflect"
	"testing"

	"github.com/hashicorp/consul/testutil"
)

func TestReadConsulKVScalingDocument_correctSingleDocument(t *testing.T) {

	srv1 := testutil.NewTestServer(t)
	defer srv1.Stop()

	c, err := NewConsulClient(srv1.HTTPAddr)
	if err != nil {
		t.Fatal(err)
	}

	srv1.SetKV("replicator/config/jobs/test1", []byte(`{"enabled":true,"groups":[{"name":"group1","scaling":{"min":3,"max":10,"scaleout":{"cpu":80,"mem":90},"scalein":{"cpu":30,"mem":40}}}]}`))

	var expected []*JobScalingPolicy
	var group []*GroupScalingPolicy

	g := &GroupScalingPolicy{
		GroupName: "group1",

		Scaling: &Scaling{
			Min:            3,
			Max:            10,
			ScaleDirection: "",

			ScaleIn: &scalein{
				CPU: 30,
				MEM: 40,
			},
			ScaleOut: &scaleout{
				CPU: 80,
				MEM: 90,
			},
		},
	}

	group = append(group, g)

	e := &JobScalingPolicy{
		JobName: "test1",
		Enabled: true,

		GroupScalingPolicies: group,
	}

	expected = append(expected, e)

	resp, err := c.ListConsulKV("", "replicator/config/jobs")
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(resp, expected) {
		t.Fatalf("expected \n%#v\n\n, got \n\n%#v\n\n", expected, resp)
	}
}
