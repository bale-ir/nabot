package nabot

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

var (
	ErrDataKeyNotFound = errors.New("key not found")
)

// DataStorage stores arbitrary data for each chat.
// An in-memory implementation is available via NewInMemoryDataStore.
// You can implement this interface to use databases or caches.
type DataStorage interface {
	SetData(ctx context.Context, chatKey string, dataKey string, value any) error
	GetData(ctx context.Context, chatKey string, dataKey string, pointer any) error
	RemoveData(ctx context.Context, chatKey string, dataKey string) error
	ClearData(ctx context.Context, chatKey string) error
}

// StorageContext provides dependencies for DataStorage operations.
type StorageContext interface {
	ChatKey() string
	Store() DataStorage
	context.Context
}

// DataKey is a type-safe key for storing and retrieving data from DataStorage.
// Each chat has its own map of data that persists across updates.
//
// Example:
//
//		const firstNameKey nabot.DataKey[string] = "first_name"
//
//	 func myHandler(ctx nabot.Context) error {
//		  // Store data
//		  nabot.Set(ctx, firstNameKey, "John")
//
//		  // Retrieve data
//		  name, err := nabot.Get(ctx, firstNameKey)
//	   ...
//	 }
type DataKey[T any] string

// Set stores a value in the chat's DataStorage.
//
// Example:
//
//	 func myHandler(ctx nabot.Context) error {
//		  // Store data
//		  nabot.Set(ctx, firstNameKey, "John")
//	   ...
//	 }
func Set[T any](c StorageContext, key DataKey[T], value T) error {
	err := c.Store().SetData(c, c.ChatKey(), string(key), value)
	if err != nil {
		return fmt.Errorf("failed to set data: %w", err)
	}
	return nil
}

// Get retrieves a value from the chat's DataStorage.
// Returns ErrDataKeyNotFound if the key does not exist.
//
// Example:
//
//	 func myHandler(ctx nabot.Context) error {
//		  // Retrieve data
//		  name, err := nabot.Get(ctx, firstNameKey)
//	   ...
//	 }
func Get[T any](c StorageContext, key DataKey[T]) (T, error) {
	var result T
	err := c.Store().GetData(c, c.ChatKey(), string(key), &result)
	if err != nil {
		if errors.Is(err, ErrDataKeyNotFound) {
			return result, err
		}
		return result, fmt.Errorf("failed to get data: %w", err)
	}
	return result, nil
}

// Remove deletes a key from the chat's DataStorage.
func Remove[T any](c StorageContext, key DataKey[T]) error {
	err := c.Store().RemoveData(c, c.ChatKey(), string(key))
	if err != nil {
		return fmt.Errorf("failed to remove data: %w", err)
	}
	return nil
}

// Clear removes all data of a chat from the chat's DataStorage.
func Clear(c StorageContext) error {
	err := c.Store().ClearData(c, c.ChatKey())
	if err != nil {
		return fmt.Errorf("failed to clear data: %w", err)
	}
	return nil
}

type memoryStore struct {
	data      sync.Map
	navStacks sync.Map
}

// NewInMemoryDataStore creates an in-memory data storage.
func NewInMemoryDataStore() DataStorage {
	return &memoryStore{}
}

func (m *memoryStore) SetData(_ context.Context, chatKey string, key string, value any) error {
	d, _ := m.data.LoadOrStore(chatKey, &sync.Map{})
	data := d.(*sync.Map)
	data.Store(key, value)
	return nil
}

func (m *memoryStore) GetData(_ context.Context, chatKey string, key string, pointer any) error {
	d, ok := m.data.Load(chatKey)
	if !ok {
		return ErrDataKeyNotFound
	}
	data := d.(*sync.Map)
	v, ok := data.Load(key)
	if !ok {
		return ErrDataKeyNotFound
	}
	p := reflect.ValueOf(pointer).Elem()
	val := reflect.ValueOf(v)
	if !val.Type().AssignableTo(p.Type()) {
		return fmt.Errorf("stored value of type %v is not assignable to type %v", val.Type(), p.Type())
	}
	p.Set(val)
	return nil
}

func (m *memoryStore) RemoveData(_ context.Context, chatKey string, key string) error {
	d, _ := m.data.LoadOrStore(chatKey, &sync.Map{})
	data := d.(*sync.Map)
	data.Delete(key)
	return nil
}

func (m *memoryStore) ClearData(_ context.Context, chatKey string) error {
	m.data.Delete(chatKey)
	return nil
}
