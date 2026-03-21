package concepts_test

import (
	"fmt"
	"testing"

	c1 "github.com/buildkite/cli/v3/internal/preflight/concepts/1"
	c2 "github.com/buildkite/cli/v3/internal/preflight/concepts/2"
	c3 "github.com/buildkite/cli/v3/internal/preflight/concepts/3"
	c4 "github.com/buildkite/cli/v3/internal/preflight/concepts/4"
	c5 "github.com/buildkite/cli/v3/internal/preflight/concepts/5"
)

func TestConcept1_LabNotebook(t *testing.T)    { fmt.Println(c1.Demo()) }
func TestConcept2_Oscilloscope(t *testing.T)   { fmt.Println(c2.Demo()) }
func TestConcept3_Redline(t *testing.T)        { fmt.Println(c3.Demo()) }
func TestConcept3_RedlinePassing(t *testing.T) { fmt.Println(c3.DemoPassing()) }
func TestConcept4_Spectrograph(t *testing.T)   { fmt.Println(c4.Demo()) }
func TestConcept5_Aperture(t *testing.T)       { fmt.Println(c5.Demo()) }
func TestConcept5_ApertureRunning(t *testing.T) { fmt.Println(c5.DemoRunning()) }
func TestConcept5_AperturePassed(t *testing.T) { fmt.Println(c5.DemoPassed()) }
