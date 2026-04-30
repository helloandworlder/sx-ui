package controller

import "testing"

const baseXrayConfigForRuntimeApply = `{
  "log": {"loglevel": "warning"},
  "api": {"tag": "api"},
  "inbounds": [],
  "outbounds": [{"tag":"direct","protocol":"freedom","settings":{}}],
  "routing": {"domainStrategy":"AsIs","rules":[{"type":"field","outboundTag":"direct","ip":["geoip:private"]}]},
  "reverse": {"portals":[{"tag":"portal-a","domain":"example.com"}]}
}`

func TestApplyRuntimeOnlyChangesAppliesOutboundsRoutingReverseWithoutRestart(t *testing.T) {
	newConfig := `{
  "log": {"loglevel": "warning"},
  "api": {"tag": "api"},
  "inbounds": [],
  "outbounds": [{"tag":"proxy","protocol":"freedom","settings":{}}],
  "routing": {"domainStrategy":"AsIs","rules":[{"type":"field","outboundTag":"proxy","ip":["geoip:private"]}]},
  "reverse": {"portals":[{"tag":"portal-b","domain":"example.com"}]}
}`
	plan, err := buildRuntimeApplyPlan(baseXrayConfigForRuntimeApply, newConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.runtimeOnly {
		t.Fatal("expected runtime-only changes to apply")
	}
	if !plan.changedOutbounds || !plan.changedRouting || !plan.changedReverse {
		t.Fatalf("expected all runtime sections changed, got outbounds=%v routing=%v reverse=%v", plan.changedOutbounds, plan.changedRouting, plan.changedReverse)
	}
}

func TestApplyRuntimeOnlyChangesRejectsNonRuntimeSectionChange(t *testing.T) {
	newConfig := `{
  "log": {"loglevel": "debug"},
  "api": {"tag": "api"},
  "inbounds": [],
  "outbounds": [{"tag":"direct","protocol":"freedom","settings":{}}],
  "routing": {"domainStrategy":"AsIs","rules":[{"type":"field","outboundTag":"direct","ip":["geoip:private"]}]},
  "reverse": {"portals":[{"tag":"portal-a","domain":"example.com"}]}
}`
	plan, err := buildRuntimeApplyPlan(baseXrayConfigForRuntimeApply, newConfig)
	if err != nil {
		t.Fatal(err)
	}
	if plan.runtimeOnly {
		t.Fatal("expected non-runtime section change to require normal restart path")
	}
}
