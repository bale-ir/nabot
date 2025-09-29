package handlers

import (
	"github.com/bale-ir/nabot"
	"github.com/mymmrac/telego"
	"strings"
)

// Func is a simple function handler.
//
// Example:
//
//	app.Handle(handlers.Func(func(ctx nabot.Context) error {
//	    // handle any update
//	    return nil
//	}))
type Func func(ctx nabot.Context) error

func (f Func) Name() string {
	return "function_handler"
}

func (f Func) Handle(ctx nabot.Context) error {
	return f(ctx)
}

// Filter passes updates only if the filter function returns true.
// Returns nil (not ErrPass) if the filter returns false, stopping the handler chain.
// Useful for creating allowlists for access control.
//
// Example (admin only):
//
//	app.Handle(handlers.Filter(func(ctx nabot.Context) bool {
//	    return ctx.ChatID().ID == adminChatID
//	}))
type Filter func(ctx nabot.Context) bool

func (f Filter) Name() string {
	return "filter_handler"
}

func (f Filter) Handle(ctx nabot.Context) error {
	if f(ctx) {
		return nabot.ErrPass
	}
	return nil
}

// Text handles text messages.
//
// Example:
//
//	app.Handle(handlers.Text{
//	    HandlerName: "echo",
//	    HandleFunc: func(ctx nabot.Context, text string) error {
//	        _, err := ctx.Bot().SendMessage(tu.Message(ctx.ChatID(), text))
//	        return err
//	    },
//	})
type Text struct {
	HandlerName string
	HandleFunc  func(ctx nabot.Context, text string) error
}

func (t Text) Name() string {
	return t.HandlerName
}

func (t Text) Handle(ctx nabot.Context) error {
	if msg := ctx.Update().Message; msg != nil && msg.Text != "" {
		return t.HandleFunc(ctx, msg.Text)
	}
	return nabot.ErrPass
}

// Command handles bot commands like /start or /help.
//
// Example:
//
//	app.Handle(handlers.Command{
//	    Command: "start",
//	    HandleFunc: func(ctx nabot.Context, args []string) error {
//	        // if text is '/start 1 2 3', args will be equal to []string{"1", "2", "3"}
//	        return nil
//	    },
//	})
type Command struct {
	Command    string
	HandleFunc func(ctx nabot.Context, args []string) error
	Separator  func(cmd string, fullText string) []string
}

func (c Command) Name() string {
	if strings.HasPrefix(c.Command, "/") {
		return c.Command
	}
	return "/" + c.Command
}

func (c Command) Handle(ctx nabot.Context) error {
	update := ctx.Update()
	if update.Message == nil {
		return nabot.ErrPass
	}
	cmd := c.Name()
	idx := strings.Index(update.Message.Text, cmd)
	if idx < 0 {
		return nabot.ErrPass
	}
	var args []string
	if c.Separator != nil {
		args = c.Separator(cmd, update.Message.Text)
	} else {
		args = strings.Fields(update.Message.Text[idx+len(cmd):])
	}
	return c.HandleFunc(ctx, args)
}

const (
	callbackDataSeparator = "\\"
)

// InlineButton handles inline keyboard button callbacks.
//
// Example:
//
//	 btn := handlers.InlineButton{
//	     ID: "accept",
//	     DefaultText: "Accept",
//			HandleFunc: func(ctx nabot.Context, data string) error {
//			    // handle button click
//			    return nil
//			},
//	 }
//	 app.Handle(btn)
//		// Create button:
//	 btn.Button("request_id")
//		// Or:
//		btn.ButtonWithText("Yes", "request_id")
type InlineButton struct {
	ID          string
	DefaultText string
	HandleFunc  func(ctx nabot.Context, data string) error
}

func (i InlineButton) Name() string {
	return i.ID
}

func (i InlineButton) Handle(ctx nabot.Context) error {
	if ctx.Update().CallbackQuery == nil {
		return nabot.ErrPass
	}
	data, ok := strings.CutPrefix(ctx.Update().CallbackQuery.Data, i.ID+callbackDataSeparator)
	if !ok {
		return nabot.ErrPass
	}
	return i.HandleFunc(ctx, data)
}

// CallbackData returns the callback data string for this button with the given data.
func (i InlineButton) CallbackData(data string) string {
	return i.ID + callbackDataSeparator + data
}

// Button creates an inline keyboard button with the DefaultText.
func (i InlineButton) Button(data string) telego.InlineKeyboardButton {
	return i.ButtonWithText(i.DefaultText, data)
}

// ButtonWithText creates an inline keyboard button with custom text.
func (i InlineButton) ButtonWithText(text, data string) telego.InlineKeyboardButton {
	return telego.InlineKeyboardButton{
		Text:         text,
		CallbackData: i.CallbackData(data),
	}
}

// KeyboardButton handles reply keyboard button clicks.
//
// Example:
//
//	btn := handlers.KeyboardButton{
//	    Text: "Help",
//	    HandleFunc: func(ctx nabot.Context) error {
//	        // handle button click
//	        return nil
//	    },
//	}
//	// Create button:
//	btn.Button()
type KeyboardButton struct {
	Text       string
	HandleFunc func(ctx nabot.Context) error
}

func (k KeyboardButton) Name() string {
	return "reply_button_" + k.Text
}

func (k KeyboardButton) Handle(ctx nabot.Context) error {
	if msg := ctx.Update().Message; msg != nil && msg.Text == k.Text {
		return k.HandleFunc(ctx)
	}
	return nabot.ErrPass
}

// Button creates a reply keyboard button.
func (k KeyboardButton) Button() telego.KeyboardButton {
	return telego.KeyboardButton{
		Text: k.Text,
	}
}
