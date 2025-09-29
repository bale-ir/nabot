package nabot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mymmrac/telego"
	"log/slog"
	"slices"
	"sync"
)

var (
	ErrStateNotFound = errors.New("state not found")
)

// StateStorage stores and retrieves state stacks for StateHandler.
// An in-memory implementation is available via NewInMemoryStateStore.
type StateStorage interface {
	GetStack(ctx context.Context, chatKey string) ([]byte, error)
	// SetStack stores the stack for the given chat key, replacing any existing stack.
	SetStack(ctx context.Context, chatKey string, stack []byte) error
}

type memoryStateStore struct {
	data sync.Map
}

// NewInMemoryStateStore creates an in-memory state storage.
func NewInMemoryStateStore() StateStorage {
	return &memoryStateStore{}
}

func (m *memoryStateStore) GetStack(_ context.Context, chatKey string) ([]byte, error) {
	v, ok := m.data.Load(chatKey)
	val, ok2 := v.([]byte)
	if !ok || !ok2 {
		return nil, ErrStateNotFound
	}
	return val, nil
}

func (m *memoryStateStore) SetStack(_ context.Context, chatKey string, stack []byte) error {
	m.data.Store(chatKey, stack)
	return nil
}

// StateHandler manages bot states using a stack-based approach.
// Each chat has its own state stack. The top state on the stack handles updates.
//
// Example:
//
//	stateHandler := nabot.NewStateHandler(app)
//	toMainState := stateHandler.RegisterState(myMainState)
//	app.Handle(stateHandler)
type StateHandler struct {
	app     *App
	states  map[string]State
	storage StateStorage
}

// NewStateHandler creates a new state handler.
func NewStateHandler(app *App, options ...StateHandlerOption) *StateHandler {
	sh := &StateHandler{
		app:     app,
		states:  make(map[string]State),
		storage: NewInMemoryStateStore(),
	}
	for _, option := range options {
		option(sh)
	}
	return sh
}

func (s *StateHandler) Name() string {
	return "state_handler"
}

func (s *StateHandler) Handle(ctx Context) error {
	stack, err := s.getStack(ctx, ctx.ChatKey())
	if err != nil {
		return err
	}
	if stack == nil {
		return ErrPass
	}
	top := stack[len(stack)-1]
	ctx = ContextWithLogger(ctx, ctx.Logger().With(slog.String("state", top.Name())))
	return top.Handle(ctx)
}

// RegisterState registers a state and returns a Transition to it.
// Calling the returned Transition, changes the current state to the registered state.
// State names must be unique.
//
// Example:
//
//	toMain := stateHandler.RegisterState(mainState)
//	toMain.Go(ctx) // transitions to mainState
func (s *StateHandler) RegisterState(state State) Transition {
	if state == nil {
		panic("nabot: cannot register nil state")
	}
	if _, ok := s.states[state.Name()]; ok {
		panic(fmt.Sprintf("nabot: a state with name %q already exists", state.Name()))
	}
	s.states[state.Name()] = state
	return toState{
		stateHandler: s,
		state:        state,
	}
}

// RegisterAndChainStates registers multiple ChainableState instances and links them together.
// Returns a Transition to the first state. Each state's Next() will point to the next state.
//
// Example:
//
//	toMainState := stateHandler.RegisterAndChainStates(
//	    mainState,
//	    quizState,
//	)
//	// mainState.Next() now points to quizState
func (s *StateHandler) RegisterAndChainStates(states ...ChainableState) Transition {
	if len(states) == 0 {
		panic("nabot: at least one chainable state required")
	}
	var t Transition
	for i := len(states) - 1; i >= 0; i-- {
		if t != nil {
			*states[i].Next() = t
		}
		t = s.RegisterState(states[i])
	}
	return t
}

func (s *StateHandler) getStack(ctx context.Context, key string) ([]State, error) {
	st, err := s.storage.GetStack(ctx, key)
	if errors.Is(err, ErrStateNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get stack: %w", err)
	}
	var states []string
	if err = json.Unmarshal(st, &states); err != nil {
		s.app.logger.Error("failed to unmarshal stored stack. skipping state handler", "error", err)
		return nil, nil
	}

	result := make([]State, 0, len(states))
	for _, name := range states {
		if state, ok := s.states[name]; ok {
			result = append(result, state)
		}
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func (s *StateHandler) setStack(ctx context.Context, key string, stack []State) error {
	var states []string
	for _, st := range stack {
		if _, ok := s.states[st.Name()]; ok {
			states = append(states, st.Name())
		}
	}
	st, err := json.Marshal(&states)
	if err != nil {
		return fmt.Errorf("failed to marshal stack: %w", err)
	}
	if err := s.storage.SetStack(ctx, key, st); err != nil {
		return fmt.Errorf("failed to set stack: %w", err)
	}
	return nil
}

// Back returns a Transition that goes back to the previous state on the stack.
// If the stack becomes empty, no state will be active and StateHandler skips all updates.
func (s *StateHandler) Back() Transition {
	return back{
		stateHandler: s,
	}
}

// StateHandlerOption configures a StateHandler.
type StateHandlerOption func(*StateHandler)

// WithStateStore sets a custom state storage implementation.
// Default is NewInMemoryStateStore().
func WithStateStore(stateStore StateStorage) StateHandlerOption {
	return func(s *StateHandler) {
		s.storage = stateStore
	}
}

// Transition represents a state transition.
// Call Go to perform the transition.
type Transition interface {
	Go(ctx TransitionContext) error
}

type toState struct {
	stateHandler *StateHandler
	state        State
}

func (t toState) Go(ctx TransitionContext) error {
	stack, err := t.stateHandler.getStack(ctx, ctx.ChatKey())
	if err != nil {
		return err
	}

	idx := slices.IndexFunc(stack, func(state State) bool {
		return state.Name() == t.state.Name()
	})

	if idx >= 0 {
		stack = stack[:idx+1]
	} else {
		stack = append(stack, t.state)
	}

	err = t.stateHandler.setStack(ctx, ctx.ChatKey(), stack)
	if err != nil {
		return err
	}
	return t.state.Render(ctx)
}

type back struct {
	stateHandler *StateHandler
}

func (b back) Go(ctx TransitionContext) error {
	stack, err := b.stateHandler.getStack(ctx, ctx.ChatKey())
	if err != nil {
		return err
	}
	if len(stack) == 0 {
		return nil
	}
	stack = stack[:len(stack)-1]
	err = b.stateHandler.setStack(ctx, ctx.ChatKey(), stack)
	if err != nil {
		return err
	}

	if len(stack) > 0 {
		return stack[len(stack)-1].Render(ctx)
	}
	return nil
}

// TransitionContext provides dependencies for state transitions and rendering.
type TransitionContext interface {
	Bot() *telego.Bot
	ChatID() telego.ChatID
	StorageContext
}

// State represents a bot state that can handle updates and also render a UI.
// State names must be unique within a StateHandler.
type State interface {
	Handler
	Render(ctx TransitionContext) error
}

// ChainableState is a State that can be linked to another state.
// Used in StateHandler.RegisterAndChainStates to create state chains.
type ChainableState interface {
	State
	Next() *Transition
}

// BaseState implements State with configurable fields.
// Embed this in your state structs to quickly create states.
//
// Example:
//
//	type MyState struct {
//	    nabot.BaseState
//	    // ... your fields
//	}
//
//	func NewMyState() nabot.ChainableState {
//	    s := &MyState{}
//	    s.BaseState = nabot.BaseState{
//	        ID: "my_state",
//	        Renderer: s.Render,
//	        Handlers: []nabot.Handler{
//			    handlers.Func(func(ctx nabot.Context) error {
//				    return s.ToNext.Go(ctx)
//			    }),
//	        },
//	    }
//	    return s
//	}
type BaseState struct {
	ID       string
	Renderer func(ctx TransitionContext) error
	Handlers []Handler
	ToNext   Transition
}

func (b *BaseState) Name() string {
	return b.ID
}

func (b *BaseState) Render(ctx TransitionContext) error {
	if b.Renderer == nil {
		return nil
	}
	return b.Renderer(ctx)
}

func (b *BaseState) Handle(ctx Context) error {
	var err error
	for _, h := range b.Handlers {
		err = h.Handle(ctx)
		if errors.Is(err, ErrPass) {
			continue
		}
		break
	}
	return err
}

func (b *BaseState) Next() *Transition {
	return &b.ToNext
}
