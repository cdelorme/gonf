package multiconf

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

var mockError = errors.New("mock error")
var code int
var filedata string
var fileerror error

type mockLogger struct {
	Store string
}

func (self *mockLogger) Info(f string, args ...interface{})  { self.Store = fmt.Sprintf(f, args...) }
func (self *mockLogger) Debug(f string, args ...interface{}) { self.Store = fmt.Sprintf(f, args...) }

type mockConfig struct {
	Key      int    `json:"key,omitempty"`
	Name     string `json:"name,omitempty"`
	Password string `json:"-"`
	Extra    struct {
		Data    int  `json:"data,omitempty"`
		Correct bool `json:"correct,omitempty"`
	} `json:"extra,omitempty"`
	Final bool `json:"Final,omitempty"`
}

func (self mockConfig) String() string                { return "correct" }
func (self mockConfig) GoString() string              { return self.String() }
func (self *mockConfig) MarshalJSON() ([]byte, error) { return []byte(self.String()), nil }

func init() {
	stdout = ioutil.Discard
	exit = func(c int) { code = c }
	readfile = func(name string) ([]byte, error) { return []byte(filedata), fileerror }
}

func TestPlacebo(t *testing.T) {
	t.Parallel()
	if !true {
		t.FailNow()
	}
}

func TestInitLoad(t *testing.T) {
	os.Setenv("APPDATA", "testappdata")
	os.Setenv("XDG_CONFIG_DIR", "testxdgdir")
	os.Unsetenv("HOME")
	load()
	if len(paths) != 8 {
		t.FailNow()
	}

	os.Unsetenv("APPDATA")
	os.Unsetenv("XDG_CONFIG_DIR")
	load()
	if len(paths) != 5 {
		t.FailNow()
	}

	os.Setenv("HOME", "testhomedir")
	load()
	if len(paths) != 5 {
		t.FailNow()
	}
}

func TestMerge(t *testing.T) {
	t.Parallel()
	o := Config{}

	// maps to test merging and depth
	m1 := map[string]interface{}{"key": "value", "b": true, "deep": map[string]interface{}{"copy": "me"}, "fail": map[string]interface{}{"no": false}}
	m2 := map[string]interface{}{"key": "value2", "a": 1, "deep": map[string]interface{}{"next": "keypair"}, "fail": "test"}

	// acquire results /w assertions and validate
	v := o.merge(m1, m2)
	if v["key"] != "value2" || v["a"] != 1 || v["b"] != true || v["fail"] != "test" {
		t.FailNow()
	}
	if m, ok := v["deep"].(map[string]interface{}); !ok || m["next"] != "keypair" || m["copy"] != "me" {
		t.FailNow()
	}
}

func TestTo(t *testing.T) {
	t.Parallel()
	o := Config{Logger: &mockLogger{}}

	// set mock configuration
	c := &mockConfig{}
	o.Configuration = c

	// validate map casts to config correctly and ignores other values
	o.to(map[string]interface{}{"key": 123, "Key": "banana", "name": "hammock", "Final": true, "Extra": map[string]interface{}{"Data": "123"}})
	if c.Key != 123 || c.Name != "hammock" || c.Final != true || c.Extra.Data != 0 {
		t.FailNow()
	}
}

func TestEnv(t *testing.T) {
	t.Parallel()

	o := Config{}

	// good, bad, bad
	o.Env("env", "", "ENV")
	o.Env("", "", "ENV")
	o.Env("env", "", "")

	if len(o.envs) != 1 {
		t.Logf("%+v\n", o)
		t.FailNow()
	}
}

func TestParseEnv(t *testing.T) {
	o := Config{}

	// register some env vars
	o.Env("test", "", "MULTICONF_TEST_ENVVAR")

	// set env vars for testing parse
	os.Setenv("MULTICONF_TEST_ENVVAR", "nope")

	// parse env
	v := o.parseEnvs()

	// verify results
	if v["test"] != "nope" {
		t.FailNow()
	}
}

func TestOption(t *testing.T) {
	t.Parallel()

	o := Config{}

	// good
	o.Option("option", "", "-o", "--option")
	o.Option("option", "", "o", "option")
	o.Option("option", "", "op")

	// bad
	o.Option("", "-o", "", "--option")
	o.Option("option", "")

	// verify by count
	if len(o.long) != 3 || len(o.short) != 2 {
		t.FailNow()
	}
}

func TestParseOptions(t *testing.T) {
	o := &Config{}
	var v map[string]interface{}

	// register flags of all types
	o.Option("first", "", "--first", "-1")
	o.Option("greedy", "", "--greedy:", "-g:")
	o.Option("second", "", "--second", "-2")
	o.Option("third", "", "--third", "-3")
	o.Option("fourth", "", "--fourth", "-4:")
	o.Option("fifth", "", "--fifth", "-5")
	o.Option("sixth", "", "-6")

	// test long arguments
	os.Args = []string{"--first=hasvalue", "--second=", "--third", "misc", "ignored", "--fourth", "--greedy", "--first", "--fifth"}
	v = o.parseOptions()
	if v["first"] != "hasvalue" || v["second"] != true || v["third"] != "misc" || v["fourth"] != true || v["greedy"] != "--first" || v["fifth"] != true {
		t.FailNow()
	}

	// test bypass
	os.Args = []string{"--first=hasvalue", "--", "--first=skipped"}
	v = o.parseOptions()
	if v["first"] != "hasvalue" {
		t.FailNow()
	}

	// test bypass with greedy
	os.Args = []string{"--greedy", "--", "--greedy=skipped"}
	v = o.parseOptions()
	if v["greedy"] != true {
		t.FailNow()
	}

	// test short flags
	os.Args = []string{"-1", "-g", "-2", "ignored", "-23", "-45", "-5six", "-6", "seven"}
	v = o.parseOptions()
	if v["first"] != true || v["second"] != true || v["third"] != true || v["fourth"] != "5" || v["greedy"] != "-2" || v["fifth"] != "six" || v["sixth"] != "seven" {
		t.FailNow()
	}

	// test short triple-character scenario /w greedy
	os.Args = []string{"-142"}
	v = o.parseOptions()
	if _, ok := v["second"]; ok || v["first"] != true || v["fourth"] != "2" {
		t.FailNow()
	}

	// test combination short and long flag with greedy edge-case
	os.Args = []string{"-4", "--first"}
	v = o.parseOptions()
	if _, ok := v["first"]; ok || v["fourth"] != "--first" {
		t.FailNow()
	}

	// test all combinations of help
	o.Description = "Test"
	o.Example("Test")
	os.Args = []string{"help"}
	code = 1
	v = o.parseOptions()
	if code != 0 {
		t.FailNow()
	}
	os.Args = []string{"-h"}
	code = 1
	v = o.parseOptions()
	if code != 0 {
		t.FailNow()
	}
	os.Args = []string{"--help"}
	code = 1
	v = o.parseOptions()
	if code != 0 {
		t.FailNow()
	}
	o.Description = ""
	os.Args = []string{"--help"}
	code = 1
	v = o.parseOptions()
	if code != 1 {
		t.FailNow()
	}
}

func TestLoadConfig(t *testing.T) {
	l := &mockLogger{}
	o := Config{Logger: l}
	v := map[string]interface{}{}

	// test with error response
	o.paths = []string{"nope"}
	fileerror = mockError
	v = o.loadConfig()
	if len(v) > 0 {
		t.FailNow()
	}
	fileerror = nil

	// test with invalid json
	filedata = `not json`
	v = o.loadConfig()
	if len(v) > 0 || !strings.HasPrefix(l.Store, "failed to parse") {
		t.FailNow()
	}

	// test with valid json
	filedata = `{
		"key": 123,
		"name": "casey",
		"extra": {
			"data": 123,
			"correct": true
		},
		"Final": true
	}`
	v = o.loadConfig()
	if v["name"] != "casey" || v["Final"] != true || v["key"] != float64(123) {
		t.FailNow()
	}
}

func TestLoad(t *testing.T) {
	l := &mockLogger{}
	c := &mockConfig{}
	o := Config{Logger: l, Configuration: c}

	// override readfile data and load
	filedata = `{}`
	o.Load()

	// verify log output
	if !strings.HasPrefix(l.Store, "Configuration: correct") {
		t.FailNow()
	}
}

func TestHelp(_ *testing.T) {
	o := Config{}
	o.Help()
}