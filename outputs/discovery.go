package outputs

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/krameff/syver/util"
)

type Discovery struct{}

func (r Discovery) ValidOptions() []*formatOption {
	return []*formatOption{
		{name: foPretty},
	}
}

func (r Discovery) Output(w io.Writer, discovered map[string]bool, outConfig util.OutputConfig) int {
	pretty := util.IsValueInList(foPretty, outConfig.FormatOptions)

	out := map[string]any{
		"Discovered": discovered,
	}

	var payload []byte
	var err error
	if pretty {
		payload, err = json.MarshalIndent(out, "", "    ")
	} else {
		payload, err = json.Marshal(out)
	}
	if err != nil {
		fmt.Fprintf(w, "failed to marshal discovery output: %v\n", err)
		return 1
	}

	fmt.Fprintln(w, string(payload))
	return 0
}
