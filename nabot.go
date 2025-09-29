package nabot

import (
	"errors"
	"github.com/mymmrac/telego"
	"log/slog"
	"strconv"
	"sync"
)

// App is the main bot application.
// Register handlers with Handle and start processing updates with Run.
//
// Example:
//
//	bot, _ := telego.NewBot(token)
//	updates, _ := bot.UpdatesViaLongPolling(ctx, params)
//	app := nabot.NewApp(bot, updates)
//	app.Handle(myHandler)
//	app.Run()
type App struct {
	bot             *telego.Bot
	updatesChan     <-chan telego.Update
	handlers        []Handler
	logger          *slog.Logger
	dataStore       DataStorage
	extractChatInfo ChatInfoExtractor
	executor        Executor
	wg              sync.WaitGroup
}

// NewApp creates a new bot App.
// The update channel can be created using bot.UpdatesViaLongPolling or bot.UpdatesViaWebhook.
func NewApp(bot *telego.Bot, updates <-chan telego.Update, options ...AppOption) *App {
	app := &App{
		bot:             bot,
		updatesChan:     updates,
		logger:          slog.Default(),
		dataStore:       NewInMemoryDataStore(),
		extractChatInfo: DefaultChatKeyAndID,
		executor:        DefaultExecutor,
	}
	for _, ops := range options {
		ops(app)
	}
	return app
}

// Handle adds a handler to the list of handlers.
// Handlers are called in registration order until one returns an error other than ErrPass.
//
// Example:
//
//	app.Handle(handlers.Command{
//	    Command: "start",
//	    HandleFunc: func(ctx nabot.Context, args []string) error {
//	        // handle /start command
//	        return nil
//	    },
//	})
func (a *App) Handle(handler Handler) {
	a.handlers = append(a.handlers, handler)
}

// Run starts processing updates and blocks until the update channel is closed.
func (a *App) Run() {
	for update := range a.updatesChan {
		a.wg.Add(1)
		a.executor(func() {
			defer a.wg.Done()
			a.processUpdate(update)
		})
	}
}

// Stop blocks until all currently processing handlers are done.
// Call this after the update channel is closed to ensure a clean shutdown.
func (a *App) Stop() {
	a.wg.Wait()
}

func (a *App) processUpdate(update telego.Update) {
	ctx := a.newContext(update)
	if ctx == nil {
		a.logger.Warn("nabot: could not determine context; ignoring update",
			slog.String("update_type", GetTypeOfUpdate(update)),
		)
		return
	}
	var err error
	var handler Handler
	for _, h := range a.handlers {
		handler = h
		err = h.Handle(ContextWithLogger(ctx, a.logger.With(slog.String("handler", h.Name()))))
		if errors.Is(err, ErrPass) {
			continue
		}
		break
	}
	if err != nil {
		if errors.Is(err, ErrPass) {
			a.logger.Info("nabot: update was not handled by any handler")
		} else {
			handlerName := "<nil>"
			if handler != nil {
				handlerName = handler.Name()
			}
			a.logger.
				With("error", err).
				With("handler", handlerName).
				Error("nabot: failed to handle update")
		}
	}
}

func (a *App) newContext(update telego.Update) Context {
	chatKey, chatId, ok := a.extractChatInfo(update)
	if !ok {
		return nil
	}
	n := &nativeContext{
		Context:   update.Context(),
		bot:       a.bot,
		update:    update,
		dataStore: a.dataStore,
		chatKey:   chatKey,
		chatID:    chatId,
		logger:    a.logger,
	}
	n.logger = n.logger.With(
		slog.String("chat", n.chatID.String()),
	)
	return n
}

// AppOption configures an App.
type AppOption func(*App)

// WithLogger sets a custom logger for the app.
func WithLogger(logger *slog.Logger) AppOption {
	return func(a *App) {
		a.logger = logger
	}
}

// WithDataStore sets a custom data storage implementation.
// Default is NewInMemoryDataStore().
func WithDataStore(dataStorage DataStorage) AppOption {
	return func(a *App) {
		a.dataStore = dataStorage
	}
}

// ChatInfoExtractor extracts chat key and chat ID from an update.
// The chat key is used as the parent key in DataStorage.
// Returns false if the update type is not supported and should not be processed by App.
type ChatInfoExtractor func(update telego.Update) (string, telego.ChatID, bool)

// WithCustomChatKeyAndID sets a custom function to extract chat key and chat ID from updates.
// Default is DefaultChatKeyAndID.
func WithCustomChatKeyAndID(extractor ChatInfoExtractor) AppOption {
	return func(a *App) {
		a.extractChatInfo = extractor
	}
}

// DefaultChatKeyAndID extracts chat key and chat ID from most update types.
// Uses chat ID as the key, or user ID for user-specific updates like inline queries.
func DefaultChatKeyAndID(update telego.Update) (string, telego.ChatID, bool) {
	var chatId telego.ChatID
	var user telego.User

	switch {
	case update.Message != nil:
		chatId = update.Message.Chat.ChatID()
	case update.CallbackQuery != nil:
		chatId = update.CallbackQuery.Message.GetChat().ChatID()
	case update.EditedMessage != nil:
		chatId = update.EditedMessage.Chat.ChatID()
	case update.ChannelPost != nil:
		if update.ChannelPost.PinnedMessage != nil {
			chatId = update.ChannelPost.PinnedMessage.GetChat().ChatID()
		}
		chatId = update.ChannelPost.Chat.ChatID()
	case update.EditedChannelPost != nil:
		chatId = update.EditedChannelPost.Chat.ChatID()
	case update.MyChatMember != nil:
		chatId = update.MyChatMember.Chat.ChatID()
	case update.ChatMember != nil:
		chatId = update.ChatMember.Chat.ChatID()
	case update.ChatJoinRequest != nil:
		chatId = update.ChatJoinRequest.Chat.ChatID()
	case update.MessageReaction != nil:
		chatId = update.MessageReaction.Chat.ChatID()
	case update.MessageReactionCount != nil:
		chatId = update.MessageReactionCount.Chat.ChatID()
	case update.InlineQuery != nil:
		user = update.InlineQuery.From
	case update.ChosenInlineResult != nil:
		user = update.ChosenInlineResult.From
	case update.ShippingQuery != nil:
		user = update.ShippingQuery.From
	case update.PreCheckoutQuery != nil:
		user = update.PreCheckoutQuery.From
	case update.PurchasedPaidMedia != nil:
		user = update.PurchasedPaidMedia.From
	case update.PollAnswer != nil:
		if update.PollAnswer.VoterChat != nil {
			chatId = update.PollAnswer.VoterChat.ChatID()
		}
		if update.PollAnswer.User != nil {
			user = *update.PollAnswer.User
		}
	}

	if chatId.ID != 0 {
		return strconv.FormatInt(chatId.ID, 10), chatId, true
	}
	if user.ID != 0 {
		return strconv.FormatInt(user.ID, 10), telego.ChatID{ID: user.ID}, true
	}
	return "", telego.ChatID{}, false
}

// Executor runs handler functions for each update.
// Can be used to set up a worker pool for processing updates.
type Executor func(func())

// WithExecutor sets a custom executor for running handlers.
// Default is DefaultExecutor.
func WithExecutor(executor Executor) AppOption {
	return func(a *App) {
		a.executor = executor
	}
}

// DefaultExecutor processes each update in a new goroutine.
// This makes update handling fully asynchronous.
func DefaultExecutor(f func()) {
	go f()
}

// GetTypeOfUpdate returns a string representing the update type.
// Useful for logging and debugging.
func GetTypeOfUpdate(update telego.Update) string {
	switch {
	case update.Message != nil:
		return "message"
	case update.EditedMessage != nil:
		return "edited_message"
	case update.ChannelPost != nil:
		return "channel_post"
	case update.EditedChannelPost != nil:
		return "edited_channel_post"
	case update.MessageReaction != nil:
		return "message_reaction"
	case update.MessageReactionCount != nil:
		return "message_reaction_count"
	case update.CallbackQuery != nil:
		return "callback_query"
	case update.InlineQuery != nil:
		return "inline_query"
	case update.ChosenInlineResult != nil:
		return "chosen_inline_result"
	case update.ShippingQuery != nil:
		return "shipping_query"
	case update.PreCheckoutQuery != nil:
		return "pre_checkout_query"
	case update.PurchasedPaidMedia != nil:
		return "purchased_paid_media"
	case update.Poll != nil:
		return "poll"
	case update.PollAnswer != nil:
		return "poll_answer"
	case update.MyChatMember != nil:
		return "my_chat_member"
	case update.ChatMember != nil:
		return "chat_member"
	case update.ChatJoinRequest != nil:
		return "chat_join_request"
	case update.ChatBoost != nil:
		return "chat_boost"
	case update.RemovedChatBoost != nil:
		return "removed_chat_boost"
	case update.BusinessConnection != nil:
		return "business_connection"
	case update.BusinessMessage != nil:
		return "business_message"
	case update.EditedBusinessMessage != nil:
		return "edited_business_message"
	case update.DeletedBusinessMessages != nil:
		return "deleted_business_messages"
	}
	return "<unknown>"
}
