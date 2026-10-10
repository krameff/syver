package main

import (
	"testing"

	"github.com/krameff/syver/resource"
)

// TestAddSubcommandsCoverEveryAddableType checks that every resource type
// `syver add` can write (a descriptor with AppendSys) is listed in
// addSubcommandOrder and has help text in addSubcommandUsage, and that
// neither list names a type that is not addable. checkAccessorExhaustiveness
// in the root package covers dispatch.go's tables but not these two, and a
// type missing from addSubcommandOrder is skipped silently: `syver add
// <type>` just does not exist.
//
// REVERT-PROOF: deleting "registry" from addSubcommandOrder, or its entry
// from addSubcommandUsage, fails this test.
func TestAddSubcommandsCoverEveryAddableType(t *testing.T) {
	addable := map[string]bool{}
	for key, d := range resource.Descriptors() {
		if d.AppendSys != nil {
			addable[key] = true
		}
	}
	if len(addable) == 0 {
		t.Fatal("no addable resource types registered; the check below would be vacuous")
	}

	for key := range addable {
		n := 0
		for _, k := range addSubcommandOrder {
			if k == key {
				n++
			}
		}
		if n != 1 {
			t.Errorf("addable type %q appears %d times in addSubcommandOrder, want 1", key, n)
		}
		if addSubcommandUsage[key] == "" {
			t.Errorf("addable type %q has no addSubcommandUsage entry", key)
		}
	}
	for _, key := range addSubcommandOrder {
		if !addable[key] {
			t.Errorf("addSubcommandOrder lists %q, which is not an addable resource type", key)
		}
	}
	for key := range addSubcommandUsage {
		if !addable[key] {
			t.Errorf("addSubcommandUsage has an entry for %q, which is not an addable resource type", key)
		}
	}

	// And the lists reach the command tree: one subcommand per addable type.
	if got := len(addSubcommands()); got != len(addable) {
		t.Errorf("addSubcommands() built %d commands, want %d", got, len(addable))
	}
}
