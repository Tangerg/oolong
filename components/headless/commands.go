package headless

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/core/fuzzy"
)

// Command is the searchable description of one thing a user can ask for by name.
// What the command means belongs to the caller and is stored as the value of
// [Commands], not prescribed here.
type Command struct {
	// Name is the canonical name a user searches or types. It is the identity: two
	// commands with the same name are one command, and the second registration wins.
	Name string
	// Title is the one-line description a list shows beside the name.
	Title string
	// Aliases are other names that find this command. They are matched but never
	// listed, so a command can be renamed without the old name disappearing from
	// under everyone who learned it.
	Aliases []string
}

// Commands is a set of named commands that can be found by typing part of a name.
//
// # Why the matching is not simply a prefix
//
// A user who knows a command types enough of it and expects to be right. A user who
// does not know it types what they remember, which is rarely the beginning: "sess"
// for "new-session", "clr" for "clear". Matching on subsequences with a bias towards
// word starts finds both, which is why this ranks with [fuzzy] rather than filtering
// on a prefix.
//
// # Why order is remembered
//
// The command somebody ran a moment ago is overwhelmingly the one they want next, and
// no amount of scoring the names will discover that. So ties are broken by how
// recently a command was used, and an empty query lists the recent ones first. It is
// the only part of the ranking that knows anything about this particular user.
//
// T is the caller-owned meaning associated with a command. The registry keeps and
// returns it by assignment, as a map does; it never interprets, invokes, or copies
// through references inside it. This keeps command execution, arguments, and product
// syntax outside the component while avoiding a second application-side lookup table.
//
// The zero value is an empty registry. A Commands value must not be copied after
// first use; registration and recency are one mutable index.
type Commands[T any] struct {
	noCopy noCopy

	list []registeredCommand[T]
	// used holds the names most recently run, newest first.
	used []string
}

type registeredCommand[T any] struct {
	command Command
	value   T
}

// Add associates value with a command, replacing both for an existing name. The
// registry copies the command's text and aliases; value follows ordinary assignment
// semantics for T.
func (c *Commands[T]) Add(cmd Command, value T) {
	if cmd.Name == "" {
		return
	}
	cmd = cloneCommand(cmd)
	for i := range c.list {
		if c.list[i].command.Name == cmd.Name {
			c.list[i] = registeredCommand[T]{command: cmd, value: value}
			return
		}
	}
	c.list = append(c.list, registeredCommand[T]{command: cmd, value: value})
}

// Remove forgets the command with canonical name, reporting whether it was there.
// Aliases are lookup spellings rather than identities.
func (c *Commands[T]) Remove(name string) bool {
	for i := range c.list {
		if c.list[i].command.Name != name {
			continue
		}
		c.list = trim(slices.Delete(c.list, i, i+1))
		c.used = slices.DeleteFunc(c.used, func(s string) bool { return s == name })
		return true
	}
	return false
}

// Len is how many commands are registered.
func (c *Commands[T]) Len() int { return len(c.list) }

// Lookup returns a command snapshot and its caller-owned value for exactly this name
// or alias. An exact name wins over every alias: an older spelling must not shadow
// the canonical name of another command.
func (c *Commands[T]) Lookup(name string) (Command, T, bool) {
	entry, ok := c.lookup(name)
	if !ok {
		var zero T
		return Command{}, zero, false
	}
	return cloneCommand(entry.command), entry.value, true
}

func (c *Commands[T]) lookup(name string) (registeredCommand[T], bool) {
	for _, entry := range c.list {
		if entry.command.Name == name {
			return entry, true
		}
	}
	for _, entry := range c.list {
		if slices.Contains(entry.command.Aliases, name) {
			return entry, true
		}
	}
	return registeredCommand[T]{}, false
}

// Used records that a command was run, which is what moves it up the list.
func (c *Commands[T]) Used(name string) {
	entry, ok := c.lookup(name)
	if !ok {
		return
	}
	name = entry.command.Name
	c.used = slices.DeleteFunc(c.used, func(s string) bool { return s == name })
	c.used = append([]string{strings.Clone(name)}, c.used...)
	if len(c.used) > maxRecentCommands {
		c.used = c.used[:maxRecentCommands]
	}
}

// maxRecentCommands bounds what is remembered. Past a handful, "recently" stops
// meaning anything and the list is just the registry again.
const maxRecentCommands = 16

// Found is a command a query matched, and where in its name it matched.
type Found struct {
	Command Command
	// At is the byte offsets in the name that the query matched, for underlining
	// them. It is empty for a match on an alias or for an empty query, because
	// neither has anything to point at in the name being shown.
	At []int
}

// Find is the commands a query matches, best first.
//
// An empty query is every command, most recently used first, which is what a palette
// shows when it opens.
func (c *Commands[T]) Find(query string) []Found {
	if query == "" {
		out := make([]Found, 0, len(c.list))
		for _, cmd := range c.byRecency() {
			out = append(out, Found{Command: cloneCommand(cmd)})
		}
		return out
	}

	// Names first, so that what is shown is what was matched.
	names := make([]string, len(c.list))
	for i, entry := range c.list {
		names[i] = entry.command.Name
	}
	out := make([]Found, 0, len(c.list))
	matched := make([]bool, len(c.list))
	for _, r := range fuzzy.Filter(query, names) {
		matched[r.Index] = true
		out = append(out, Found{Command: cloneCommand(c.list[r.Index].command), At: r.Match.At})
	}
	// Then the aliases, for commands the name did not find. An alias match has
	// nothing to underline in the name, which is honest: the user typed something
	// else.
	for i, entry := range c.list {
		if matched[i] {
			continue
		}
		cmd := entry.command
		for _, alias := range cmd.Aliases {
			if _, ok := fuzzy.Score(query, alias); ok {
				out = append(out, Found{Command: cloneCommand(cmd)})
				break
			}
		}
	}
	return out
}

// cloneCommand makes the registry the owner of its values on the way in and gives
// callers snapshots on the way out. Aliases are the only mutable part, but cloning
// the strings too prevents a short-lived parser buffer from being retained by a
// long-lived registry through a small substring.
func cloneCommand(cmd Command) Command {
	cmd.Name = strings.Clone(cmd.Name)
	cmd.Title = strings.Clone(cmd.Title)
	cmd.Aliases = slices.Clone(cmd.Aliases)
	for i := range cmd.Aliases {
		cmd.Aliases[i] = strings.Clone(cmd.Aliases[i])
	}
	return cmd
}

// byRecency is every command, the recently used ones first and the rest in the order
// they were registered.
func (c *Commands[T]) byRecency() []Command {
	out := make([]Command, 0, len(c.list))
	for _, name := range c.used {
		if entry, ok := c.lookup(name); ok {
			out = append(out, cloneCommand(entry.command))
		}
	}
	for _, entry := range c.list {
		if !slices.Contains(c.used, entry.command.Name) {
			out = append(out, cloneCommand(entry.command))
		}
	}
	return out
}
