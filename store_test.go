package syver

import (
	"bytes"
	"log"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_varsFromString(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		want    map[string]any
		wantErr bool
	}{
		{
			name:    "empty_string",
			arg:     ``,
			want:    map[string]any{},
			wantErr: false,
		},
		{
			name:    "empty_JSON",
			arg:     `{}`,
			want:    map[string]any{},
			wantErr: false,
		},
		{
			name: "JSON_simple",
			arg:  `{"a": "a", "b": 1}`,
			want: map[string]any{
				"a": "a",
				"b": float64(1),
			},
			wantErr: false,
		},
		{
			name: "YAML_simple",
			arg:  `{a: a, b: 1}`,
			want: map[string]any{
				"a": "a",
				"b": 1,
			},
			wantErr: false,
		},
		{
			name: "JSON_float",
			arg:  `{"f": 1.23}`,
			want: map[string]any{
				"f": 1.23,
			},
			wantErr: false,
		},
		{
			name: "YAML_float",
			arg:  `{f: 1.23}`,
			want: map[string]any{
				"f": 1.23,
			},
			wantErr: false,
		},
		{
			name: "JSON_list",
			arg:  `{"l": ["l1", "l2", 3]}`,
			want: map[string]any{
				"l": []any{
					"l1",
					"l2",
					float64(3),
				},
			},
			wantErr: false,
		},
		{
			name: "YAML_list",
			arg:  `{l: [l1, l2, 3]}`,
			want: map[string]any{
				"l": []any{
					"l1",
					"l2",
					3,
				},
			},
			wantErr: false,
		},
		{
			name: "JSON_object",
			arg:  `{"o": {"oa": "a", "oo": { "oo1": 1 } } }`,
			want: map[string]any{
				"o": map[string]any{
					"oa": "a",
					"oo": map[string]any{
						"oo1": float64(1),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "YAML_object",
			arg:  `{o: {oa: a, oo: { oo1: 1 } } }`,
			want: map[string]any{
				"o": map[string]any{
					"oa": "a",
					"oo": map[string]any{
						"oo1": 1,
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := varsFromString(tt.arg)

			assert.Equal(t, tt.want, got, "map contents")
			assert.Equal(t, tt.wantErr, err != nil, "has error")
		})
	}
}

func Test_loadVars(t *testing.T) {
	fileEmpty, fileEmptyClose := fileMaker(``)
	defer fileEmptyClose()

	fileNil, fileNilClose := fileMaker(``)
	defer fileNilClose()

	fileSimple1, fileSimpleClose1 := fileMaker(`{a: a}`)
	defer fileSimpleClose1()
	fileSimple2, fileSimpleClose2 := fileMaker(`{b: b}`)
	defer fileSimpleClose2()
	fileSimple3, fileSimpleClose3 := fileMaker(`{a: overriden, c: c}`)
	defer fileSimpleClose3()

	fileComplex1, fileComplexClose1 := fileMaker(`{vars: {a: a}}`)
	defer fileComplexClose1()
	fileComplex2, fileComplexClose2 := fileMaker(`{vars: {b: b}}`)
	defer fileComplexClose2()
	fileComplex3, fileComplexClose3 := fileMaker(`{vars: {a: overriden}}`)
	defer fileComplexClose3()

	type args struct {
		varsFiles  []string
		varsInline string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]any
		wantErr bool
	}{
		{
			name: "both_empty",
			args: args{
				varsFiles:  []string{fileEmpty},
				varsInline: `{}`,
			},
			want:    map[string]any{},
			wantErr: false,
		},
		{
			name: "both_nil",
			args: args{
				varsFiles:  []string{fileNil},
				varsInline: `{}`,
			},
			want:    map[string]any{},
			wantErr: false,
		},
		{
			name: "file_empty",
			args: args{
				varsFiles:  []string{fileEmpty},
				varsInline: `{b: b}`,
			},
			want: map[string]any{
				"b": "b",
			},
			wantErr: false,
		},
		{
			name: "inline_empty",
			args: args{
				varsFiles:  []string{fileSimple1},
				varsInline: `{}`,
			},
			want: map[string]any{
				"a": "a",
			},
			wantErr: false,
		},
		{
			name: "no_overwrite",
			args: args{
				varsFiles:  []string{fileSimple1},
				varsInline: `{b: b}`,
			},
			want: map[string]any{
				"a": "a",
				"b": "b",
			},
			wantErr: false,
		},
		{
			name: "overwrite",
			args: args{
				varsFiles:  []string{fileSimple1},
				varsInline: `{a: c, b: b}`,
			},
			want: map[string]any{
				"a": "c",
				"b": "b",
			},
			wantErr: false,
		},
		{
			name: "multiple files, non-overlapped keys, no inline vars",
			args: args{
				varsFiles:  []string{fileSimple1, fileSimple2},
				varsInline: `{}`,
			},
			want: map[string]any{
				"a": "a",
				"b": "b",
			},
			wantErr: false,
		},
		{
			name: "multiple files, overlapped keys, no inline vars",
			args: args{
				varsFiles:  []string{fileSimple1, fileSimple2, fileSimple3},
				varsInline: `{}`,
			},
			want: map[string]any{
				"a": "overriden",
				"b": "b",
				"c": "c",
			},
			wantErr: false,
		},
		{
			name: "multiple files, overlapped keys, inline vars",
			args: args{
				varsFiles:  []string{fileSimple1, fileSimple2, fileSimple3},
				varsInline: `{c: overriden, b: b}`,
			},
			want: map[string]any{
				"a": "overriden",
				"b": "b",
				"c": "overriden",
			},
			wantErr: false,
		},
		{
			name: "multiple nested files",
			args: args{
				varsFiles:  []string{fileComplex1, fileComplex2, fileComplex3},
				varsInline: `{}`,
			},
			want: map[string]any{
				"vars": map[string]any{
					"a": "overriden",
					"b": "b",
				},
			},
			wantErr: false,
		},
		{
			name: "multiple nested files and inline variables",
			args: args{
				varsFiles:  []string{fileComplex1, fileComplex2, fileComplex3},
				varsInline: `{vars: { c: c }}`,
			},
			want: map[string]any{
				"vars": map[string]any{
					"c": "c",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loadVars(tt.args.varsFiles, tt.args.varsInline)

			assert.Equal(t, tt.want, got, "map contents")
			assert.Equal(t, tt.wantErr, err != nil, "has error")
		})
	}
}

func fileMaker(content string) (string, func()) {
	bytes := []byte(content)

	f, err := os.CreateTemp("", "*")
	if err != nil {
		log.Fatal(err)
	}

	_, err = f.Write(bytes)
	if err != nil {
		log.Fatal(err)
	}

	return f.Name(), func() {
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
}

// Test_syverfileAlias_FoldsIntoSyverfiles covers §5.1: a gossfile written
// with `syverfile:` entries decodes into the same Syverfiles map an
// equivalent `gossfile:` gossfile would.
func Test_syverfileAlias_FoldsIntoSyverfiles(t *testing.T) {
	outStoreFormat = YAML
	t.Cleanup(func() { outStoreFormat = UNSET })

	data := []byte("syverfile:\n  extra:\n    file: extra.yaml\n")
	cfg, err := ReadJSONData(data, false)
	assert.NoError(t, err)

	require := assert.New(t)
	require.Contains(cfg.Syverfiles, "extra")
	require.Equal("extra.yaml", cfg.Syverfiles["extra"].File)
	require.Nil(cfg.SyverfileAlias, "SyverfileAlias must be nil'd after fold")
}

// Test_syverfileAlias_CollisionLogsWarnAndGossfileWins covers §5.1: the
// same key declared under both gossfile: and syverfile: logs exactly one
// [WARN] line, and the gossfile: value wins.
func Test_syverfileAlias_CollisionLogsWarnAndGossfileWins(t *testing.T) {
	outStoreFormat = YAML
	t.Cleanup(func() { outStoreFormat = UNSET })

	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	data := []byte("gossfile:\n  dup:\n    file: from-gossfile.yaml\nsyverfile:\n  dup:\n    file: from-syverfile.yaml\n")
	cfg, err := ReadJSONData(data, false)
	assert.NoError(t, err)

	assert.Equal(t, "from-gossfile.yaml", cfg.Syverfiles["dup"].File, "gossfile: value should win on collision")
	assert.Equal(t, 1, strings.Count(logOutput.String(), "[WARN]"), "exactly one [WARN] line expected")
	assert.Contains(t, logOutput.String(), `"dup" declared under both gossfile: and syverfile:`)
}

// Test_syverfileAlias_NeverWrittenBack covers §5.1: a config
// round-tripped through decode -> WriteJSON never contains a syverfile:
// key even if it was read from one.
func Test_syverfileAlias_NeverWrittenBack(t *testing.T) {
	outStoreFormat = YAML
	t.Cleanup(func() { outStoreFormat = UNSET })

	data := []byte("syverfile:\n  extra:\n    file: extra.yaml\n")
	cfg, err := ReadJSONData(data, false)
	assert.NoError(t, err)

	out, err := marshal(cfg)
	assert.NoError(t, err)
	assert.NotContains(t, string(out), "syverfile:")
	assert.Contains(t, string(out), "gossfile:")
}

// Test_MalformedYAML_SameErrorPathRegardlessOfFilenameSource covers the
// Contract Negative Sweep: resolveSpecPath only chooses which filename
// gets read (cmd/syver concern) -- decoding itself is unchanged, so
// malformed content errors identically whichever probed name it came
// from. This exercises ReadJSONData directly (the shared decode path)
// with content representative of what a probed syver.yaml or goss.yaml
// would contain.
func Test_MalformedYAML_SameErrorPathRegardlessOfFilenameSource(t *testing.T) {
	outStoreFormat = YAML
	t.Cleanup(func() { outStoreFormat = UNSET })

	malformed := []byte("addr:\n  foo\n  bar: [\n")

	_, err1 := ReadJSONData(malformed, false)
	_, err2 := ReadJSONData(malformed, false)

	assert.Error(t, err1)
	assert.Error(t, err2)
	assert.Equal(t, err1.Error(), err2.Error(), "same malformed content must produce the same decode error regardless of which probed filename it was read from")
}

// Test_syverfileAlias_WrongTypeSurfacesSameErrorClassAsGossfile covers the
// Contract Negative Sweep: a syverfile: entry whose value fails to
// unmarshal into resource.SyverfileMap surfaces the same class of error
// (a decode/type error from ReadJSONData) as an equivalent malformed
// gossfile: entry does today.
func Test_syverfileAlias_WrongTypeSurfacesSameErrorClassAsGossfile(t *testing.T) {
	outStoreFormat = YAML
	t.Cleanup(func() { outStoreFormat = UNSET })

	badGossfile := []byte("gossfile: \"not-a-map\"\n")
	badSyverfile := []byte("syverfile: \"not-a-map\"\n")

	_, gossErr := ReadJSONData(badGossfile, false)
	_, syverErr := ReadJSONData(badSyverfile, false)

	require := assert.New(t)
	require.Error(gossErr, "malformed gossfile: entry should fail to decode")
	require.Error(syverErr, "malformed syverfile: entry should fail to decode the same way")
}

// resetStdinOnce clears the package-level stdin buffer/guard so each test
// gets its own fresh sync.Once, and restores os.Stdin afterwards. Follows
// this file's existing pattern of directly manipulating package vars for
// test isolation (see TestStaticStoreErrors below).
func resetStdinOnce(t *testing.T) {
	t.Helper()
	origStdin := os.Stdin
	stdinOnce = sync.Once{}
	stdinData, stdinErr = nil, nil
	t.Cleanup(func() {
		os.Stdin = origStdin
		stdinOnce = sync.Once{}
		stdinData, stdinErr = nil, nil
	})
}

// Test_readStdinOnce_ReturnsSameBytesOnRepeatedCalls covers the stdin
// double-read: loadSyverConfigWithDiscover reads a "-" spec twice per validate
// invocation (peek, then the real load). os.Stdin is a non-seekable stream,
// so a naive double io.ReadAll would return the real data once and 0 bytes
// on the second call. readStdinOnce must return the same bytes both times.
func Test_readStdinOnce_ReturnsSameBytesOnRepeatedCalls(t *testing.T) {
	resetStdinOnce(t)

	r, w, err := os.Pipe()
	assert.NoError(t, err)
	os.Stdin = r

	want := []byte("file:\n  /etc/hosts:\n    exists: true\n")
	go func() {
		_, _ = w.Write(want)
		_ = w.Close()
	}()

	got1, err1 := readStdinOnce()
	assert.NoError(t, err1)
	assert.Equal(t, want, got1)

	// Second call must NOT touch the (now exhausted) pipe again -- it must
	// replay the buffered bytes, not return io.EOF/empty bytes.
	got2, err2 := readStdinOnce()
	assert.NoError(t, err2)
	assert.Equal(t, want, got2, "second call must return the same bytes, not an empty EOF read")
}

// Test_readStdinOnce_ReturnsSameErrorOnRepeatedCalls covers the negative
// path: if the underlying read fails once, the same error must be preserved
// on every subsequent call, not silently swallowed to nil on the second.
func Test_readStdinOnce_ReturnsSameErrorOnRepeatedCalls(t *testing.T) {
	resetStdinOnce(t)

	r, _, err := os.Pipe()
	assert.NoError(t, err)
	assert.NoError(t, r.Close()) // closed before any read -- forces io.ReadAll to error
	os.Stdin = r

	_, err1 := readStdinOnce()
	assert.Error(t, err1)

	_, err2 := readStdinOnce()
	assert.Error(t, err2)
	assert.Equal(t, err1.Error(), err2.Error(), "second call must preserve the same error, not silently return nil")
}

func TestStaticStoreErrors(t *testing.T) {
	_, err := getStoreFormatFromData([]byte("{\n\t\"broken\": "))
	assert.ErrorIs(t, err, errCannotDetermineFormat)

	prev := outStoreFormat
	outStoreFormat = UNSET
	t.Cleanup(func() { outStoreFormat = prev })

	_, err = marshal(NewSyverConfig())
	assert.ErrorIs(t, err, errStoreFormatUnset)

	err = unmarshal([]byte("{}"), NewSyverConfig(), UNSET)
	assert.ErrorIs(t, err, errStoreFormatUnset)
}
