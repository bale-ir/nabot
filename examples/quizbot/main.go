package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/bale-ir/nabot"
	"github.com/bale-ir/nabot/handlers"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	botToken := os.Getenv("BOT_TOKEN")

	bot, err := telego.NewBot(botToken,
		telego.WithAPIServer("https://tapi.bale.ai"),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Kill, syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	updates, err := bot.UpdatesViaLongPolling(ctx, &telego.GetUpdatesParams{
		Timeout: 60,
	})
	if err != nil {
		log.Fatal(err)
	}

	app := nabot.NewApp(bot, updates)

	registerHandlers(app)

	go func() {
		app.Run()
	}()

	<-ctx.Done()
	app.Stop()
}

func registerHandlers(app *nabot.App) {
	stateHandler := nabot.NewStateHandler(app)

	// chain states together
	toMainState := stateHandler.RegisterAndChainStates(
		newMainState(),
		newQuizState(stateHandler.Back()),
	)

	// put /start as first handler, so it is handled regardless of state
	app.Handle(handlers.Command{
		Command: "start",
		HandleFunc: func(ctx nabot.Context, args []string) error {
			err := nabot.Clear(ctx)
			if err != nil {
				return err
			}
			return toMainState.Go(ctx)
		},
	})

	app.Handle(stateHandler)

	// default handler when no other handler matches
	app.Handle(handlers.Func(func(ctx nabot.Context) error {
		return toMainState.Go(ctx)
	}))
}

const (
	welcomeMessage = `👋 سلام!
به بازوی آزمونک خوش اومدی! 🎉
دوست داری در مورد چه موضوعی ازت سؤال بپرسم؟ 🤔`

	categoryExitMessage = `✨ امیدوارم که از جواب دادن به سؤالای %s لذت برده باشی! 😊`

	changeCategoryMessage = `🔄 الان دوست داری در مورد چه موضوعی ازت سؤال بپرسم؟ 🤔`

	correctAnswerMessage = `🎉 آفرین! جوابت درست بود! ✅

میخوای بازم ازت سؤال بپرسم؟ 😃`

	wrongAnswerMessage = `❌ اشتباه بود! 😢
پاسخ درست: 
*%s*

میخوای بازم ازت سؤال بپرسم؟ 😃`
)

const (
	categoryDataKey    nabot.DataKey[string]   = "category"
	currentQuestionKey nabot.DataKey[Question] = "currentQuestion"
)

type mainState struct {
	nabot.BaseState
	categoryButton handlers.InlineButton
}

func newMainState() nabot.ChainableState {
	s := &mainState{}
	s.categoryButton = handlers.InlineButton{
		ID:         "category",
		HandleFunc: s.handleCategory,
	}
	s.BaseState = nabot.BaseState{
		ID:       "main",
		Renderer: s.Render,
		Handlers: []nabot.Handler{
			s.categoryButton,
		},
	}
	return s
}

func (s *mainState) Render(ctx nabot.TransitionContext) error {
	text := welcomeMessage
	// remove keyboard if coming from quiz state
	if categoryId, err := nabot.Get(ctx, categoryDataKey); err == nil {
		_, err = ctx.Bot().SendMessage(
			ctx, tu.Message(ctx.ChatID(), fmt.Sprintf(categoryExitMessage, questions[categoryId].Name)).
				WithReplyMarkup(tu.ReplyKeyboardRemove()))
		if err != nil {
			return err
		}
		text = changeCategoryMessage
	} else if !errors.Is(err, nabot.ErrDataKeyNotFound) {
		return err
	}

	var rows [][]telego.InlineKeyboardButton
	for k, v := range questions {
		rows = append(rows, tu.InlineKeyboardRow(s.categoryButton.ButtonWithText(v.Name, k)))
	}
	_, err := ctx.Bot().SendMessage(ctx, tu.Message(ctx.ChatID(), text).
		WithReplyMarkup(tu.InlineKeyboard(rows...)))
	return err
}

func (s *mainState) handleCategory(ctx nabot.Context, data string) error {
	err := nabot.Set(ctx, categoryDataKey, data)
	if err != nil {
		return err
	}
	return s.ToNext.Go(ctx) // is set in RegisterAndChainStates
}

type quizState struct {
	nabot.BaseState
	againButton handlers.KeyboardButton
	backButton  handlers.KeyboardButton
}

func newQuizState(ToBack nabot.Transition) nabot.ChainableState {
	s := &quizState{}
	s.againButton = handlers.KeyboardButton{
		Text:       "بازم بپرس",
		HandleFunc: s.handleAgain,
	}
	s.backButton = handlers.KeyboardButton{
		Text:       "نه میخوام موضوع رو عوض کنم",
		HandleFunc: s.handleBack,
	}
	s.BaseState = nabot.BaseState{
		ID:       "quiz",
		Renderer: s.Render,
		Handlers: []nabot.Handler{
			s.againButton,
			s.backButton,
			handlers.Text{
				HandlerName: "chosen_option",
				HandleFunc:  s.handleOption,
			},
		},
		ToNext: ToBack, // set it manually because it is the last state in the chain
	}
	return s
}

func (s *quizState) Render(ctx nabot.TransitionContext) error {
	categoryId, err := nabot.Get(ctx, categoryDataKey)
	if err != nil {
		return err
	}
	category := questions[categoryId]
	question := category.Questions[rand.Intn(len(category.Questions))]
	err = nabot.Set(ctx, currentQuestionKey, question)
	if err != nil {
		return err
	}

	var options [][]telego.KeyboardButton
	for _, o := range question.Options {
		b := tu.KeyboardButton(o)
		// organize options in two rows
		if len(options) > 0 && len(options[len(options)-1]) == 1 {
			options[len(options)-1] = append(options[len(options)-1], b)
		} else {
			options = append(options, tu.KeyboardRow(b))
		}
	}
	_, err = ctx.Bot().SendMessage(ctx, tu.Message(ctx.ChatID(), question.Text).
		WithReplyMarkup(tu.KeyboardGrid(options)))
	return err
}

func (s *quizState) handleAgain(ctx nabot.Context) error {
	return s.Render(ctx) // stay in this state and rerender
}

func (s *quizState) handleBack(ctx nabot.Context) error {
	return s.ToNext.Go(ctx)
}

func (s *quizState) handleOption(ctx nabot.Context, chosenOption string) error {
	currentQuestion, err := nabot.Get(ctx, currentQuestionKey)
	if err != nil {
		return err
	}
	text := correctAnswerMessage
	if correctOption := currentQuestion.Options[currentQuestion.CorrectIndex]; chosenOption != correctOption {
		text = fmt.Sprintf(wrongAnswerMessage, correctOption)
	}
	_, err = ctx.Bot().SendMessage(ctx, tu.Message(ctx.ChatID(), text).
		WithReplyMarkup(tu.Keyboard(tu.KeyboardRow(s.backButton.Button(), s.againButton.Button()))),
	)
	return err
}

type Category struct {
	Name      string
	Questions []Question
}

type Question struct {
	Text         string
	Options      []string
	CorrectIndex int
}

var (
	questions = map[string]Category{
		"math": {
			Name: "ریاضی",
			Questions: []Question{
				{
					Text:         "کدام یک از این اعداد از همه بزرگ‌تر است؟",
					Options:      []string{"9!", "3^8", "2^10", "6! × 5!"},
					CorrectIndex: 0,
				},
				{
					Text:         "در یک جمع ۲۵ نفره، احتمال اینکه حداقل دو نفر تاریخ تولد یکسانی داشته باشند به کدام گزینه نزدیک‌تر است؟",
					Options:      []string{"6%", "12%", "25%", "50%"},
					CorrectIndex: 3,
				},
				{
					Text:         "کدام یک از این اعداد عدد اول است؟",
					Options:      []string{"3171", "4153", "2873", "7051"},
					CorrectIndex: 1,
				},
				{
					Text:         "کدام یک از این عبارات از همه کوچک‌تر است؟",
					Options:      []string{"2^18", "3^12", "9!", "7^7"},
					CorrectIndex: 0,
				},
			},
		},
		"history": {
			Name: "تاریخی",
			Questions: []Question{
				{
					Text:         "کدام پادشاه هخامنشی دستور ساخت تخت جمشید را داد؟",
					Options:      []string{"کوروش بزرگ", "داریوش بزرگ", "خشایارشا", "اردشیر یکم"},
					CorrectIndex: 1,
				},
				{
					Text:         "واقعه مشروطه ایران در زمان سلطنت کدام پادشاه قاجار رخ داد؟",
					Options:      []string{"ناصرالدین‌شاه", "مظفرالدین‌شاه", "محمدعلی‌شاه", "احمدشاه"},
					CorrectIndex: 1,
				},
				{
					Text:         "نخستین سلسله ایرانی پس از ورود اسلام به ایران کدام بود؟",
					Options:      []string{"سامانیان", "صفاریان", "طاهریان", "غزنویان"},
					CorrectIndex: 2,
				},
				{
					Text:         "پایتخت ایران در دوره صفویان چه شهری بود؟",
					Options:      []string{"قزوین", "شیراز", "اصفهان", "تبریز"},
					CorrectIndex: 2,
				},
			},
		},
	}
)
