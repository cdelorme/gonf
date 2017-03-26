package gonf

import (
	"errors"
	"fmt"
	// "io"
	"io/ioutil"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

type mockLogger struct {
	Store string
}

func (self *mockLogger) Info(f string, args ...interface{})  { self.Store = fmt.Sprintf(f, args...) }
func (self *mockLogger) Debug(f string, args ...interface{}) { self.Store = fmt.Sprintf(f, args...) }

// composite object demonstrating that "unnamed" composition
// receives properties and casting is correctly handled
type mockCompositeConfig struct {
	Implicit bool
	Tagged   int `json:"implicit,omitempty"`
	Conflict int `json:"conflict,omitempty"`
}

// functions that validate dynamic behavior
// and demonstrate fmt property protection
func (self mockCompositeConfig) String() string                { return "correct" }
func (self mockCompositeConfig) GoString() string              { return self.String() }
func (self *mockCompositeConfig) MarshalJSON() ([]byte, error) { return []byte(self.String()), nil }
func (self *mockCompositeConfig) Callback()                    {}

// mockStat (shamelessly "borrowed" from `types_unix.go`)
type mockStat struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	sys     interface{}
	dir     bool
}

func (self *mockStat) Name() string       { return self.name }
func (self *mockStat) Size() int64        { return self.size }
func (self *mockStat) IsDir() bool        { return self.dir }
func (self *mockStat) Mode() os.FileMode  { return self.mode }
func (self *mockStat) ModTime() time.Time { return self.modTime }
func (self *mockStat) Sys() interface{}   { return &self.sys }

// parent level structure demonstrating the use of composition
// to induce dynamic functionality (locking & logging)
// while also demonstrating correct handling of property conflicts
// when dealing with implicit or unnamed composite structures
type mockConfig struct {
	sync.Mutex
	mockLogger
	mockCompositeConfig

	Conflict string  `json:"conflict,omitempty"`
	Name     string  `json:"name,omitempty"`
	Number   float32 `json:"number,omitempty"`
	Boolean  bool    `json:"boolean,omitempty"`
	Ignored  bool    `json:"-"`

	Named struct {
		Data int `json:"data,omitempty"`
	} `json:"named,omitempty"`
}

var (
	code          int
	filedata      string
	mockError     = errors.New("mock error")
	fileerror     error
	createerror   error
	mockFileStat  = &mockStat{modTime: time.Now()}
	mockStatError error
)

func TestConfigMerge(t *testing.T) {
	o := &Config{}

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

func TestConfigCast(t *testing.T) {
	g := &Config{Target: &mockConfig{}}

	// prepare map matching config struct to verify types after
	m := map[string]interface{}{
		"name":     "casey",
		"number":   "15.9",
		"boolean":  "true",
		"conflict": "42",
		"named":    map[string]interface{}{"data": "42"},
	}

	g.cast(g.Target, m, map[string]interface{}{})
	if d, e := m["number"].(float64); !e || d != 15.9 {
		t.FailNow()
	} else if d, e := m["boolean"].(bool); !e || !d {
		t.FailNow()
	} else if s, ok := m["conflict"].(string); !ok || s != "42" {
		t.FailNow()
	} else if d, e := m["named"]; !e {
		t.FailNow()
	} else if tm, ok := d.(map[string]interface{}); !ok {
		t.FailNow()
	} else if v, ok := tm["data"]; !ok {
		t.FailNow()
	} else if f, ok := v.(float64); !ok || f != 42 {
		t.FailNow()
	}
}

func TestConfigTo(t *testing.T) {
	c := &mockConfig{}
	o := &Config{Target: c}

	// map with edge cases to validate expected behavior
	m := map[string]interface{}{
		"number":  15.9,
		"Number":  "banana",
		"name":    "hammock",
		"Boolean": true,
		"extra":   "pay me no mind",
		"named":   map[string]interface{}{"Data": 123},
	}

	// populate configuration and validate properties
	o.to(m)
	if c.Number != 15.9 {
		t.FailNow()
	} else if c.Name != "hammock" {
		t.FailNow()
	} else if c.Boolean != true {
		t.FailNow()
	} else if c.Named.Data != 123 {
		t.FailNow()
	}
}

func TestConfigSet(t *testing.T) {
	o := &Config{}
	m := map[string]interface{}{"x": false}

	// test single key/value
	o.set(m, "key", "value")
	if m["key"] != "value" {
		t.FailNow()
	}

	// test depth via period and number
	o.set(m, "go.deeper", 123)
	if d, ok := m["go"]; !ok {
		t.FailNow()
	} else if tm, ok := d.(map[string]interface{}); !ok {
		t.FailNow()
	} else if v, ok := tm["deeper"]; !ok {
		t.FailNow()
	} else if f, ok := v.(int); !ok || f != 123 {
		t.FailNow()
	}

	// test depth via period using override
	o.set(m, "x.truthy", true)
	if d, ok := m["x"]; !ok {
		t.FailNow()
	} else if tm, ok := d.(map[string]interface{}); !ok {
		t.FailNow()
	} else if v, ok := tm["truthy"]; !ok {
		t.FailNow()
	} else if f, ok := v.(bool); !ok || !f {
		t.FailNow()
	}
}

func TestConfigParseEnvs(t *testing.T) {
	os.Clearenv()
	os.Args = []string{}
	o := &Config{}

	// register multiple settings
	o.Add("test", "", "MULTICONF_TEST_ENVVAR")
	o.Add("testing.depth", "", "MULTICONF_TEST_DEPTH")
	o.Add("testing.depth.deeper", "", "MULTICONF_TEST_DEEPER")
	o.Add("empty", "", "", "-e")

	// set env vars for testing parse
	os.Setenv("MULTICONF_TEST_ENVVAR", "narp")
	os.Setenv("MULTICONF_TEST_DEPTH", "yarp")
	os.Setenv("MULTICONF_TEST_DEEPER", "yarp")

	// parse env
	v := o.parseEnvs()

	// verify results
	if v["test"] != "narp" {
		t.FailNow()
	}
	if _, ok := v["testing"]; !ok {
		t.FailNow()
	}
}

func TestConfigPrivateHelp(t *testing.T) {
	exit = func(c int) { code = c }
	fmtPrintf = func(f string, a ...interface{}) (int, error) { return fmt.Fprintf(ioutil.Discard, f, a...) }

	o := &Config{}
	o.Add("cli", "test help cli flag", "", "-c", "--cli")
	o.Add("env", "test help env", "env")
	o.Add("both", "test help env and cli", "both", "-b", "--both")
	o.Example("test help example")

	// test without description
	code = 1
	o.help(true)
	if code != 1 {
		t.FailNow()
	}

	// test with description
	o.Description = "testing help"
	code = 1
	o.help(true)
	if code != 0 {
		t.FailNow()
	}
}

func TestConfigParseLong(t *testing.T) {
	g := &Config{}

	// register all combinations of flags
	g.Add("first", "", "", "--first")
	g.Add("greedy", "", "", "--greedy:")
	g.Add("second", "", "", "--second")
	g.Add("test.depth", "", "", "--depth")
	g.Add("bad.depth.", "", "", "--bad")
	g.Add("also..bad", "", "", "--also")
	g.Add("test.depth.deeper", "", "", "--deeper")

	var m map[string]interface{}

	// test bypass
	os.Args = []string{"--first", "--", "--first=skipped"}
	m = g.parseOptions()
	if m["first"] != true {
		t.FailNow()
	}

	// test bypass with greedy
	os.Args = []string{"--greedy", "--", "--greedy=skipped"}
	m = g.parseOptions()
	if m["greedy"] != true {
		t.FailNow()
	}

	// test depth support
	os.Args = []string{"--depth", "--deeper", "--bad", "--also"}
	m = g.parseOptions()
	if _, ok := m["test"]; !ok {
		t.FailNow()
	}

	// sunny-day scenario
	os.Args = []string{"--first=hasvalue", "--second", "hasvalue", "--greedy", "--eats-objects"}
	m = g.parseOptions()
	if m["first"] != "hasvalue" || m["second"] != "hasvalue" || m["greedy"] != "--eats-objects" {
		t.FailNow()
	}
}

func TestConfigParseShort(t *testing.T) {
	g := &Config{}
	g.Add("first", "", "", "-f")
	g.Add("greedy", "", "", "-g:")
	g.Add("second", "", "", "-2")
	g.Add("test.depth", "", "", "-d")

	var m map[string]interface{}

	// with bypass
	os.Args = []string{"-f", "--", "-2"}
	m = g.parseOptions()
	if _, ok := m["second"]; ok || m["first"] != true {
		t.FailNow()
	}

	// greedy with bypass
	os.Args = []string{"-g", "--", "-2"}
	m = g.parseOptions()
	if _, ok := m["second"]; ok || m["greedy"] != true {
		t.FailNow()
	}

	// combination of flags starting with greedy
	os.Args = []string{"-gf2"}
	m = g.parseOptions()
	if len(m) != 1 || m["greedy"] != "f2" {
		t.FailNow()
	}

	// combination of flags
	os.Args = []string{"-f2d"}
	m = g.parseOptions()
	if _, ok := m["test"]; !ok || m["first"] != true || m["second"] != true {
		t.FailNow()
	}

	// combination of flags ending in greedy
	os.Args = []string{"-f2g"}
	m = g.parseOptions()
	if m["first"] != true || m["second"] != true || m["greedy"] != true {
		t.FailNow()
	}

	// combination with separate for final property
	os.Args = []string{"-f2", "yarp"}
	m = g.parseOptions()
	if m["first"] != true || m["second"] != "yarp" {
		t.FailNow()
	}

	// combination ending with greedy with separate for final property
	os.Args = []string{"-f2g", "yarp"}
	m = g.parseOptions()
	if m["first"] != true || m["second"] != true || m["greedy"] != "yarp" {
		t.FailNow()
	}
}

func TestConfigParseOptions(t *testing.T) {
	fmtPrintf = func(f string, a ...interface{}) (int, error) { return fmt.Fprintf(ioutil.Discard, f, a...) }
	os.Clearenv()

	g := &Config{Description: "testing parse options"}
	g.Add("key", "test", "", "-k", "--key")
	var m map[string]interface{}

	// test bad-single-skip and bypass
	os.Args = []string{"-", "--"}
	m = g.parseOptions()
	if len(m) != 0 {
		t.FailNow()
	}

	// test help in all three standard forms
	code, os.Args = 1, []string{"help"}
	m = g.parseOptions()
	if code != 0 {
		t.FailNow()
	}

	code, os.Args = 1, []string{"-h"}
	m = g.parseOptions()
	if code != 0 {
		t.FailNow()
	}

	code, os.Args = 1, []string{"--help"}
	m = g.parseOptions()
	if code != 0 {
		t.FailNow()
	}

	// test last help format without a description
	g.Description = ""
	m = g.parseOptions()
	if m == nil {
		t.FailNow()
	}

	// test invalid
	os.Args = []string{"blah"}
	m = g.parseOptions()
	if len(m) != 0 {
		t.FailNow()
	}

	// test short and long
	os.Args = []string{"-k", "--key"}
	m = g.parseOptions()
	if m["key"] != true {
		t.FailNow()
	}
}

func TestConfigComment(t *testing.T) {
	g := Config{}

	before := []byte(`{
		// remove this
		//remove this
		"key": " // keep this" // remove this
		/ / keep bad syntax
/*
		"removed": "this is removed"
		*/
		"keep": " /* this is to be kept*/"
		/*"termination": "can happen inside quotes*/"
	}`)
	after := []byte(`{
						"key": " // keep this" 		/ / keep bad syntax

		"keep": " /* this is to be kept*/"
		"
	}`)

	// verify that we strip single and multi-line comments outside quotes
	if o := g.comment(before); string(o) != string(after) {
		t.FailNow()
	}
}

func TestConfigReadfile(t *testing.T) {
	stat = func(_ string) (os.FileInfo, error) { return mockFileStat, mockStatError }
	readfile = func(name string) ([]byte, error) { return []byte(filedata), fileerror }
	o := &Config{Target: &mockConfig{}}

	// test successful return
	filedata = `{
			"key": 123,
			"name": "casey",
			"extra": {
				"data": 123,
				"correct": true
			},
			"Final": true
		}`
	if v, e := o.readfile(); e != nil || len(v) == 0 {
		t.FailNow()
	}

	// test json parsing error
	filedata = `not valid json`
	o.configModified = time.Time{}
	if v, e := o.readfile(); e == nil || len(v) != 0 {
		t.FailNow()
	}

	// test readfile error
	fileerror = mockError
	o.configModified = time.Time{}
	if v, e := o.readfile(); e == nil || len(v) != 0 {
		t.FailNow()
	}

	// test configModified
	o.configModified = mockFileStat.modTime
	if v, e := o.readfile(); e != nil || len(v) != 0 {
		t.FailNow()
	}
}

func TestConfigParseFiles(t *testing.T) {
	stat = func(_ string) (os.FileInfo, error) { return mockFileStat, mockStatError }
	readfile = func(name string) ([]byte, error) { return []byte(filedata), fileerror }
	o := &Config{Target: &mockConfig{}}
	paths = []string{"/tmp/nope"}

	// test no files (empty data)
	fileerror = mockError
	if v := o.parseFiles(appName); len(v) > 0 {
		t.FailNow()
	}

	// test file match /w valid filedata
	fileerror = nil
	filedata = `{
			"key": 123,
			"name": "casey",
			"extra": {
				"data": 123,
				"correct": true
			},
			"Final": true
		}`
	o.configFile = ""
	if v := o.parseFiles(appName); len(v) == 0 {
		t.FailNow()
	}

	// test from existing configFile
	o.configModified = time.Time{}
	if v := o.parseFiles(appName); len(v) == 0 {
		t.FailNow()
	}
}

func TestConfigPublicReload(_ *testing.T) {
	stat = func(_ string) (os.FileInfo, error) { return mockFileStat, mockStatError }
	readfile = func(name string) ([]byte, error) { return []byte(filedata), fileerror }
	g := &Config{Target: &mockConfig{}}

	// without config file
	g.Reload()

	// with config file
	g.configFile = "/tmp/test.gonf.json"
	g.Reload()
}

func TestConfigPublicSave(_ *testing.T) {
	mkdirall = func(_ string, _ os.FileMode) error { return nil }
	create = func(_ string) (*os.File, error) { return nil, createerror }
	g := &Config{Target: &mockConfig{}}

	// test empty configuration file
	g.Save()

	// test valid configuration file
	g.configFile = "/tmp/gonf"
	g.Save()

	// test with save fail
	createerror = mockError
	g.Save()
}

func TestConfigPublicLoad(t *testing.T) {
	exit = func(c int) { code = c }
	fmtPrintf = func(f string, a ...interface{}) (int, error) { return fmt.Fprintf(ioutil.Discard, f, a...) }
	stat = func(_ string) (os.FileInfo, error) { return mockFileStat, mockStatError }
	readfile = func(name string) ([]byte, error) { return []byte(filedata), fileerror }
	mkdirall = func(_ string, _ os.FileMode) error { return nil }
	create = func(_ string) (*os.File, error) { return nil, createerror }
	goos = "linux"

	c := &mockConfig{Name: "casey"}
	g := &Config{Target: c}
	g.Add("name", "test-overrides-from-public-load", "TEST_NAME", "-a:")

	// clear all inputs
	filedata = ""
	os.Clearenv()
	os.Args = []string{}

	// verify defaults remain with no contents
	g.Load()
	if c.Name != "casey" {
		t.FailNow()
	}

	// test passing a signal
	g.sighup <- syscall.SIGHUP

	// verify file overrides default
	filedata = `{"name": "banana"}`
	g.Load()
	if c.Name != "banana" {
		t.FailNow()
	}

	// verify env overrides file /w supplied parameters to test filtering empty strings
	os.Setenv("TEST_NAME", "hammock")
	g.Load("", "test", "", "empty", "")
	if c.Name != "hammock" {
		t.FailNow()
	}

	// verify cli overrides env
	os.Args = []string{"-ahurrah"}
	g.Load()
	if c.Name != "hurrah" {
		t.FailNow()
	}
}

func TestConfigPublicAdd(t *testing.T) {
	o := &Config{}

	// none of these should get added
	o.Add("", "", "")
	o.Add("nameonly", "", "")
	o.Add("nameanddesc", "description but nothing else", "")
	if len(o.settings) > 0 {
		t.FailNow()
	}

	// next let's try some good ones
	o.Add("nameandenv", "", "ENV")
	o.Add("namedescandoptions", "description with an option", "", "-n")
	o.Add("allfieldsplus", "all fields with multiple options", "ALLFP", "--all", "-a")
	if len(o.settings) != 3 {
		t.FailNow()
	}
}

func TestConfigPublicExample(_ *testing.T) {
	o := &Config{}
	o.Example("Whatever")
}

func TestConfigPublicHelp(_ *testing.T) {
	o := &Config{}
	o.Help()
}

func TestConfigConfigFile(t *testing.T) {
	g := Config{configFile: "test.txt"}
	if g.ConfigFile() != g.configFile {
		t.FailNow()
	}
}