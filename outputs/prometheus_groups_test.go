package outputs

import (
	"bytes"
	"testing"
	"time"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
)

// Results arrive in groups, one per resource. An early failing group must
// keep the run's outcome at fail even when later groups pass.
func TestPrometheusRunOutcomeAcrossGroups(t *testing.T) {
	defer resetMetrics()

	groups := [][]resource.TestResult{
		{{ResourceType: "File", Duration: time.Millisecond, Result: resource.FAIL}},
		{{ResourceType: "Command", Duration: time.Millisecond, Result: resource.SUCCESS}},
		{{ResourceType: "Command", Duration: time.Millisecond, Result: resource.SKIP}},
	}
	ch := make(chan []resource.TestResult, len(groups))
	for _, g := range groups {
		ch <- g
	}
	close(ch)

	buf := &bytes.Buffer{}
	assert.Equal(t, 0, Prometheus{}.Output(buf, ch, util.OutputConfig{}))

	out := buf.String()
	assert.Contains(t, out, `goss_tests_run_outcomes_total{outcome="fail"} 1`)
	assert.Contains(t, out, `syver_tests_run_outcomes_total{outcome="fail"} 1`)
	assert.NotContains(t, out, `goss_tests_run_outcomes_total{outcome="pass"}`)
}
