# Busen

`Busen` 是一个 **本地进程内、typed-first、小而美** 的 Go EventBus。

它面向简单应用中的模块解耦与事件分发，强调泛型、`context.Context`、
functional options 和明确的并发语义。它不是消息平台，也不承担分布式、
持久化或重型治理能力。

## 定位

`Busen` 的目标很明确：

- 只做 **in-process** 事件分发
- 默认走 **typed-first** 模型
- 提供轻量 topic 路由、middleware 和 hooks
- 保持 API 小而清晰
- 不内置 tracing、metrics、retry、rate limiting 这类重型能力

## 核心特性

- **类型安全发布/订阅**
  使用 `Subscribe[T]` 和 `Publish[T]`，以 Go `struct` 作为事件载体。
- **轻量 Topic 路由**
  支持基于 `string topic` 的路由，以及克制的 wildcard 匹配。
- **多种订阅方式**
  支持按类型、按 topic、按 predicate filter 订阅。
- **同步 / 异步分发**
  支持 sync handler、async handler，以及 worker/parallelism 配置。
- **显式背压控制**
  提供 bounded queue 与 `OverflowBlock`、`OverflowFailFast`、`OverflowDropNewest`、`OverflowDropOldest` 等策略。
- **局部顺序保证**
  支持 single-worker FIFO，以及同一订阅者内的同 key 局部顺序。
- **轻量扩展能力**
  提供 thin middleware 与 lightweight hooks，但不演变成重型框架。

## 安装

```bash
go get github.com/lin-snow/Busen
```

## 何时适合使用

`Busen` 适合这些场景：

- 你想在单个 Go 应用内部做 typed event 解耦
- 你需要轻量 topic 路由和 bounded async delivery
- 你希望库本身足够小，而不是引入一个事件平台
- 你更愿意把业务治理策略放在 bus 外部组合

`Busen` 不适合这些场景：

- 你需要 durability、replay 或 cross-process delivery
- 你需要内置 tracing、metrics、retry、rate limiting 框架
- 你需要全局顺序保证

## 快速开始

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/lin-snow/Busen"
)

type UserCreated struct {
	ID    string
	Email string
}

func main() {
	bus := busen.New()

	unsubscribe, err := busen.Subscribe(bus, func(ctx context.Context, event busen.Event[UserCreated]) error {
		fmt.Printf("welcome %s\n", event.Value.Email)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	defer unsubscribe()

	err = busen.Publish(context.Background(), bus, UserCreated{
		ID:    "u_123",
		Email: "hello@example.com",
	})
	if err != nil {
		log.Fatal(err)
	}

	_ = bus.Close(context.Background())
}
```

## Topic 路由

`Busen` 支持点分隔的轻量 topic 路由。

- `*`：匹配恰好一个 segment
- `>`：匹配剩余的一个或多个 segment，且必须位于末尾

```go
sub, err := busen.SubscribeTopic(bus, "orders.>", func(ctx context.Context, event busen.Event[string]) error {
	fmt.Println(event.Topic, event.Value)
	return nil
})
if err != nil {
	log.Fatal(err)
}
defer sub()

_ = busen.Publish(context.Background(), bus, "created", busen.WithTopic("orders.eu.created"))
```

## 异步分发与顺序

异步订阅使用有界队列，背压策略是显式的：

- `OverflowBlock`
- `OverflowFailFast`
- `OverflowDropNewest`
- `OverflowDropOldest`

```go
_, err = busen.Subscribe(bus, func(ctx context.Context, event busen.Event[UserCreated]) error {
	return nil
},
	busen.Async(),
	busen.Sequential(),
	busen.WithBuffer(128),
	busen.WithOverflow(busen.OverflowBlock),
)
```

如果发布时带上 `WithKey(...)`，那么同一 async 订阅者内、相同非空
ordering key 的事件会保持局部顺序：

```go
_, err = busen.Subscribe(bus, func(ctx context.Context, event busen.Event[UserCreated]) error {
	return nil
}, busen.Async(), busen.WithParallelism(4), busen.WithBuffer(256))

_ = busen.Publish(context.Background(), bus, UserCreated{ID: "1"}, busen.WithKey("tenant-a"))
_ = busen.Publish(context.Background(), bus, UserCreated{ID: "2"}, busen.WithKey("tenant-a"))
```

边界说明：

- ordering key 只对 async subscriber 生效
- 空 key 会回退到普通非 keyed 调度
- 顺序保证是 **per subscriber / per key**，不是全局顺序

## Middleware 与 Hooks

### Middleware

`Busen` 提供一个很薄的 dispatch middleware 层，用来做本地组合，而不是做
重型 pipeline framework。

```go
err = bus.Use(func(next busen.Next) busen.Next {
	return func(ctx context.Context, dispatch busen.Dispatch) error {
		return next(ctx, dispatch)
	}
})
if err != nil {
	log.Fatal(err)
}
```

`middleware` 的边界：

- 只包 handler invocation
- 不替代 hooks
- 不承担 retries、metrics、tracing、distributed concerns
- 不影响 subscriber matching、topic routing、async queue selection
- 对 `Dispatch` 的修改只影响后续 middleware 和最终 handler
- hooks 仍然观察原始 publish 元数据

如果你更喜欢构造期注册方式，也可以使用 `WithMiddleware(...)`：

```go
bus := busen.New(
	busen.WithMiddleware(func(next busen.Next) busen.Next {
		return func(ctx context.Context, dispatch busen.Dispatch) error {
			return next(ctx, dispatch)
		}
	}),
)
```

### Hooks

`Hooks` 用来观察运行时事件，而不是参与 handler 调用链控制。

```go
bus := busen.New(
	busen.WithHooks(busen.Hooks{
		OnHandlerError: func(info busen.HandlerError) {
			log.Printf("async=%v topic=%q err=%v", info.Async, info.Topic, info.Err)
		},
		OnHandlerPanic: func(info busen.HandlerPanic) {
			log.Printf("panic in %v: %v", info.EventType, info.Value)
		},
		OnEventDropped: func(info busen.DroppedEvent) {
			log.Printf("dropped event for topic %q with policy %v", info.Topic, info.Policy)
		},
	}),
)
```

当前 hooks 触发点包括：

- `OnPublishStart`
- `OnPublishDone`
- `OnHandlerError`
- `OnHandlerPanic`
- `OnEventDropped`

## 和社区四类模式的差异

这里的四类并不完全处在同一抽象层级，但它们是很多用户在选型时会自然拿来比较的对象。

### 1. EventEmitter

`EventEmitter` 往往追求简单和灵活，通常更接近：

- callback 风格
- string/event-name 驱动
- 类型约束较弱

`Busen` 相比之下更强调：

- typed-first
- 本地并发语义明确
- bounded async delivery
- 背压和局部顺序

### 2. TypedBus

`TypedBus` 与 `Busen` 最接近，因为都强调类型安全。  
区别通常在于：

- 很多 `TypedBus` 只把“类型安全发布/订阅”做得很好
- `Busen` 在此基础上继续补了 topic、背压、局部顺序、middleware、hooks

所以 `Busen` 可以理解成：**typed-first，但不是只有类型安全这一层能力。**

### 3. CQRS

`CQRS` 是一种架构模式，核心是命令和查询分离。  
它不等于本地事件总线，也不天然提供：

- 本地 pub/sub
- topic routing
- bounded async delivery

`Busen` 不是 CQRS framework。  
如果你的应用用了 CQRS，`Busen` 可以作为其中的一个本地事件分发组件，但不会替你实现整个 CQRS 模型。

### 4. Mediator

`Mediator` 更强调“协调调用”而不是“事件广播”：

- 更像 request/handler 组织方式
- 往往是一对一或中心协调
- 不一定提供真正的 async pub/sub 语义

`Busen` 不是 orchestration-style mediator。  
它更适合用来做 **事件广播式解耦**，而不是中心化业务调度。

## 性能测试

`Busen` 已经内置了可重复运行的 benchmark，主要覆盖这些热路径：

- `Publish[T]` 在 `1 / 10 / 100` 个订阅者下的成本
- sync 与 async sequential 的差异
- async keyed delivery
- middleware 开启/关闭
- middleware + hooks 同时开启
- async keyed + topic routing
- exact / wildcard 路由
- direct router matcher 成本

运行方式：

```bash
go test ./... -run '^$' -bench . -benchmem
```

这些数字代表的是 **in-process event bus 的热路径开销**，不是消息系统吞吐保证。

在一台较新的 Apple Silicon 机器上，当前基线大致为：

- sync publish（1 subscriber）：约 `130-145 ns/op`
- sync publish（10 subscribers）：约 `500-530 ns/op`
- async sequential publish：约 `250-265 ns/op`
- async keyed publish：约 `300-305 ns/op`
- middleware-enabled publish：约 `190-195 ns/op`
- middleware + hooks publish：约 `235-240 ns/op`
- async keyed + topic publish：约 `375-380 ns/op`
- exact topic publish：约 `160 ns/op`
- wildcard topic publish：约 `170 ns/op`

这一轮里，一个最明确的数据驱动优化是：把 wildcard router match 路径的分配降到了 `0 allocs/op`，direct matcher 大约在 `7 ns/op` 左右。

## 设计边界

- 类型匹配是精确的，不会自动按接口层级分发
- 不保证全局顺序
- `Sequential()` 本质上是 single-worker async FIFO 的 shorthand
- 非空 ordering key 只提供局部顺序保证
- sync handler 错误会直接返回给 `Publish`
- async handler error / panic 不会回传给 `Publish`，应通过 `Hooks` 观察
- 这是一个简单 app 使用的小库，不是 distributed event platform

## 开发

常用本地命令已经放在 `Makefile` 里：

```bash
make help
make fmt
make lint
make vet
make test
make test-race
make cover
make check
```

`make lint` 会用当前 Go toolchain 把 `golangci-lint` `v2.3.0` 安装到 `./.bin`，避免和 `go.mod` 中声明的 Go 版本产生偏差。

## 相关文档

- 支持与提问：`SUPPORT.md`
- 贡献指南：`CONTRIBUTING.md`
- 安全策略：`SECURITY.md`
- 发布流程：`RELEASING.md`
- 项目治理：`GOVERNANCE.md`
- 行为准则：`CODE_OF_CONDUCT.md`
