package aliyun

import (
	"testing"
)

func TestParseSLBTagsContentFormats(t *testing.T) {
	a := []byte(`{"TagResources":[{"ResourceId":"lb-1","Tags":[{"Key":"CodeName","Value":"api-gw"}]}]}`)
	m := parseSLBTagsContent(a)
	if m["lb-1"] != "api-gw" {
		t.Fatalf("fmtA")
	}
	b := []byte(`{"TagResources":{"TagResource":[{"ResourceId":"lb-2","TagKey":"CodeName","TagValue":"web"}]}}`)
	m2 := parseSLBTagsContent(b)
	if m2["lb-2"] != "web" {
		t.Fatalf("fmtB")
	}
	c := []byte(`{"TagResources":[{"ResourceARN":"arn:acs:slb:cn:uid:loadbalancer/lb-3","Tags":[{"TagKey":"code_name","TagValue":"svc"}]}]}`)
	m3 := parseSLBTagsContent(c)
	if m3["lb-3"] != "svc" {
		t.Fatalf("arn")
	}
}
