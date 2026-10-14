// Package catalog describes the typed Terraform interface for the router's SLP API.
// Wire names and selectors come from the device's management pages, not a guessed REST API.
package catalog

import (
	"context"
	"terraform-provider-tplink-router/internal/client"
)

type Kind string

const (
	String     Kind = "string"
	Bool       Kind = "bool"
	Int        Kind = "int64"
	StringList Kind = "list_string"
	StringSet  Kind = "set_string"
	Object     Kind = "object"
	ObjectList Kind = "list_object"
	ObjectSet  Kind = "set_object"
)

type Mode string

const (
	Entry    Mode = "entry"    // Individually created and deleted table row.
	Settings Mode = "settings" // Existing singleton; destroy only releases management.
	Existing Mode = "existing" // Discovered hardware row; never create/delete hardware.
	Ruleset  Mode = "ruleset"  // One authoritative, ordered list of user rules.
)

type Field struct {
	Name, Wire, Description                  string
	Kind                                     Kind
	Required, ReadOnly, Sensitive, WriteOnly bool
	// Optional configurable attributes are computed unless they have an explicit Default.
	Default               any
	TrueValue, FalseValue string
	Enum                  []string
	Format                string // ipv4, ipv6, ip, cidr, mac
	MaxLength             int
	Min, Max              int64 // Applied to integer fields when Max > Min.
	Fields                []Field
	Separator             string // Nonempty means the wire value joins a string collection.
	Section               string // Override the singleton section for forms spanning several sections.
	RequiresReplace       bool
	// Transform hooks are reserved for device-specific encodings, not opaque public JSON.
	Encode func(any) (any, error)
	Decode func(any) (any, error)
}

type Spec struct {
	Name, Description, Category string
	Module, Table, Section      string
	WriteModule                 string // Rare cross-module writer, e.g. port PVID belongs to vlan.
	Mode                        Mode
	Capability                  string
	Fields                      []Field
	IDField                     string         // Stable wire identity; blank uses the returned section name.
	IDFields                    []string       // Composite identity; import uses slash-separated URL-escaped values in this order.
	NameField                   string         // Unique user-visible wire field, checked before create.
	FilterField                 string         // set/delete use filter:[{FilterField:id}] instead of section name.
	FilterList                  bool           // Selector value must be [id], e.g. interface.if_name.
	FilterFields                []string       // Composite selector fields; normally identical to IDFields.
	FilterListFields            []string       // Composite selector values that must be wrapped in a list.
	Filter                      map[string]any // Static filter included alongside the identity.
	Query                       map[string]any // Additional keys within the module's read request.
	CreateDefaults              map[string]any
	WriteDefaults               map[string]any
	OmitWriteFields             []string
	PreserveWriteFields         []string            // Required unchanged form fields copied from the current row on update.
	Protected                   map[string][]string // Immutable firmware objects, by wire field/value.
	ProtectUpdates              bool                // Most built-in interfaces are updateable but never deletable.
	// Source is the management page used to establish this API contract.
	Source, Notes string
	// A data-source-only spec is used for operational state, never for guessed writable APIs.
	DataSourceOnly bool
	// Ordered tables normally append new rules. An explicit move action is optional.
	OrderField           string
	CreateViaSet         bool                                                                  // Binding tables create membership through set(table,filter,para).
	DeleteValues         map[string]any                                                        // Binding deletion updates an explicit unbind value.
	DeleteIdentityFields []string                                                              // Include these identity fields in an unbind update.
	ReadRows             func(context.Context, *client.Client, Spec) ([]map[string]any, error) // Typed multi-query adapters return wire rows, with .name when available.
	// Resolve selects firmware modes at operation time without changing the public schema.
	Resolve func(context.Context, *client.Client, Spec) (Spec, error)
}

func Text(name, wire, description string) Field {
	return Field{Name: name, Wire: wire, Description: description, Kind: String}
}

func RequiredText(name, wire, description string) Field {
	f := Text(name, wire, description)
	f.Required = true
	return f
}

func Boolean(name, wire, description, on, off string) Field {
	return Field{Name: name, Wire: wire, Description: description, Kind: Bool, TrueValue: on, FalseValue: off}
}

func Integer(name, wire, description string) Field {
	return Field{Name: name, Wire: wire, Description: description, Kind: Int}
}

func Strings(name, wire, description string) Field {
	return Field{Name: name, Wire: wire, Description: description, Kind: StringList}
}

func Secret(name, wire, description string) Field {
	return Field{Name: name, Wire: wire, Description: description, Kind: String, Sensitive: true, WriteOnly: true}
}

func ReadText(name, wire, description string) Field {
	f := Text(name, wire, description)
	f.ReadOnly = true
	return f
}

func All() []Spec {
	var specs []Spec
	specs = append(specs, NetworkSpecs()...)
	specs = append(specs, SecuritySpecs()...)
	specs = append(specs, AdvancedSpecs()...)
	specs = append(specs, SystemSpecs()...)
	return specs
}
