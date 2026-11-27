package service

import (
	"testing"

	greet "github.com/dej4vu/cloudwego-skills/fixtures/kitex-greet/rpc_gen/kitex_gen/greet"
)

func TestRunFailsFastWithoutInitDeps(t *testing.T) {
	ResetDepsForTest()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("want panic when InitDeps was not called")
		}
	}()
	_, _ = NewHelloService(t.Context()).Run(&greet.HelloReq{Name: "a"})
}

func TestRunUsesInjectedGreeter(t *testing.T) {
	ResetDepsForTest()
	SetGreeterForTest(stubGreeter{})
	resp, err := NewHelloService(t.Context()).Run(&greet.HelloReq{Name: "Ada", Lang: "ZH"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message != "stub:Ada" || resp.Lang != "zh" {
		t.Fatalf("unexpected resp %+v", resp)
	}
}

type stubGreeter struct{}

func (stubGreeter) Greet(_, name string) string { return "stub:" + name }

func TestInitDepsPanicsOnDoubleInit(t *testing.T) {
	ResetDepsForTest()
	InitDeps()
	defer ResetDepsForTest()
	defer func() {
		if recover() == nil {
			t.Fatal("want panic on second InitDeps")
		}
	}()
	InitDeps()
}

func TestNormalizeLang(t *testing.T) {
	for in, want := range map[string]string{" ZH ": "zh", "": "en", "fr": "en", "ja": "ja"} {
		if got := normalizeLang(in); got != want {
			t.Errorf("normalizeLang(%q) = %q, want %q", in, got, want)
		}
	}
}
