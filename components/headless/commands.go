package headless

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/core/fuzzy"
)

// Command is one thing a user can ask for by name.
type Command struct {
	// Name is what the user types, without the slash. It is the identity: two
	// commands with the same name are one command, and the second registration wins.
	Name string
	// Title is the one-line description a list shows beside the name.
	Title string
	// Aliases are other names that find this command. They are matched but never
	// listed, so a command can be renamed without the old name disappearing from
	// under everyone who learned it.
	Aliases []string
	// Takes says the command expects an argument after its name, which is what tells
	// a caller to ask for one rather than run it.
	Takes bool
	// Run is what it does. It is the caller's own function and this package never
	// calls it: a registry that ran things would need to know what they could be run
	// against, and that is the whole of an application.
	Run func(arg string)
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
type Commands struct {
	list []Command
	// used holds the names most recently run, newest first.
	used []string
}

// Add registers a command, replacing one of the same name. The registry copies the
// command's aliases; the caller may reuse or change its input afterwards.
func (c *Commands) Add(cmd Command) {
	if cmd.Name == "" {
		return
	}
	cmd = cloneCommand(cmd)
	for i := range c.list {
		if c.list[i].Name == cmd.Name {
			c.list[i] = cmd
			return
		}
	}
	c.list = append(c.list, cmd)
}

// Remove forgets a command, reporting whether it was there.
func (c *Commands) Remove(name string) bool {
	for i := range c.list {
		if c.list[i].Name != name {
			continue
		}
		c.list = trim(slices.Delete(c.list, i, i+1))
		c.used = slices.DeleteFunc(c.used, func(s string) bool { return s == name })
		return true
	}
	return false
}

// Len is how many commands are registered.
func (c *Commands) Len() int { return len(c.list) }

// Lookup is a snapshot of the command with exactly this name or alias. An exact name
// wins over every alias: compatibility spelling on an older command must not shadow
// the canonical name of another command.
func (c *Commands) Lookup(name string) (Command, bool) {
	for _, cmd := range c.list {
		if cmd.Name == name {
			return cloneCommand(cmd), true
		}
	}
	for _, cmd := range c.list {
		if slices.Contains(cmd.Aliases, name) {
			return cloneCommand(cmd), true
		}
	}
	return Command{}, false
}

// Used records that a command was run, which is what moves it up the list.
func (c *Commands) Used(name string) {
	cmd, ok := c.Lookup(name)
	if !ok {
		return
	}
	name = cmd.Name
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
func (c *Commands) Find(query string) []Found {
	query = strings.TrimPrefix(query, "/")
	if query == "" {
		out := make([]Found, 0, len(c.list))
		for _, cmd := range c.byRecency() {
			out = append(out, Found{Command: cloneCommand(cmd)})
		}
		return out
	}

	// Names first, so that what is shown is what was matched.
	names := make([]string, len(c.list))
	for i, cmd := range c.list {
		names[i] = cmd.Name
	}
	out := make([]Found, 0, len(c.list))
	matched := make([]bool, len(c.list))
	for _, r := range fuzzy.Filter(query, names) {
		matched[r.Index] = true
		out = append(out, Found{Command: cloneCommand(c.list[r.Index]), At: r.Match.At})
	}
	// Then the aliases, for commands the name did not find. An alias match has
	// nothing to underline in the name, which is honest: the user typed something
	// else.
	for i, cmd := range c.list {
		if matched[i] {
			continue
		}
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
func (c *Commands) byRecency() []Command {
	out := make([]Command, 0, len(c.list))
	for _, name := range c.used {
		if cmd, ok := c.Lookup(name); ok {
			out = append(out, cmd)
		}
	}
	for _, cmd := range c.list {
		if !slices.Contains(c.used, cmd.Name) {
			out = append(out, cmd)
		}
	}
	return out
}

// Parse splits a typed line into a command name and its argument.
//
// It reports false for anything that is not a command, which is any line that does
// not begin with a slash. A line that is only a slash is a command with no name, and
// is what a palette opens on.
func Parse(line string) (name, arg string, ok bool) {
	if !strings.HasPrefix(line, "/") {
		return "", "", false
	}
	name, arg, _ = strings.Cut(strings.TrimPrefix(line, "/"), " ")
	return name, strings.TrimSpace(arg), true
}
