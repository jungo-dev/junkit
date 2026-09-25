// External test package: frames of package sqlfmt itself are skipped by Origin.
package sqlfmt_test

import (
	"strings"
	"testing"

	"github.com/jungo-dev/junkit/sqlfmt"
)

func TestOrigin_PointsAtTheCaller(t *testing.T) {
	location, function := sqlfmt.Origin()

	if !strings.Contains(location, "origin_test.go:") {
		t.Errorf("location = %q, want this test file", location)
	}
	if !strings.HasSuffix(function, "TestOrigin_PointsAtTheCaller") {
		t.Errorf("function = %q, want this test function", function)
	}
}
