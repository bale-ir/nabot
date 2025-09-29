package nabot

import (
	"context"
	"errors"
	"github.com/mymmrac/telego"
	"log/slog"
)

var (
	// ErrPass is returned by Handler.Handle when the handler does not want to process the update.
	// When returned, the update is passed to the next handler in the chain.
	ErrPass = errors.New("pass context to next handler")
)

// Handler processes bot updates.
// Handlers are registered with App.Handle and are called in registration order.
// Return ErrPass to skip processing and pass the update to the next handler.
type Handler interface {
	// Name returns the handler name for logging.
	Name() string
	// Handle processes the update. Return ErrPass to skip this handler.
	Handle(ctx Context) error
}

// Context wraps a bot update and provides access to Bot, DataStorage, and other utilities.
// It also implements context.Context for standard context operations.
//
// Example:
//
//	func myHandler(ctx nabot.Context) error {
//	    chatID := ctx.ChatID()
//	    bot := ctx.Bot()
//	    // ... send message using bot
//	    return nil
//	}
type Context interface {
	StorageContext
	Bot() *telego.Bot
	Update() telego.Update
	ChatID() telego.ChatID
	Logger() *slog.Logger
}

type nativeContext struct {
	context.Context
	bot       *telego.Bot
	update    telego.Update
	dataStore DataStorage
	chatKey   string
	chatID    telego.ChatID
	logger    *slog.Logger
}

func (n *nativeContext) Bot() *telego.Bot {
	return n.bot
}

func (n *nativeContext) Update() telego.Update {
	return n.update
}

func (n *nativeContext) ChatKey() string {
	return n.chatKey
}

func (n *nativeContext) ChatID() telego.ChatID {
	return n.chatID
}

func (n *nativeContext) Store() DataStorage {
	return n.dataStore
}

func (n *nativeContext) Logger() *slog.Logger {
	return n.logger
}

type wrappedLogger struct {
	Context
	logger *slog.Logger
}

func (w wrappedLogger) Logger() *slog.Logger {
	return w.logger
}

// ContextWithLogger returns a new Context with the given logger.
// Useful for adding handler-specific log fields.
func ContextWithLogger(ctx Context, logger *slog.Logger) Context {
	return wrappedLogger{
		Context: ctx,
		logger:  logger,
	}
}
