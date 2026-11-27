package service

import "strings"

// greeter 把"按语言选招呼语"这件纯逻辑抽成接口，service 只依赖它用到的面。
type greeter interface {
	Greet(lang, name string) string
}

// dependencies 由 InitDeps 在服务启动前装配一次。
type dependencies struct {
	greeter greeter
}

var deps *dependencies

// d 返回已装配依赖；未初始化时快速失败。
func d() *dependencies {
	if deps == nil {
		panic("greet service deps not initialized: call service.InitDeps before serving requests")
	}
	return deps
}

// InitDeps 装配业务依赖；重复调用直接 panic。
func InitDeps() {
	if deps != nil {
		panic("greet service.InitDeps called more than once")
	}
	deps = &dependencies{greeter: langGreeter{}}
}

// ShutdownDeps 释放后台资源；可重复调用。
func ShutdownDeps() {
	deps = nil
}

// ResetDepsForTest 仅供测试隔离使用。
func ResetDepsForTest() { deps = nil }

// SetGreeterForTest 注入替身，仅测试使用。
func SetGreeterForTest(g greeter) {
	if deps == nil {
		deps = &dependencies{}
	}
	deps.greeter = g
}

type langGreeter struct{}

func (langGreeter) Greet(lang, name string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	word := map[string]string{"zh": "你好", "en": "Hello", "ja": "こんにちは"}[lang]
	if word == "" {
		lang, word = "en", "Hello"
	}
	if name == "" {
		return word
	}
	return word + ", " + name + "!"
}
