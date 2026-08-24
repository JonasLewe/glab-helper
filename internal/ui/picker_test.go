package ui

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestChooseReturnsOnlyAListedChoice(t *testing.T) {
	var input string
	var arguments []string
	picker := &Picker{output: func(_ context.Context, candidateInput string, args ...string) ([]byte, error) {
		input = candidateInput
		arguments = append([]string(nil), args...)
		return []byte("second\n"), nil
	}}

	selected, found, err := picker.Choose(context.Background(), []string{"first", "second"}, Options{Prompt: "Item", BorderLabel: "items"})
	if err != nil {
		t.Fatal(err)
	}
	if !found || selected != "second" {
		t.Fatalf("selection = %q, %t; want second, true", selected, found)
	}
	if input != "first\nsecond\n" {
		t.Fatalf("fzf input = %q", input)
	}
	wantArguments := []string{
		"--prompt=  Item > ",
		"--header=  ENTER=select  ESC=cancel",
		"--height=~40",
		"--reverse",
		"--border=rounded",
		"--border-label= items ",
	}
	if !reflect.DeepEqual(arguments, wantArguments) {
		t.Fatalf("fzf arguments = %q, want %q", arguments, wantArguments)
	}

	picker.output = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errCancelled
	}
	if _, found, err := picker.Choose(context.Background(), []string{"first"}, Options{}); err != nil || found {
		t.Fatalf("cancelled selection = found %t, error %v", found, err)
	}

	picker.output = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("not-listed\n"), nil
	}
	if _, _, err := picker.Choose(context.Background(), []string{"first"}, Options{}); err == nil {
		t.Fatal("unknown fzf output was accepted")
	}

	picker.output = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("failed")
	}
	if _, _, err := picker.Choose(context.Background(), []string{"first"}, Options{}); err == nil {
		t.Fatal("fzf failure was ignored")
	}
}

func TestChooseManyReturnsOnlyUniqueListedChoices(t *testing.T) {
	var arguments []string
	picker := &Picker{output: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		arguments = append([]string(nil), args...)
		return []byte("first\nthird\n"), nil
	}}

	selected, found, err := picker.ChooseMany(context.Background(), []string{"first", "second", "third"}, Options{Prompt: "Labels", BorderLabel: "labels"})
	if err != nil {
		t.Fatal(err)
	}
	if !found || !reflect.DeepEqual(selected, []string{"first", "third"}) {
		t.Fatalf("selection = %#v, %t", selected, found)
	}
	if len(arguments) == 0 || arguments[0] != "--multi" {
		t.Fatalf("fzf arguments = %q", arguments)
	}

	picker.output = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("first\nfirst\n"), nil
	}
	if _, _, err := picker.ChooseMany(context.Background(), []string{"first"}, Options{}); err == nil {
		t.Fatal("duplicate fzf output was accepted")
	}

	picker.output = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errCancelled
	}
	if _, found, err := picker.ChooseMany(context.Background(), []string{"first"}, Options{}); err != nil || found {
		t.Fatalf("cancelled selection = found %t, error %v", found, err)
	}
}

func TestPickerAddsColorOnlyWhenEnabled(t *testing.T) {
	var arguments []string
	picker := NewPicker(true)
	picker.output = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		arguments = append([]string(nil), args...)
		return []byte("first\n"), nil
	}

	if _, _, err := picker.Choose(context.Background(), []string{"first"}, Options{}); err != nil {
		t.Fatal(err)
	}
	want := "--color=border:cyan,header:-1:dim,prompt:cyan,pointer:cyan,marker:cyan"
	if len(arguments) == 0 || arguments[len(arguments)-1] != want {
		t.Fatalf("fzf arguments = %q, want final argument %q", arguments, want)
	}
}
