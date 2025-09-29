# nabot

A simple and powerful bot development framework for Go, built on top of [telego](https://github.com/mymmrac/telego).

## Features

- **State Management**: Stack-based state system for building complex conversational flows
- **Data Storage**: Type-safe data storage for passing information between updates
- **Handler Chain**: Process updates through a chain of handlers with easy control flow
- **Simple API**: Clean, easy-to-understand interface for building bots

## Installation

```bash
go get github.com/bale-ir/nabot@latest
```

## Quick Start

Here's a simple echo bot:

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/bale-ir/nabot"
    "github.com/bale-ir/nabot/handlers"
    "github.com/mymmrac/telego"
    tu "github.com/mymmrac/telego/telegoutil"
)

func main() {
    bot, err := telego.NewBot(os.Getenv("BOT_TOKEN"), telego.WithAPIServer("https://tapi.bale.ai"))
    if err != nil {
        log.Fatal(err)
    }

    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()

	updates, err := bot.UpdatesViaLongPolling(ctx, &telego.GetUpdatesParams{
		Timeout: 60,
	})
    if err != nil {
        log.Fatal(err)
    }

    app := nabot.NewApp(bot, updates)

    // Handle /start command
    app.Handle(handlers.Command{
        Command: "start",
        HandleFunc: func(ctx nabot.Context, args []string) error {
            _, err := ctx.Bot().SendMessage(ctx, tu.Message(ctx.ChatID(), "سلام! یه پیام بفرست تا منم تکرارش کنم."))
            return err
        },
    })

    // Echo all text messages
    app.Handle(handlers.Text{
        HandlerName: "echo",
        HandleFunc: func(ctx nabot.Context, text string) error {
            _, err := ctx.Bot().SendMessage(ctx, tu.Message(ctx.ChatID(), text))
            return err
        },
    })

    go app.Run()

    <-ctx.Done()
    app.Stop()
}
```

## Core Concepts

### Handlers

Handlers process updates. They are called in the order they're registered:

```go
app.Handle(firstHandler)
app.Handle(secondHandler)
```

Return `nabot.ErrPass` to skip a handler and pass the update to the next one.

### State Management

States help build conversational flows. Each chat has a stack of states:

```go
stateHandler := nabot.NewStateHandler(app)
mainState := stateHandler.RegisterState(myMainState)
app.Handle(stateHandler)

// Transition to a state
mainState.Go(ctx)
```

### Data Storage

Store and retrieve typed data for each chat:

```go
const firstNameKey nabot.DataKey[string] = "first_name"

func myHandler(ctx nabot.Context) error {
  // Store data
  nabot.Set(ctx, firstNameKey, "John")

  // Retrieve data
  name, err := nabot.Get(ctx, firstNameKey)
  // ...
}
```

## Examples

See [examples/quizbot](examples/quizbot) for a complete example featuring:
- Multiple states with transitions
- Inline and reply keyboard buttons
- Data storage between updates
- State chaining
