package matchers

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestToNumeric pins how to-numeric reads the values commands and registry
// lookups hand it. Registry and command output routinely carries surrounding
// whitespace and a trailing newline, and a spec compares that against a plain
// YAML number, so the trimming is part of the contract.
func TestToNumeric(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      interface{}
		want    interface{}
		wantErr string
	}{
		{name: "plain integer string", in: "42", want: float64(42)},
		{name: "leading and trailing spaces", in: "  42  ", want: float64(42)},
		{name: "tab", in: "\t42", want: float64(42)},
		{name: "trailing LF", in: "42\n", want: float64(42)},
		{name: "trailing CRLF", in: "42\r\n", want: float64(42)},
		{name: "float", in: "3.25", want: float64(3.25)},
		{name: "float with CRLF", in: " 3.25\r\n", want: float64(3.25)},
		{name: "negative", in: "-7", want: float64(-7)},
		{name: "exponent", in: "1e3", want: float64(1000)},
		{name: "hex is not accepted", in: "0x10", wantErr: "invalid syntax"},
		{name: "non-numeric", in: "abc", wantErr: "invalid syntax"},
		{name: "empty", in: "", wantErr: "invalid syntax"},
		{name: "whitespace only", in: " \r\n", wantErr: "invalid syntax"},
		{name: "float64 passes through", in: float64(1.5), want: float64(1.5)},
		{name: "int passes through", in: 3, want: 3},
		{name: "single-line []string", in: []string{"5"}, want: float64(5)},
		{name: "[]string with trailing empty line", in: []string{"5", ""}, want: float64(5)},
		{name: "multi-line []string is not a number", in: []string{"5", "6"}, wantErr: "invalid syntax"},
		{name: "unsupported type", in: true, wantErr: "expected numeric, Got:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToNumeric{}.Transform(tt.in)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestToNumericAgainstYAMLInt is the end-to-end shape a spec produces: a
// registry string with trailing whitespace, transformed and compared against
// an int decoded from YAML.
func TestToNumericAgainstYAMLInt(t *testing.T) {
	t.Parallel()

	for _, actual := range []string{"1", "1\n", "1\r\n", " 1 "} {
		m := WithSafeTransform(ToNumeric{}, BeNumerically("eq", 1))
		ok, err := m.Match(actual)
		require.NoError(t, err, "actual %q", actual)
		assert.True(t, ok, "actual %q should equal 1 after to-numeric", actual)
	}

	m := WithSafeTransform(ToNumeric{}, BeNumerically("eq", 1))
	ok, err := m.Match("2\r\n")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestToString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   interface{}
		want string
	}{
		{name: "string", in: "a", want: "a"},
		{name: "int", in: 3, want: "3"},
		{name: "float", in: 1.5, want: "1.5"},
		{name: "bool", in: true, want: "true"},
		{name: "[]string joins with newline", in: []string{"a", "b"}, want: "a\nb"},
		{name: "[]interface{} joins with newline", in: []interface{}{"a", 2, true}, want: "a\n2\ntrue"},
		{name: "empty []string", in: []string{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToString{}.Transform(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToArray(t *testing.T) {
	t.Parallel()

	got, err := ToArray{}.Transform("a\nb")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, got)

	in := []string{"x"}
	got, err = ToArray{}.Transform(in)
	require.NoError(t, err)
	assert.Equal(t, in, got)
}

func TestReaderToString(t *testing.T) {
	t.Parallel()

	got, err := ReaderToString{}.Transform(strings.NewReader("hello\n"))
	require.NoError(t, err)
	assert.Equal(t, "hello\n", got)

	_, err = ReaderToString{}.Transform("not a reader")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected io.reader")
}

func TestGjson(t *testing.T) {
	t.Parallel()

	got, err := Gjson{Path: "a.b"}.Transform(`{"a":{"b":7}}`)
	require.NoError(t, err)
	assert.Equal(t, float64(7), got)

	_, err = Gjson{Path: "a"}.Transform(42)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected string")

	_, err = Gjson{Path: "a"}.Transform(`{"a":`)
	assert.True(t, errors.Is(err, errInvalidJSON), "got %v", err)

	_, err = Gjson{Path: "missing"}.Transform(`{"a":1}`)
	require.Error(t, err)
	assert.Equal(t, "path not found: missing", err.Error())
}

// TestTransformerMarshalJSON pins the transform-chain shape that appears in
// json output.
func TestTransformerMarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   json.Marshaler
		want string
	}{
		{ToNumeric{}, `{"to-numeric":{}}`},
		{ToString{}, `{"to-string":{}}`},
		{ToArray{}, `{"to-array":{}}`},
		{Gjson{Path: "a.b"}, `{"gjson":{"Path":"a.b"}}`},
	}
	for _, tt := range tests {
		b, err := json.Marshal(tt.in)
		require.NoError(t, err)
		assert.JSONEq(t, tt.want, string(b))
	}
}
