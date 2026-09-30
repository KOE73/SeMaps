// Package model is the data side of the sample: every holds row of
// docs/extractors/go.md has an example here.
package model

// Base is embedded into User.
type Base struct {
	ID string
}

// Item is what everything else holds.
type Item struct {
	Name  string
	Price int
}

// Key keys ByKey.
type Key string

// Event travels over a channel.
type Event struct {
	At int64
}

// Status is an enum-like type; its constants are its members.
type Status int

const (
	StatusNew Status = iota
	StatusDone
)

// MaxItems is an untyped constant: a value without edges.
const MaxItems = 10

// Default is a package variable of an output type.
var Default Item

// User covers the member-relation features.
type User struct {
	Base                         // embedded: holds, native embed
	Name    string               // no symbol: no edge
	Owner   *Item                // pointer: one
	Items   []Item               // many, [item]
	Fixed   [2]Item              // many, [item]
	ByID    map[string]*Item     // keyed, [value]
	ByKey   map[Key]int          // keyed, [key]
	Events  chan Event           // many, deferred, [item]
	OnDone  func(Item) error     // deferred, [arg:0]
	Next    func() (Item, error) // deferred, [result:0]
	Nested  map[Key][]Item       // keyed, [key] and [value item]
	status  Status               // unexported: modifiers private
	Details struct{ Last Item }  // anonymous struct: [field:Last]
}

// Batch is a defined type over a slice: holds, native underlying.
type Batch []Item

// Admin is a defined type over a named type: holds, native underlying.
type Admin User

// HandlerFunc is a defined function type: uses, native signature.
type HandlerFunc func(in Item) Status

// ItemAlias is an alias: uses, native alias.
type ItemAlias = Item

// Box is generic; an instantiation Box[Item] points at Box and at Item.
type Box[T any] struct {
	V T
}
