package control

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/intelsdilabs/gomit"
	"github.com/intelsdilabs/pulse/control/plugin"
	"github.com/intelsdilabs/pulse/control/plugin/cpolicy"
	"github.com/intelsdilabs/pulse/core"
	"github.com/intelsdilabs/pulse/core/cdata"
	"github.com/intelsdilabs/pulse/core/ctypes"
	"github.com/intelsdilabs/pulse/pkg/logger"

	. "github.com/smartystreets/goconvey/convey"
)

// Mock Executor used to test
type MockPluginExecutor struct {
	Killed          bool
	Response        string
	WaitTime        time.Duration
	WaitError       error
	WaitForResponse func(time.Duration) (*plugin.Response, error)
}

// Mock plugin manager that will fail swap on the last rollback for testing rollback failure is caught
type MockPluginManagerBadSwap struct {
	Mode           int
	ExistingPlugin CatalogedPlugin
}

func (m *MockPluginManagerBadSwap) LoadPlugin(string, gomit.Emitter) (*loadedPlugin, error) {
	return new(loadedPlugin), nil
}
func (m *MockPluginManagerBadSwap) UnloadPlugin(c CatalogedPlugin) error { return errors.New("fake") }
func (m *MockPluginManagerBadSwap) LoadedPlugins() *loadedPlugins        { return nil }
func (m *MockPluginManagerBadSwap) SetMetricCatalog(catalogsMetrics)     {}
func (m *MockPluginManagerBadSwap) SetEmitter(gomit.Emitter)             {}
func (m *MockPluginManagerBadSwap) GenerateArgs() plugin.Arg             { return plugin.Arg{} }

func TestPluginControlGenerateArgs(t *testing.T) {
	Convey("pluginControl.Start", t, func() {
		c := New()
		Convey("starts successfully", func() {
			err := c.Start()
			So(c.Started, ShouldBeTrue)
			So(err, ShouldBeNil)
		})
		Convey("sets monitor duration", func() {
			c.SetMonitorOptions(MonitorDurationOption(time.Millisecond * 100))
			So(c.pluginRunner.Monitor().duration, ShouldResemble, 100*time.Millisecond)
		})
	})
}

func TestPluginControlStart(t *testing.T) {
	Convey("pluginControl.generateArgs", t, func() {
		Convey("returns arg", func() {
			c := New()
			c.Start()
			a := c.generateArgs()
			So(a, ShouldNotBeNil)
		})
	})
}

func TestSwapPlugin(t *testing.T) {
	if PulsePath != "" {
		Convey("SwapPlugin", t, func() {
			c := New()
			c.Start()
			e := c.Load(PluginPath)

			So(e, ShouldBeNil)

			dummy2Path := strings.Replace(PluginPath, "pulse-collector-dummy1", "pulse-collector-dummy2", 1)
			pc := c.PluginCatalog()
			dummy := pc[0]

			Convey("successfully swaps plugins", func() {
				err := c.SwapPlugins(dummy2Path, dummy)
				pc = c.PluginCatalog()
				So(err, ShouldBeNil)
				So(pc[0].Name(), ShouldEqual, "dummy2")
			})

			Convey("does not unload & returns an error if it cannot load a plugin", func() {
				err := c.SwapPlugins("/fake/plugin/path", pc[0])
				So(err, ShouldNotBeNil)
				So(pc[0].Name(), ShouldEqual, "dummy1")
			})

			Convey("rollsback loaded plugin & returns an error if it cannot unload a plugin", func() {
				dummy := pc[0]
				c.SwapPlugins(dummy2Path, dummy)
				pc = c.PluginCatalog()

				err := c.SwapPlugins(PluginPath, dummy)
				So(err, ShouldNotBeNil)
				So(pc[0].Name(), ShouldEqual, "dummy2")
			})

			Convey("rollback failure returns error", func() {
				dummy := pc[0]
				pm := new(MockPluginManagerBadSwap)
				pm.ExistingPlugin = dummy
				c.pluginManager = pm

				err := c.SwapPlugins(dummy2Path, dummy)
				So(err, ShouldNotBeNil)
			})
		})
	}
}

// Uses the dummy collector plugin to simulate Loading
func TestLoad(t *testing.T) {
	// These tests only work if PULSE_PATH is known.
	// It is the responsibility of the testing framework to
	// build the plugins first into the build dir.
	if PulsePath != "" {
		Convey("pluginControl.Load", t, func() {

			Convey("loads successfully", func() {
				c := New()
				c.Start()
				err := c.Load(PluginPath)

				So(c.pluginManager.LoadedPlugins(), ShouldNotBeEmpty)
				So(err, ShouldBeNil)
			})

			Convey("returns error if not started", func() {
				c := New()
				err := c.Load(PluginPath)

				So(len(c.pluginManager.LoadedPlugins().Table()), ShouldEqual, 0)
				So(err, ShouldNotBeNil)
			})

			Convey("adds to pluginControl.pluginManager.LoadedPlugins on successful load", func() {
				c := New()
				c.Start()
				err := c.Load(PluginPath)

				So(err, ShouldBeNil)
				So(len(c.pluginManager.LoadedPlugins().Table()), ShouldBeGreaterThan, 0)
			})

			Convey("returns error from pluginManager.LoadPlugin()", func() {
				c := New()
				c.Start()
				err := c.Load(PluginPath + "foo")

				So(err, ShouldNotBeNil)
				// So(len(c.pluginManager.LoadedPlugins.Table()), ShouldBeGreaterThan, 0)
			})

		})
	} else {
		fmt.Printf("PULSE_PATH not set. Cannot test %s plugin.\n", PluginName)
	}
}

func TestUnload(t *testing.T) {
	// These tests only work if PULSE_PATH is known.
	// It is the responsibility of the testing framework to
	// build the plugins first into the build dir.
	if PulsePath != "" {
		Convey("pluginControl.Unload", t, func() {
			Convey("unloads successfully", func() {
				c := New()
				c.Start()
				err := c.Load(PluginPath)

				So(c.pluginManager.LoadedPlugins, ShouldNotBeEmpty)
				So(err, ShouldBeNil)

				pc := c.PluginCatalog()

				So(len(pc), ShouldEqual, 1)
				err = c.Unload(pc[0])
				So(err, ShouldBeNil)
			})

			Convey("returns error on unload for unknown plugin(or already unloaded)", func() {
				c := New()
				c.Start()
				err := c.Load(PluginPath)

				So(c.pluginManager.LoadedPlugins, ShouldNotBeEmpty)
				So(err, ShouldBeNil)

				pc := c.PluginCatalog()

				So(len(pc), ShouldEqual, 1)
				err = c.Unload(pc[0])
				So(err, ShouldBeNil)
				err = c.Unload(pc[0])
				So(err, ShouldResemble, errors.New("plugin [dummy1] -- [1] not found (has it already been unloaded?)"))
			})
		})

	} else {
		fmt.Printf("PULSE_PATH not set. Cannot test %s plugin.\n", PluginName)
	}
}

func TestStop(t *testing.T) {
	Convey("pluginControl.Stop", t, func() {
		c := New()
		c.Start()
		c.Stop()

		Convey("stops", func() {
			So(c.Started, ShouldBeFalse)
		})

	})

}

func TestPluginCatalog(t *testing.T) {
	ts := time.Now()

	c := New()

	lp1 := new(loadedPlugin)
	lp1.Meta = plugin.PluginMeta{Name: "test1", Version: 1}
	lp1.Type = 0
	lp1.State = "loaded"
	lp1.LoadedTime = ts
	c.pluginManager.LoadedPlugins().Append(lp1)

	lp2 := new(loadedPlugin)
	lp2.Meta = plugin.PluginMeta{Name: "test2", Version: 1}
	lp2.Type = 0
	lp2.State = "loaded"
	lp2.LoadedTime = ts
	c.pluginManager.LoadedPlugins().Append(lp2)

	lp3 := new(loadedPlugin)
	lp3.Meta = plugin.PluginMeta{Name: "test3", Version: 1}
	lp3.Type = 0
	lp3.State = "loaded"
	lp3.LoadedTime = ts
	c.pluginManager.LoadedPlugins().Append(lp3)

	pc := c.PluginCatalog()

	Convey("it returns a list of CatalogedPlugins (PluginCatalog)", t, func() {
		So(pc, ShouldHaveSameTypeAs, PluginCatalog{})
	})

	Convey("the loadedPlugins implement the interface CatalogedPlugin interface", t, func() {
		So(lp1.Name(), ShouldEqual, "test1")
	})

}

type mc struct {
	e int
}

func (m *mc) Fetch(ns []string) ([]*metricType, error) {
	return nil, nil
}

func (m *mc) resolvePlugin(mns []string, ver int) (*loadedPlugin, error) {
	return nil, nil
}

func (m *mc) GetPlugin([]string, int) (*loadedPlugin, error) {
	return nil, nil
}

func (m *mc) Get(ns []string, ver int) (*metricType, error) {
	if m.e == 1 {
		return &metricType{
			policy: &mockCDProc{},
		}, nil
	}
	return nil, errMetricNotFound
}

func (m *mc) Subscribe(ns []string, ver int) error {
	if ns[0] == "nf" {
		return errMetricNotFound
	}
	return nil
}

func (m *mc) Unsubscribe(ns []string, ver int) error {
	if ns[0] == "nf" {
		return errMetricNotFound
	}
	if ns[0] == "neg" {
		return errNegativeSubCount
	}
	return nil
}

func (m *mc) Add(*metricType)                 {}
func (m *mc) Table() map[string][]*metricType { return map[string][]*metricType{} }
func (m *mc) Item() (string, []*metricType)   { return "", []*metricType{} }

func (m *mc) Next() bool {
	m.e = 1
	return false
}

func (m *mc) AddLoadedMetricType(*loadedPlugin, core.MetricType) {

}

type mockCDProc struct {
}

func (m *mockCDProc) Process(in map[string]ctypes.ConfigValue) (*map[string]ctypes.ConfigValue, *cpolicy.ProcessingErrors) {
	if _, ok := in["fail"]; ok {
		pe := cpolicy.NewProcessingErrors()
		pe.AddError(errors.New("test fail"))
		return nil, pe
	}
	return &in, nil
}

func TestSubscribeMetric(t *testing.T) {
	c := New()
	c.Start()
	c.Load(PluginPath)
	c.pluginRunner.(*runner).monitor.duration = time.Millisecond * 100
	Convey("does not return errors when metricCatalog.Subscribe() does not return an error", t, func() {
		cd := cdata.NewNode()
		cd.AddItem("password", &ctypes.ConfigValueStr{Value: "value"})
		mt := MockMetricType{namespace: []string{"intel", "dummy", "foo"}}
		_, err := c.SubscribeMetricType(mt, cd)
		So(err, ShouldBeNil)
	})
	Convey("returns errors when metricCatalog.Subscribe() returns an error", t, func() {
		cd := cdata.NewNode()
		mt := MockMetricType{namespace: []string{"intel", "dummy", "foo"}}
		_, err := c.SubscribeMetricType(mt, cd)
		So(err, ShouldNotBeEmpty)
	})
}

func TestUnsubscribeMetric(t *testing.T) {
	c := New()
	c.metricCatalog = &mc{}
	lp := new(loadedPlugin)
	Convey("When an error is returned", t, func() {
		Convey("it panics", func() {
			mt := newMetricType([]string{"nf"}, time.Now(), lp)
			So(func() { c.UnsubscribeMetricType(mt) }, ShouldPanic)
			mt = newMetricType([]string{"nf"}, time.Now(), lp)
			So(func() { c.UnsubscribeMetricType(mt) }, ShouldPanic)
		})
	})
	Convey("When no error is returned", t, func() {
		Convey("it doesn't panic", func() {
			mt := newMetricType([]string{"hello"}, time.Now(), lp)
			So(func() { c.UnsubscribeMetricType(mt) }, ShouldNotPanic)
		})
	})
}

// TODO move to metricCatalog
// func TestResolvePlugin(t *testing.T) {
// 	Convey(".resolvePlugin()", t, func() {
// 		c := New()
// 		lp := &loadedPlugin{}
// 		mt := newMetricType([]string{"foo", "bar"}, time.Now(), lp)
// 		c.metricCatalog.Add(mt)
// 		Convey("it resolves the plugin", func() {
// 			p, err := c.resolvePlugin([]string{"foo", "bar"}, -1)
// 			So(err, ShouldBeNil)
// 			So(p, ShouldEqual, lp)
// 		})
// 		Convey("it returns an error if the metricType cannot be found", func() {
// 			p, err := c.resolvePlugin([]string{"baz", "qux"}, -1)
// 			So(p, ShouldBeNil)
// 			So(err, ShouldResemble, errors.New("metric not found"))
// 		})
// 	})
// }

func TestExportedMetricCatalog(t *testing.T) {
	Convey(".MetricCatalog()", t, func() {
		c := New()
		lp := &loadedPlugin{}
		mt := newMetricType([]string{"foo", "bar"}, time.Now(), lp)
		c.metricCatalog.Add(mt)
		Convey("it returns a collection of core.MetricTypes", func() {
			t, err := c.MetricCatalog()
			So(err, ShouldBeNil)
			So(len(t), ShouldEqual, 1)
			So(t[0].Namespace(), ShouldResemble, []string{"foo", "bar"})
		})
	})
}

func TestMetricExists(t *testing.T) {
	Convey("MetricExists()", t, func() {
		c := New()
		c.metricCatalog = &mc{}
		So(c.MetricExists([]string{"hi"}, -1), ShouldEqual, false)
		// c.metricCatalog.Next()
		// So(c.MetricExists([]string{"hi"}, -1), ShouldEqual, true)
	})
}

// ------------------ bunch of entities to just test CollectMetrics -----------

type MockMetricType struct {
	namespace []string
}

func (m MockMetricType) Namespace() []string {
	return m.namespace
}

func (m MockMetricType) LastAdvertisedTime() time.Time {
	return time.Now()
}

func (m MockMetricType) Version() int {
	return 1
}

func (m MockMetricType) Config() *cdata.ConfigDataNode {
	return nil
}

func TestCollectMetrics(t *testing.T) {

	Convey("given a new router", t, func() {
		// adjust HB timeouts for test
		plugin.PingTimeoutLimit = 1
		plugin.PingTimeoutDurationDefault = time.Second * 1

		// Create controller
		c := New()
		c.pluginRunner.(*runner).monitor.duration = time.Millisecond * 100
		c.Start()
		// Load plugin
		c.Load(PluginPath)

		m := []core.MetricType{}
		m1 := MockMetricType{namespace: []string{"intel", "dummy", "foo"}}
		m2 := MockMetricType{namespace: []string{"intel", "dummy", "bar"}}

		m = append(m, m1)
		m = append(m, m2)

		cd := cdata.NewNode()
		cd.AddItem("password", ctypes.ConfigValueStr{Value: "testval"})

		mt, errs := c.SubscribeMetricType(m1, cd)
		for _, er := range errs {
			logger.Infof("TestCollectMetrics", "errs: %#v", er)
		}
		So(mt, ShouldNotBeNil)
		So(errs, ShouldBeEmpty)

		c.SubscribeMetricType(m1, cd)
		c.SubscribeMetricType(m1, cd)
		c.SubscribeMetricType(m1, cd)
		// fmt.Println(a, b)
		time.Sleep(time.Millisecond * 100)
		c.SubscribeMetricType(m2, cd)
		time.Sleep(time.Millisecond * 200)

		// Call collect on router

		for x := 0; x < 5; x++ {
			// fmt.Println("\n *  Calling Collect")
			_, err := c.CollectMetrics(m, cd, time.Now().Add(time.Second*60))
			So(err, ShouldBeNil)
			// fmt.Printf(" *  Collect Response: %+v\n", cr)
		}
		time.Sleep(time.Millisecond * 1000)
	})
}
